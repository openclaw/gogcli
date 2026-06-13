package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

func defaultAuthTestOperations() (
	app.OpenSecretsStoreFunc,
	app.AuthorizeGoogleFunc,
	app.EnsureKeychainAccessFunc,
	app.FetchAuthorizedIdentityFunc,
) {
	return secrets.OpenDefault, googleauth.Authorize, secrets.EnsureKeychainAccessContext, googleauth.IdentityForRefreshToken
}

func runtimeWithAuthTestOperations(
	openStore app.OpenSecretsStoreFunc,
	authorize app.AuthorizeGoogleFunc,
	ensureKeychain app.EnsureKeychainAccessFunc,
	fetchIdentity app.FetchAuthorizedIdentityFunc,
) *app.Runtime {
	return &app.Runtime{Auth: app.AuthOperations{
		OpenSecretsStore:        openStore,
		AuthorizeGoogle:         authorize,
		EnsureKeychainAccess:    ensureKeychain,
		FetchAuthorizedIdentity: fetchIdentity,
	}}
}

func TestAuthAddCmd_JSON(t *testing.T) {
	var (
		authorizeGoogle         app.AuthorizeGoogleFunc
		ensureKeychainAccess    app.EnsureKeychainAccessFunc
		fetchAuthorizedIdentity app.FetchAuthorizedIdentityFunc
	)
	var openSecretsStore app.OpenSecretsStoreFunc
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"gmail,drive,gmail",
				"--manual",
				"--force-consent",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !gotOpts.Manual || !gotOpts.ForceConsent {
		t.Fatalf("expected options set, got %+v", gotOpts)
	}
	if len(gotOpts.Services) != 2 {
		t.Fatalf("expected deduped services, got %v", gotOpts.Services)
	}

	var parsed struct {
		Stored   bool     `json:"stored"`
		Email    string   `json:"email"`
		Services []string `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if !parsed.Stored || parsed.Email != "user@example.com" || len(parsed.Services) != 2 {
		t.Fatalf("unexpected response: %#v", parsed)
	}
	tok, err := store.GetToken(config.DefaultClientName, "user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt" || !strings.Contains(strings.Join(tok.Services, ","), "gmail") {
		t.Fatalf("unexpected token: %#v", tok)
	}
}

func TestAuthAddCmd_KeychainError(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")

	var (
		authorizeGoogle         app.AuthorizeGoogleFunc
		ensureKeychainAccess    app.EnsureKeychainAccessFunc
		fetchAuthorizedIdentity app.FetchAuthorizedIdentityFunc
	)

	// Simulate keychain locked error
	ensureKeychainAccess = func(context.Context) error {
		return errors.New("keychain is locked")
	}

	authCalled := false
	authorizeGoogle = func(_ context.Context, _ googleauth.AuthorizeOptions) (string, error) {
		authCalled = true
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		t.Fatal("fetchAuthorizedIdentity should not be called when keychain check fails")
		return googleauth.Identity{}, nil
	}

	store := newMemSecretsStore()
	openSecretsStore := func() (secrets.Store, error) { return store, nil }

	cmd := &AuthAddCmd{Email: "test@example.com", ServicesCSV: "gmail"}
	ctx := app.WithRuntime(withTestClientResolver(context.Background()), runtimeWithAuthTestOperations(
		openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
	))
	err := cmd.Run(ctx, &RootFlags{})

	if err == nil {
		t.Fatal("expected error when keychain is locked")
	}
	if !strings.Contains(err.Error(), "keychain") {
		t.Errorf("expected error to mention keychain, got: %v", err)
	}
	if authCalled {
		t.Error("authorizeGoogle should not be called when keychain check fails")
	}
}

type setTokenErrorStore struct {
	*memSecretsStore
	err error
}

func (s *setTokenErrorStore) SetToken(string, string, secrets.Token) error {
	return s.err
}

type listTokenErrorStore struct {
	*memSecretsStore
	err error
}

func (s *listTokenErrorStore) ListTokens() ([]secrets.Token, error) {
	return nil, s.err
}

func TestAuthAddCmd_ListTokenFailureDoesNotBlockFreshTokenSave(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Subject: "sub-123", Email: "user@example.com"}, nil
	}

	store := &listTokenErrorStore{
		memSecretsStore: newMemSecretsStore(),
		err:             errors.New("read encoded file keyring item: aes.KeyUnwrap(): integrity check failed"),
	}
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	if err := execute([]string{"auth", "add", "user@example.com", "--services", "gmail", "--manual"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	tok, err := store.GetToken(config.DefaultClientName, "user@example.com")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok.RefreshToken != "rt" || tok.Subject != "sub-123" {
		t.Fatalf("unexpected saved token: %#v", tok)
	}
}

func TestExecuteAuthAddMigratesRuntimeEmailReferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ambientStore := defaultConfigStoreForTest(t)
	if err := ambientStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
	}); err != nil {
		t.Fatalf("write ambient config: %v", err)
	}
	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := runtimeStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
		AccountClients: map[string]string{"old@example.com": "work-client"},
	}); err != nil {
		t.Fatalf("write runtime config: %v", err)
	}

	store := newMemSecretsStore()
	if err := store.SetToken(config.DefaultClientName, "old@example.com", secrets.Token{
		Subject:      "sub-123",
		Email:        "old@example.com",
		RefreshToken: "old-refresh",
	}); err != nil {
		t.Fatalf("set old token: %v", err)
	}
	store.defaults[config.DefaultClientName] = "old@example.com"

	runtime := runtimeWithAuthTestOperations(
		func() (secrets.Store, error) { return store, nil },
		func(context.Context, googleauth.AuthorizeOptions) (string, error) { return "new-refresh", nil },
		func(context.Context) error { return nil },
		func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
			return googleauth.Identity{Subject: "sub-123", Email: "new@example.com"}, nil
		},
	)
	runtime.Config = runtimeStore

	result := executeWithTestRuntime(t, []string{
		"auth", "add", "new@example.com", "--services", "gmail", "--manual",
	}, runtime)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}

	runtimeCfg, err := runtimeStore.Read()
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if runtimeCfg.AccountAliases["work"] != "new@example.com" || runtimeCfg.AccountClients["new@example.com"] != "work-client" {
		t.Fatalf("runtime config = %#v", runtimeCfg)
	}
	ambientCfg, err := ambientStore.Read()
	if err != nil {
		t.Fatalf("read ambient config: %v", err)
	}
	if ambientCfg.AccountAliases["work"] != "old@example.com" {
		t.Fatalf("ambient config changed: %#v", ambientCfg)
	}
	if store.defaults[config.DefaultClientName] != "new@example.com" {
		t.Fatalf("default account = %q", store.defaults[config.DefaultClientName])
	}
}

func TestAuthAddCmd_StoreFailureReportsOAuthCompleted(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	openSecretsStore = func() (secrets.Store, error) {
		return &setTokenErrorStore{
			memSecretsStore: newMemSecretsStore(),
			err:             errors.New("keyring connection timed out after 10s while storing keyring item"),
		}, nil
	}

	err := execute([]string{"auth", "add", "user@example.com", "--services", "gmail", "--manual"})
	if err == nil {
		t.Fatal("expected store failure")
	}
	if !strings.Contains(err.Error(), "OAuth completed, but saving the refresh token failed") {
		t.Fatalf("expected post-OAuth store failure context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "storing keyring item") {
		t.Fatalf("expected keyring operation detail, got: %v", err)
	}
}

func TestAuthAddCmd_DefaultServices_UserPreset(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"--json", "auth", "add", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	want := googleauth.UserServices()
	if len(gotOpts.Services) != len(want) {
		t.Fatalf("unexpected services: %v", gotOpts.Services)
	}
	for _, s := range gotOpts.Services {
		if s == googleauth.ServiceKeep {
			t.Fatalf("unexpected keep in services: %v", gotOpts.Services)
		}
	}
}

func TestAuthAddCmd_KeepRejected(t *testing.T) {
	authorizeCalled := false
	authorizeGoogle := func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		authorizeCalled = true
		return "", nil
	}

	err := executeWithRuntime(
		[]string{"auth", "add", "user@example.com", "--services", "keep"},
		&app.Runtime{Auth: app.AuthOperations{AuthorizeGoogle: authorizeGoogle}},
	)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %#v", err, err)
	}
	if !strings.Contains(err.Error(), "keep auth") {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizeCalled {
		t.Fatalf("authorizeGoogle should not be called")
	}
}

func TestAuthAddCmd_EmailMismatch(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "actual@example.com"}, nil
	}

	err := execute([]string{"auth", "add", "expected@example.com"})
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "authorized as actual@example.com") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddCmd_ReadonlyScopes(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"gmail,drive,calendar",
				"--readonly",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.readonly") {
		t.Fatalf("missing gmail.readonly in %v", gotOpts.Scopes)
	}
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("missing drive.readonly in %v", gotOpts.Scopes)
	}
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/calendar.readonly") {
		t.Fatalf("missing calendar.readonly in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://mail.google.com/") {
		t.Fatalf("unexpected https://mail.google.com/ in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.settings.basic") {
		t.Fatalf("unexpected gmail.settings.basic in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.settings.sharing") {
		t.Fatalf("unexpected gmail.settings.sharing in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("unexpected drive in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/calendar") {
		t.Fatalf("unexpected calendar in %v", gotOpts.Scopes)
	}
}

func TestAuthAddCmd_GmailScopeReadonly(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"gmail,drive",
				"--gmail-scope",
				"readonly",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.readonly") {
		t.Fatalf("missing gmail.readonly in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.modify") {
		t.Fatalf("unexpected gmail.modify in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.settings.basic") {
		t.Fatalf("unexpected gmail.settings.basic in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.settings.sharing") {
		t.Fatalf("unexpected gmail.settings.sharing in %v", gotOpts.Scopes)
	}
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("missing drive in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("unexpected drive.readonly in %v", gotOpts.Scopes)
	}
	if !gotOpts.DisableIncludeGrantedScopes {
		t.Fatalf("expected DisableIncludeGrantedScopes when using --gmail-scope=readonly")
	}
}

func TestAuthAddCmd_DriveScopeReadonly(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"drive",
				"--drive-scope",
				"readonly",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("missing drive.readonly in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("unexpected drive in %v", gotOpts.Scopes)
	}
	if !gotOpts.DisableIncludeGrantedScopes {
		t.Fatalf("expected DisableIncludeGrantedScopes when using --drive-scope=readonly")
	}
}

func TestAuthAddCmd_DriveScopeFile(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"drive",
				"--drive-scope",
				"file",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive.file") {
		t.Fatalf("missing drive.file in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("unexpected drive in %v", gotOpts.Scopes)
	}
}

func TestAuthAddCmd_ReadonlyWithDriveScopeFileRejected(t *testing.T) {
	err := Execute([]string{"auth", "add", "user@example.com", "--services", "drive", "--readonly", "--drive-scope", "file"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %#v", err, err)
	}
	if !strings.Contains(err.Error(), "--drive-scope=file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddCmd_SheetsReadonlyIncludesDriveReadonly(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"sheets",
				"--readonly",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/spreadsheets.readonly") {
		t.Fatalf("missing spreadsheets.readonly in %v", gotOpts.Scopes)
	}
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive.readonly") {
		t.Fatalf("missing drive.readonly in %v", gotOpts.Scopes)
	}
}

func TestAuthAddCmd_SheetsDriveScopeFile(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"sheets",
				"--drive-scope",
				"file",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive.file") {
		t.Fatalf("missing drive.file in %v", gotOpts.Scopes)
	}
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/spreadsheets") {
		t.Fatalf("missing spreadsheets in %v", gotOpts.Scopes)
	}
	if containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("unexpected drive in %v", gotOpts.Scopes)
	}
}

func TestAuthAddCmd_RemoteStep1_PrintsAuthURL(t *testing.T) {
	manualCalled := false
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail",
		"--readonly",
		"--remote", "--step", "1",
	}, func(context.Context, googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
		manualCalled = true
		return googleauth.ManualAuthURLResult{URL: "https://example.com/auth", StateReused: true}, nil
	})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !manualCalled {
		t.Fatalf("expected manual auth URL operation to be called")
	}
	if !strings.Contains(result.stdout, "auth_url\thttps://example.com/auth") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "state_reused\ttrue") {
		t.Fatalf("expected state_reused output, got: %q", result.stdout)
	}
	if !strings.Contains(result.stderr, "Run again with the same root flags and --remote --step 2 --auth-url <redirect-url> --services gmail --readonly") {
		t.Fatalf("expected step 2 guidance to preserve replay flags, got: %q", result.stderr)
	}
}

func TestAuthAddCmd_RemoteStep1_PreservesAllReplayFlags(t *testing.T) {
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail,drive",
		"--remote", "--step", "1",
		"--drive-scope", "file",
		"--gmail-scope", "readonly",
		"--force-consent",
	}, fixedManualAuthURL)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	want := "Run again with the same root flags and --remote --step 2 --auth-url <redirect-url> --services gmail,drive --drive-scope file --gmail-scope readonly --force-consent"
	if !strings.Contains(result.stderr, want) {
		t.Fatalf("expected replay guidance %q, got %q", want, result.stderr)
	}
}

func TestAuthAddCmd_RemoteStep1_OmitsDefaultScopeFlags(t *testing.T) {
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail,drive",
		"--remote", "--step", "1",
	}, fixedManualAuthURL)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if strings.Contains(result.stderr, "--drive-scope full") {
		t.Fatalf("expected default drive scope to be omitted, got %q", result.stderr)
	}
	if strings.Contains(result.stderr, "--gmail-scope full") {
		t.Fatalf("expected default gmail scope to be omitted, got %q", result.stderr)
	}
}

func TestAuthAddCmd_RemoteStep1_PassesRedirectURI(t *testing.T) {
	var gotOpts googleauth.AuthorizeOptions
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail",
		"--remote", "--step", "1",
		"--redirect-uri", "https://molty2.tail8108.ts.net/oauth2/callback",
	}, func(_ context.Context, opts googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
		gotOpts = opts
		return googleauth.ManualAuthURLResult{URL: "https://example.com/auth"}, nil
	})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if gotOpts.RedirectURI != "https://molty2.tail8108.ts.net/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", gotOpts.RedirectURI)
	}
}

func TestAuthAddCmd_RemoteStep1_ReplaysRedirectURIInGuidance(t *testing.T) {
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail",
		"--remote", "--step", "1",
		"--redirect-uri", "https://molty2.tail8108.ts.net/oauth2/callback",
	}, fixedManualAuthURL)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	want := "--remote --step 2 --auth-url <redirect-url> --redirect-uri https://molty2.tail8108.ts.net/oauth2/callback --services gmail"
	if !strings.Contains(result.stderr, want) {
		t.Fatalf("expected replay guidance %q, got %q", want, result.stderr)
	}
}

func TestAuthAddCmd_RemoteStep1_PassesRedirectHostAsRedirectURI(t *testing.T) {
	var gotOpts googleauth.AuthorizeOptions
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail",
		"--remote", "--step", "1",
		"--redirect-host", "gog.example.com",
	}, func(_ context.Context, opts googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
		gotOpts = opts
		return googleauth.ManualAuthURLResult{URL: "https://example.com/auth"}, nil
	})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if gotOpts.RedirectURI != "https://gog.example.com/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", gotOpts.RedirectURI)
	}
}

func executeWithManualAuthURL(t *testing.T, args []string, manualURL app.ManualAuthURLFunc) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{
		Auth: app.AuthOperations{
			ManualAuthURL: manualURL,
			AuthorizeGoogle: func(context.Context, googleauth.AuthorizeOptions) (string, error) {
				t.Fatal("AuthorizeGoogle should not be called in remote step 1")
				return "", nil
			},
			EnsureKeychainAccess: func(context.Context) error {
				t.Fatal("keychain access should not be checked in remote step 1")
				return nil
			},
		},
	})
}

func fixedManualAuthURL(context.Context, googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
	return googleauth.ManualAuthURLResult{URL: "https://example.com/auth"}, nil
}

func TestAuthAddCmd_BrowserFlow_PassesListenAddrAndRedirectHost(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(_ context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "refresh", nil
	}
	ensureKeychainAccess = func(context.Context) error { return nil }
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}
	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	if err := execute([]string{
		"auth", "add", "user@example.com",
		"--services", "gmail",
		"--listen-addr", "0.0.0.0:8080",
		"--redirect-host", "gog.example.com",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotOpts.ListenAddr != "0.0.0.0:8080" {
		t.Fatalf("unexpected listen addr: %q", gotOpts.ListenAddr)
	}
	if gotOpts.RedirectURI != "https://gog.example.com/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", gotOpts.RedirectURI)
	}
	if gotOpts.Manual {
		t.Fatalf("redirect-host should not force manual mode")
	}
}

func TestAuthAddCmd_RemoteStep1_ReplaysExtraScopesInGuidance(t *testing.T) {
	result := executeWithManualAuthURL(t, []string{
		"auth", "add", "user@example.com",
		"--services", "gmail",
		"--remote", "--step", "1",
		"--extra-scopes", "https://www.googleapis.com/auth/gmail.labels, https://www.googleapis.com/auth/gmail.insert",
	}, fixedManualAuthURL)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	want := "--remote --step 2 --auth-url <redirect-url> --services gmail --extra-scopes https://www.googleapis.com/auth/gmail.labels,https://www.googleapis.com/auth/gmail.insert"
	if !strings.Contains(result.stderr, want) {
		t.Fatalf("expected replay guidance %q, got %q", want, result.stderr)
	}
}

func TestAuthAddCmd_RemoteStep2_RejectsAuthCode(t *testing.T) {
	err := Execute([]string{
		"auth",
		"add",
		"user@example.com",
		"--services",
		"gmail",
		"--remote",
		"--step",
		"2",
		"--auth-code",
		"abc123",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %#v", err, err)
	}
	if !strings.Contains(err.Error(), "--auth-code is not valid with --remote") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddCmd_RemoteStep2_PassesAuthURL(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	if err := execute([]string{
		"auth",
		"add",
		"user@example.com",
		"--services",
		"gmail",
		"--remote",
		"--step",
		"2",
		"--auth-url",
		"http://127.0.0.1:55555/oauth2/callback?code=abc&state=state123",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !gotOpts.Manual {
		t.Fatalf("expected manual auth in remote step 2")
	}
	if !gotOpts.RequireState {
		t.Fatalf("expected require state in remote step 2")
	}
	if gotOpts.AuthURL == "" {
		t.Fatalf("expected auth URL to be passed through")
	}
}

func TestAuthAddCmd_RemoteStep2_PassesRedirectURI(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	if err := execute([]string{
		"auth",
		"add",
		"user@example.com",
		"--services",
		"gmail",
		"--remote",
		"--step",
		"2",
		"--redirect-uri",
		"https://molty2.tail8108.ts.net/oauth2/callback",
		"--auth-url",
		"https://molty2.tail8108.ts.net/oauth2/callback?code=abc&state=state123",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if gotOpts.RedirectURI != "https://molty2.tail8108.ts.net/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", gotOpts.RedirectURI)
	}
}

func TestAuthAddCmd_AuthCode_PassesThrough(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	if err := execute([]string{
		"auth",
		"add",
		"user@example.com",
		"--services",
		"gmail",
		"--auth-code",
		"abc123",
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !gotOpts.Manual {
		t.Fatalf("expected manual auth when auth-code is provided")
	}
	if gotOpts.AuthCode != "abc123" {
		t.Fatalf("expected auth-code to be passed through, got %q", gotOpts.AuthCode)
	}
}

func TestAuthAddCmd_ExtraScopes(t *testing.T) {
	openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity := defaultAuthTestOperations()
	execute := func(args []string) error {
		return executeWithRuntime(args, runtimeWithAuthTestOperations(
			openSecretsStore, authorizeGoogle, ensureKeychainAccess, fetchAuthorizedIdentity,
		))
	}

	ensureKeychainAccess = func(context.Context) error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedIdentity = func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
		return googleauth.Identity{Email: "user@example.com"}, nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{
				"--json",
				"auth",
				"add",
				"user@example.com",
				"--services",
				"gmail",
				"--gmail-scope",
				"readonly",
				"--extra-scopes",
				"https://www.googleapis.com/auth/gmail.labels,https://www.googleapis.com/auth/gmail.readonly",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// Extra scope should be present
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.labels") {
		t.Fatalf("missing gmail.labels in %v", gotOpts.Scopes)
	}
	// gmail.readonly from --gmail-scope=readonly should still be present
	if !containsStringInSlice(gotOpts.Scopes, "https://www.googleapis.com/auth/gmail.readonly") {
		t.Fatalf("missing gmail.readonly in %v", gotOpts.Scopes)
	}
	// Duplicate gmail.readonly (from extra-scopes) should be de-duplicated
	count := 0
	for _, s := range gotOpts.Scopes {
		if s == "https://www.googleapis.com/auth/gmail.readonly" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected gmail.readonly exactly once, got %d in %v", count, gotOpts.Scopes)
	}
}

func containsStringInSlice(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}
