package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

func TestAuthAddCmd_JSON(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
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
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	// Simulate keychain locked error
	ensureKeychainAccess = func() error {
		return errors.New("keychain is locked")
	}

	authCalled := false
	authorizeGoogle = func(_ context.Context, _ googleauth.AuthorizeOptions) (string, error) {
		authCalled = true
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		t.Fatal("fetchAuthorizedEmail should not be called when keychain check fails")
		return "", nil
	}

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	cmd := &AuthAddCmd{Email: "test@example.com", ServicesCSV: "gmail"}
	err := cmd.Run(context.Background(), &RootFlags{})

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

func TestAuthAddCmd_DefaultServices_UserPreset(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "add", "user@example.com"}); err != nil {
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
	origAuth := authorizeGoogle
	t.Cleanup(func() { authorizeGoogle = origAuth })

	authorizeCalled := false
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		authorizeCalled = true
		return "", nil
	}

	err := Execute([]string{"auth", "add", "user@example.com", "--services", "keep"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected exit code 2, got %T %#v", err, err)
	}
	if !strings.Contains(err.Error(), "Keep auth") {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorizeCalled {
		t.Fatalf("authorizeGoogle should not be called")
	}
}

func TestAuthAddCmd_EmailMismatch(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "actual@example.com", nil
	}

	err := Execute([]string{"auth", "add", "expected@example.com"})
	if err == nil {
		t.Fatalf("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "authorized as actual@example.com") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddCmd_ReadonlyScopes(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
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

func TestAuthAddCmd_DriveScopeFile(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
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
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
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
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		gotOpts.Services = append([]googleauth.Service(nil), opts.Services...)
		gotOpts.Scopes = append([]string(nil), opts.Scopes...)
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
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
	origManualURL := manualAuthURL
	origAuth := authorizeGoogle
	origKeychain := ensureKeychainAccess
	t.Cleanup(func() {
		manualAuthURL = origManualURL
		authorizeGoogle = origAuth
		ensureKeychainAccess = origKeychain
	})

	manualCalled := false
	manualAuthURL = func(context.Context, googleauth.AuthorizeOptions) (googleauth.ManualAuthURLResult, error) {
		manualCalled = true
		return googleauth.ManualAuthURLResult{
			URL:         "https://example.com/auth",
			StateReused: true,
		}, nil
	}
	authorizeGoogle = func(context.Context, googleauth.AuthorizeOptions) (string, error) {
		t.Fatal("authorizeGoogle should not be called in remote step 1")
		return "", nil
	}
	ensureKeychainAccess = func() error {
		t.Fatal("keychain access should not be checked in remote step 1")
		return nil
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"auth",
				"add",
				"user@example.com",
				"--services",
				"gmail",
				"--remote",
				"--step",
				"1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !manualCalled {
		t.Fatalf("expected manualAuthURL to be called")
	}
	if !strings.Contains(out, "auth_url\thttps://example.com/auth") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "state_reused\ttrue") {
		t.Fatalf("expected state_reused output, got: %q", out)
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
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	if err := Execute([]string{
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

func TestAuthAddCmd_AuthCode_PassesThrough(t *testing.T) {
	origAuth := authorizeGoogle
	origOpen := openSecretsStore
	origKeychain := ensureKeychainAccess
	origFetch := fetchAuthorizedEmail
	t.Cleanup(func() {
		authorizeGoogle = origAuth
		openSecretsStore = origOpen
		ensureKeychainAccess = origKeychain
		fetchAuthorizedEmail = origFetch
	})

	ensureKeychainAccess = func() error { return nil }
	openSecretsStore = func() (secrets.Store, error) { return newMemSecretsStore(), nil }

	var gotOpts googleauth.AuthorizeOptions
	authorizeGoogle = func(ctx context.Context, opts googleauth.AuthorizeOptions) (string, error) {
		gotOpts = opts
		return "rt", nil
	}
	fetchAuthorizedEmail = func(context.Context, string, string, []string, time.Duration) (string, error) {
		return "user@example.com", nil
	}

	if err := Execute([]string{
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

func containsStringInSlice(items []string, want string) bool {
	for _, it := range items {
		if it == want {
			return true
		}
	}
	return false
}
