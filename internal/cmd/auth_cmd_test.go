package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type memSecretsStore struct {
	tokens   map[string]secrets.Token
	defaults map[string]string
	secrets  map[string][]byte
}

func newMemSecretsStore() *memSecretsStore {
	return &memSecretsStore{
		tokens:   make(map[string]secrets.Token),
		defaults: make(map[string]string),
		secrets:  make(map[string][]byte),
	}
}

func normalizeEmailTest(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (s *memSecretsStore) Keys() ([]string, error) {
	keys := make([]string, 0, len(s.tokens))
	for key := range s.tokens {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		keys = append(keys, secrets.TokenKey(parts[0], parts[1]))
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *memSecretsStore) SetToken(client string, email string, tok secrets.Token) error {
	email = normalizeEmailTest(email)
	if email == "" {
		return errors.New("missing email")
	}
	if strings.TrimSpace(tok.RefreshToken) == "" {
		return errors.New("missing refresh token")
	}
	if client == "" {
		client = config.DefaultClientName
	}
	tok.Email = email
	tok.Client = client
	s.tokens[client+":"+email] = tok
	return nil
}

func (s *memSecretsStore) GetToken(client string, email string) (secrets.Token, error) {
	email = normalizeEmailTest(email)
	if email == "" {
		return secrets.Token{}, errors.New("missing email")
	}
	if client == "" {
		client = config.DefaultClientName
	}
	if tok, ok := s.tokens[client+":"+email]; ok {
		return tok, nil
	}
	return secrets.Token{}, keyring.ErrKeyNotFound
}

func (s *memSecretsStore) DeleteToken(client string, email string) error {
	email = normalizeEmailTest(email)
	if email == "" {
		return errors.New("missing email")
	}
	if client == "" {
		client = config.DefaultClientName
	}
	if _, ok := s.tokens[client+":"+email]; !ok {
		return keyring.ErrKeyNotFound
	}
	delete(s.tokens, client+":"+email)
	return nil
}

func (s *memSecretsStore) ListTokens() ([]secrets.Token, error) {
	out := make([]secrets.Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		out = append(out, t)
	}
	return out, nil
}

func (s *memSecretsStore) GetDefaultAccount(client string) (string, error) {
	if client == "" {
		client = config.DefaultClientName
	}
	return s.defaults[client], nil
}

func (s *memSecretsStore) SetDefaultAccount(client string, email string) error {
	if client == "" {
		client = config.DefaultClientName
	}
	s.defaults[client] = email
	return nil
}

func (s *memSecretsStore) SetSecret(key string, value []byte) error {
	s.secrets[key] = append([]byte(nil), value...)
	return nil
}

func (s *memSecretsStore) GetSecret(key string) ([]byte, error) {
	value, ok := s.secrets[key]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *memSecretsStore) DeleteSecret(key string) error {
	if _, ok := s.secrets[key]; !ok {
		return keyring.ErrKeyNotFound
	}
	delete(s.secrets, key)
	return nil
}

func TestAuthTokens_ExportImportRoundtrip_JSON(t *testing.T) {
	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)
	runtime.Auth.EnsureKeychainAccess = func(context.Context) error { return nil }
	execute := func(args []string) error { return executeWithRuntime(args, runtime) }
	createdAt := time.Date(2025, 12, 12, 0, 0, 0, 0, time.UTC)
	if err := store.SetToken(config.DefaultClientName, "A@B.COM", secrets.Token{
		Services:             []string{"gmail"},
		Scopes:               []string{"s1"},
		CreatedAt:            createdAt,
		RefreshToken:         "rt",
		AccessToken:          "at",
		AccessTokenExpiresAt: createdAt.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "token.json")

	stdout := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"--json", "auth", "tokens", "export", "a@b.com", "--out", outPath}); err != nil {
				t.Fatalf("Execute export: %v", err)
			}
		})
	})

	var exportResp struct {
		Exported bool   `json:"exported"`
		Email    string `json:"email"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &exportResp); err != nil {
		t.Fatalf("export json: %v\nout=%q", err, stdout)
	}
	if !exportResp.Exported || exportResp.Email != "a@b.com" || exportResp.Path != outPath {
		t.Fatalf("unexpected export resp: %#v", exportResp)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read outPath: %v", err)
	}
	if !strings.Contains(string(b), "\"refresh_token\"") {
		t.Fatalf("expected refresh_token in file: %q", string(b))
	}
	if !strings.Contains(string(b), "\"access_token\"") {
		t.Fatalf("expected access_token in file: %q", string(b))
	}

	// Clear token, then import it back.
	if err := store.DeleteToken(config.DefaultClientName, "a@b.com"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	importOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"--json", "auth", "tokens", "import", outPath}); err != nil {
				t.Fatalf("Execute import: %v", err)
			}
		})
	})
	var importResp struct {
		Imported bool   `json:"imported"`
		Email    string `json:"email"`
	}
	if err := json.Unmarshal([]byte(importOut), &importResp); err != nil {
		t.Fatalf("import json: %v\nout=%q", err, importOut)
	}
	if !importResp.Imported || importResp.Email != "a@b.com" {
		t.Fatalf("unexpected import resp: %#v", importResp)
	}
	if tok, err := store.GetToken(config.DefaultClientName, "a@b.com"); err != nil || tok.RefreshToken != "rt" || tok.AccessToken != "at" {
		t.Fatalf("expected token restored, got tok=%#v err=%v", tok, err)
	}
}

func TestAuthTokensList_FiltersNonTokenKeys(t *testing.T) {
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt"})
	_ = store.SetToken("org", "c@d.com", secrets.Token{RefreshToken: "rt2"})
	runtime := runtimeWithAuthStore(store)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "tokens", "list"}, runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "token:default:a@b.com") || !strings.Contains(out, "token:org:c@d.com") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAuthTokensList_ListsKeysWithoutDecrypting(t *testing.T) {
	runtime := runtimeWithAuthStore(&errorTokenStore{
		keys: []string{secrets.TokenKey(config.DefaultClientName, "a@b.com")},
		err:  errors.New("read token: aes.KeyUnwrap(): integrity check failed"),
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "tokens", "list"}, runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.TrimSpace(out) != "token:default:a@b.com" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAuthStatus_JSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "auth", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload struct {
		Config struct {
			Path   string `json:"path"`
			Exists bool   `json:"exists"`
		} `json:"config"`
		Keyring struct {
			Backend string `json:"backend"`
			Source  string `json:"source"`
		} `json:"keyring"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Keyring.Backend != "file" {
		t.Fatalf("unexpected backend: %q", payload.Keyring.Backend)
	}
	if payload.Keyring.Source != "env" {
		t.Fatalf("unexpected backend source: %q", payload.Keyring.Source)
	}
	if payload.Config.Path == "" {
		t.Fatalf("expected config path")
	}
}

func TestAuthStatus_Text_ConfigFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	os.Unsetenv("GOG_KEYRING_BACKEND")
	t.Cleanup(func() { os.Setenv("GOG_KEYRING_BACKEND", "file") })

	cfgPath := defaultConfigStoreForTest(t).Path()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{ keyring_backend: "file" }`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.Contains(out, "config_exists\ttrue") {
		t.Fatalf("expected config_exists true, got: %q", out)
	}
	if !strings.Contains(out, "keyring_backend\tfile") {
		t.Fatalf("expected keyring_backend file, got: %q", out)
	}
	if !strings.Contains(out, "keyring_backend_source\tconfig") {
		t.Fatalf("expected keyring_backend_source config, got: %q", out)
	}
}

type errorTokenStore struct {
	keys      []string
	err       error
	keysCalls int
	getErrs   []error
	getCalls  int
}

func (s *errorTokenStore) Keys() ([]string, error) {
	s.keysCalls++

	return s.keys, nil
}

func (s *errorTokenStore) SetToken(string, string, secrets.Token) error { return nil }

func (s *errorTokenStore) GetToken(string, string) (secrets.Token, error) {
	s.getCalls++
	if s.getCalls <= len(s.getErrs) {
		return secrets.Token{}, s.getErrs[s.getCalls-1]
	}
	return secrets.Token{}, s.err
}

func (s *errorTokenStore) DeleteToken(string, string) error { return nil }

func (s *errorTokenStore) ListTokens() ([]secrets.Token, error) { return nil, s.err }

func (s *errorTokenStore) GetDefaultAccount(string) (string, error) { return "", nil }

func (s *errorTokenStore) SetDefaultAccount(string, string) error { return nil }

func TestAuthListWithFallbackSkipsReadableFallbackOnKeyringTimeout(t *testing.T) {
	store := &errorTokenStore{
		keys: []string{secrets.TokenKey(config.DefaultClientName, "a@b.com")},
		err:  errors.New("list tokens: keyring connection timed out after 10s while reading keyring item"),
	}

	tokens, readErrors, err := listAuthTokensWithFallback(store)
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !secrets.IsKeyringTimeout(err) {
		t.Fatalf("expected keyring timeout, got %v", err)
	}

	if len(tokens) != 0 || len(readErrors) != 0 {
		t.Fatalf("expected no fallback results, got tokens=%#v readErrors=%#v", tokens, readErrors)
	}

	if store.keysCalls != 0 {
		t.Fatalf("fallback should not call Keys after timeout, got %d calls", store.keysCalls)
	}
}

func TestAuthListWithFallbackStopsReadableFallbackOnKeyringTimeout(t *testing.T) {
	store := &errorTokenStore{
		keys: []string{
			secrets.TokenKey(config.DefaultClientName, "broken@example.com"),
			secrets.TokenKey(config.DefaultClientName, "timeout@example.com"),
			secrets.TokenKey(config.DefaultClientName, "unread@example.com"),
		},
		err: errors.New("list tokens: malformed token"),
		getErrs: []error{
			errors.New("decode token: malformed payload"),
			errors.New("keyring connection timed out after 10s while reading keyring item"),
			errors.New("must not be read"),
		},
	}

	tokens, readErrors, err := listAuthTokensWithFallback(store)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !secrets.IsKeyringTimeout(err) {
		t.Fatalf("expected keyring timeout, got %v", err)
	}
	if len(tokens) != 0 || len(readErrors) != 0 {
		t.Fatalf("expected no partial fallback results, got tokens=%#v readErrors=%#v", tokens, readErrors)
	}
	if store.getCalls != 2 {
		t.Fatalf("fallback should stop after timeout, got %d token reads", store.getCalls)
	}
}

func TestAuthDoctor_JSON_ClassifiesFileKeyringIntegrity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "pw")

	runtime := runtimeWithAuthStore(&errorTokenStore{
		keys: []string{secrets.TokenKey(config.DefaultClientName, "a@b.com")},
		err:  errors.New("read token: aes.KeyUnwrap(): integrity check failed"),
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "auth", "doctor"}, runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Hint   string `json:"hint"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if payload.Status != "error" {
		t.Fatalf("status=%q, want error", payload.Status)
	}
	found := false
	for _, check := range payload.Checks {
		if check.Name == "token.default.a@b.com" && check.Status == "error" && strings.Contains(check.Hint, "password mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing classified token error: %#v", payload.Checks)
	}
}

func TestAuthDoctor_JSON_ReportsForcedKeychainTrust(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runtime := runtimeWithAuthStore(newMemSecretsStore())
	runtime.KeyringOptions.Backend = "keychain"
	runtime.KeyringOptions.GOOS = "darwin"
	runtime.KeyringOptions.KeychainTrustApplication = "true"

	result := executeWithTestRuntime(t, []string{"--json", "auth", "doctor"}, runtime)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var payload struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	for _, check := range payload.Checks {
		if check.Name == "keychain.trust" {
			if check.Status != "ok" || check.Detail != "forced on via GOG_KEYCHAIN_TRUST_APPLICATION" {
				t.Fatalf("keychain trust check = %#v", check)
			}
			return
		}
	}
	t.Fatalf("missing keychain.trust check: %#v", payload.Checks)
}

func TestAuthList_JSON_ReportsUnreadableToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runtime := runtimeWithAuthStore(&errorTokenStore{
		keys: []string{secrets.TokenKey(config.DefaultClientName, "a@b.com")},
		err:  errors.New("read token: aes.KeyUnwrap(): integrity check failed"),
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "auth", "list"}, runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var payload struct {
		Accounts []struct {
			Email  string `json:"email"`
			Client string `json:"client"`
			Auth   string `json:"auth"`
			Error  string `json:"error"`
			Hint   string `json:"hint"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(payload.Accounts) != 1 {
		t.Fatalf("accounts=%#v, want one unreadable token row", payload.Accounts)
	}
	account := payload.Accounts[0]
	if account.Email != "a@b.com" || account.Client != config.DefaultClientName || account.Auth != authTypeOAuth {
		t.Fatalf("unexpected account row: %#v", account)
	}
	if !strings.Contains(account.Error, "integrity check failed") || !strings.Contains(account.Hint, "password mismatch") {
		t.Fatalf("missing classified unreadable-token details: %#v", account)
	}
}

func TestAuthDoctor_JSON_CheckClassifiesInvalidRAPT(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")

	store := newMemSecretsStore()
	if err := store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{
		RefreshToken: "rt",
		Scopes:       []string{"scope"},
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	result := executeWithTestRuntime(t, []string{"--json", "auth", "doctor", "--check"}, &app.Runtime{
		Auth: app.AuthOperations{
			OpenSecretsStore: func() (secrets.Store, error) { return store, nil },
			CheckRefreshToken: func(context.Context, string, string, []string, time.Duration) error {
				return errors.New(`oauth2: "invalid_grant" "reauth related error (invalid_rapt)"`)
			},
		},
	})
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	var payload struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Hint   string `json:"hint"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if payload.Status != "error" {
		t.Fatalf("status=%q, want error", payload.Status)
	}
	found := false
	for _, check := range payload.Checks {
		if check.Name == "refresh.default.a@b.com" && check.Status == "error" && strings.Contains(check.Hint, "service-account") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing invalid_rapt hint: %#v", payload.Checks)
	}
}

func TestAuthTokensExport_RequiresOut(t *testing.T) {
	err := Execute([]string{"--json", "auth", "tokens", "export", "a@b.com"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "empty outPath") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthTokensImport_NoInput(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")
	runtime := &app.Runtime{Auth: app.AuthOperations{
		EnsureKeychainAccess: func(context.Context) error { return errors.New("keychain locked") },
	}}

	outPath := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(outPath, []byte(`{"email":"a@b.com","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	err := executeWithRuntime([]string{"--json", "--no-input", "auth", "tokens", "import", outPath}, runtime)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "keychain access") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthTokensImport_FileBackendSkipsKeychain(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)
	runtime.Auth.EnsureKeychainAccess = func(context.Context) error { return errors.New("keychain locked") }

	outPath := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(outPath, []byte(`{"email":"a@b.com","refresh_token":"rt"}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	if err := executeWithRuntime([]string{"--json", "auth", "tokens", "import", outPath}, runtime); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if _, err := store.GetToken(config.DefaultClientName, "a@b.com"); err != nil {
		t.Fatalf("expected token stored: %v", err)
	}
}

func TestAuthListRemoveTokensListDelete_JSON(t *testing.T) {
	store := newMemSecretsStore()

	runtime := &app.Runtime{
		Auth: app.AuthOperations{
			OpenSecretsStore: func() (secrets.Store, error) { return store, nil },
			CheckRefreshToken: func(_ context.Context, _ string, refreshToken string, _ []string, _ time.Duration) error {
				if refreshToken == "rt2" {
					return errors.New("invalid_grant")
				}
				return nil
			},
		},
	}

	_ = store.SetToken(config.DefaultClientName, "b@b.com", secrets.Token{RefreshToken: "rt2"})
	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt1"})

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "auth", "list"}, runtime); err != nil {
				t.Fatalf("Execute list: %v", err)
			}
		})
	})
	var listResp struct {
		Accounts []struct {
			Email string `json:"email"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(listOut), &listResp); err != nil {
		t.Fatalf("list json: %v\nout=%q", err, listOut)
	}
	if len(listResp.Accounts) != 2 || listResp.Accounts[0].Email != "a@b.com" || listResp.Accounts[1].Email != "b@b.com" {
		t.Fatalf("unexpected accounts: %#v", listResp.Accounts)
	}

	listChecked := executeWithTestRuntime(t, []string{"--json", "auth", "list", "--check"}, runtime)
	if listChecked.err != nil {
		t.Fatalf("Execute list --check: %v", listChecked.err)
	}
	var listCheckedResp struct {
		Accounts []struct {
			Email string `json:"email"`
			Valid *bool  `json:"valid,omitempty"`
			Error string `json:"error,omitempty"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(listChecked.stdout), &listCheckedResp); err != nil {
		t.Fatalf("list --check json: %v\nout=%q", err, listChecked.stdout)
	}
	if len(listCheckedResp.Accounts) != 2 {
		t.Fatalf("unexpected accounts: %#v", listCheckedResp.Accounts)
	}
	if listCheckedResp.Accounts[0].Email != "a@b.com" || listCheckedResp.Accounts[0].Valid == nil || !*listCheckedResp.Accounts[0].Valid {
		t.Fatalf("expected a@b.com valid, got: %#v", listCheckedResp.Accounts[0])
	}
	if listCheckedResp.Accounts[1].Email != "b@b.com" || listCheckedResp.Accounts[1].Valid == nil || *listCheckedResp.Accounts[1].Valid || !strings.Contains(listCheckedResp.Accounts[1].Error, "invalid_grant") {
		t.Fatalf("expected b@b.com invalid_grant, got: %#v", listCheckedResp.Accounts[1])
	}

	// Tokens list (keys).
	keysOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "auth", "tokens", "list"}, runtime); err != nil {
				t.Fatalf("Execute tokens list: %v", err)
			}
		})
	})
	var keysResp struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(keysOut), &keysResp); err != nil {
		t.Fatalf("keys json: %v\nout=%q", err, keysOut)
	}
	if len(keysResp.Keys) != 2 {
		t.Fatalf("unexpected keys: %#v", keysResp.Keys)
	}

	// Remove (auth remove)
	rmOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "--force", "auth", "remove", "b@b.com"}, runtime); err != nil {
				t.Fatalf("Execute remove: %v", err)
			}
		})
	})
	var rmResp struct {
		Deleted bool   `json:"deleted"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal([]byte(rmOut), &rmResp); err != nil {
		t.Fatalf("remove json: %v\nout=%q", err, rmOut)
	}
	if !rmResp.Deleted || rmResp.Email != "b@b.com" {
		t.Fatalf("unexpected remove resp: %#v", rmResp)
	}

	// Tokens delete (auth tokens delete)
	delOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "--force", "auth", "tokens", "delete", "a@b.com"}, runtime); err != nil {
				t.Fatalf("Execute tokens delete: %v", err)
			}
		})
	})
	var delResp struct {
		Deleted bool   `json:"deleted"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal([]byte(delOut), &delResp); err != nil {
		t.Fatalf("delete json: %v\nout=%q", err, delOut)
	}
	if !delResp.Deleted || delResp.Email != "a@b.com" {
		t.Fatalf("unexpected delete resp: %#v", delResp)
	}

	// Now empty.
	emptyKeysOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "auth", "tokens", "list"}, runtime); err != nil {
				t.Fatalf("Execute tokens list: %v", err)
			}
		})
	})
	var emptyKeysResp struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal([]byte(emptyKeysOut), &emptyKeysResp); err != nil {
		t.Fatalf("empty keys json: %v\nout=%q", err, emptyKeysOut)
	}
	if len(emptyKeysResp.Keys) != 0 {
		t.Fatalf("expected empty keys, got: %#v", emptyKeysResp.Keys)
	}
}

func TestAuthRemove_CleansUpConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	store := newMemSecretsStore()
	_ = store.SetToken("custom-client", "remove@example.com", secrets.Token{RefreshToken: "rt-remove"})
	runtime := runtimeWithAuthStore(store)

	// Write config with alias and client entries for the email we will remove.
	cfg := config.File{
		AccountAliases: map[string]string{
			"work": "remove@example.com",
			"keep": "other@example.com",
		},
		AccountClients: map[string]string{
			"remove@example.com": "custom-client",
			"other@example.com":  "default",
		},
	}
	configStore := defaultConfigStoreForTest(t)
	if err := configStore.Write(cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	// Run auth remove.
	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--json", "--force", "auth", "remove", "remove@example.com"}, runtime); err != nil {
				t.Fatalf("Execute remove: %v", err)
			}
		})
	})

	// Verify config was cleaned up.
	updated, err := configStore.Read()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if _, ok := updated.AccountAliases["work"]; ok {
		t.Fatalf("expected alias 'work' to be removed, but it still exists")
	}
	if v, ok := updated.AccountAliases["keep"]; !ok || v != "other@example.com" {
		t.Fatalf("expected alias 'keep' to be preserved, got: %v", updated.AccountAliases)
	}
	if _, ok := updated.AccountClients["remove@example.com"]; ok {
		t.Fatalf("expected account_clients entry for remove@example.com to be removed")
	}
	if v, ok := updated.AccountClients["other@example.com"]; !ok || v != "default" {
		t.Fatalf("expected account_clients entry for other@example.com to be preserved, got: %v", updated.AccountClients)
	}
}
