package googleapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

var (
	errBoom         = errors.New("boom")
	errNope         = errors.New("nope")
	errMissingCreds = errors.New("missing creds")
)

type stubStore struct {
	lastClient string
	lastEmail  string
	tok        secrets.Token
	err        error
}

func (s *stubStore) Keys() ([]string, error)                      { return nil, nil }
func (s *stubStore) SetToken(string, string, secrets.Token) error { return nil }
func (s *stubStore) DeleteToken(string, string) error             { return nil }
func (s *stubStore) ListTokens() ([]secrets.Token, error)         { return nil, nil }
func (s *stubStore) GetDefaultAccount(string) (string, error)     { return "", nil }
func (s *stubStore) SetDefaultAccount(string, string) error       { return nil }
func (s *stubStore) GetToken(client string, email string) (secrets.Token, error) {
	s.lastClient = client
	s.lastEmail = email

	if s.err != nil {
		return secrets.Token{}, s.err
	}

	return s.tok, nil
}

func TestTokenSourceForAccountScopes_StoreErrors(t *testing.T) {
	origOpen := openSecretsStore

	t.Cleanup(func() { openSecretsStore = origOpen })

	openSecretsStore = func() (secrets.Store, error) {
		return nil, errBoom
	}

	_, err := tokenSourceForAccountScopes(context.Background(), "svc", "a@b.com", "default", "id", "secret", []string{"s1"})
	if err == nil || !errors.Is(err, errBoom) {
		t.Fatalf("expected boom, got: %v", err)
	}
}

func TestTokenSourceForAccountScopes_KeyNotFound(t *testing.T) {
	origOpen := openSecretsStore

	t.Cleanup(func() { openSecretsStore = origOpen })

	openSecretsStore = func() (secrets.Store, error) {
		return &stubStore{err: keyring.ErrKeyNotFound}, nil
	}

	_, err := tokenSourceForAccountScopes(context.Background(), "gmail", "a@b.com", "default", "id", "secret", []string{"s1"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var are *AuthRequiredError

	if !errors.As(err, &are) {
		t.Fatalf("expected AuthRequiredError, got: %T %v", err, err)
	}

	if are.Service != "gmail" || are.Email != "a@b.com" {
		t.Fatalf("unexpected: %#v", are)
	}
}

func TestTokenSourceForAccountScopes_OtherGetError(t *testing.T) {
	origOpen := openSecretsStore

	t.Cleanup(func() { openSecretsStore = origOpen })

	openSecretsStore = func() (secrets.Store, error) {
		return &stubStore{err: errNope}, nil
	}

	_, err := tokenSourceForAccountScopes(context.Background(), "svc", "a@b.com", "default", "id", "secret", []string{"s1"})
	if err == nil || !errors.Is(err, errNope) {
		t.Fatalf("expected nope, got: %v", err)
	}
}

func TestTokenSourceForAccountScopes_HappyPath(t *testing.T) {
	origOpen := openSecretsStore

	t.Cleanup(func() { openSecretsStore = origOpen })

	s := &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}
	openSecretsStore = func() (secrets.Store, error) { return s, nil }

	ts, err := tokenSourceForAccountScopes(context.Background(), "svc", "A@B.COM", "default", "id", "secret", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if ts == nil {
		t.Fatalf("expected token source")
	}
	// Ensure we pass through the email (store normalizes in production).
	if s.lastEmail != "A@B.COM" {
		t.Fatalf("expected email passed through, got: %q", s.lastEmail)
	}
}

func TestTokenSourceForAccount_ReadCredsError(t *testing.T) {
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{}, errMissingCreds
	}

	_, err := tokenSourceForAccount(context.Background(), googleauth.ServiceGmail, "a@b.com")
	if err == nil || !errors.Is(err, errMissingCreds) {
		t.Fatalf("expected missing creds, got: %v", err)
	}
}

func TestOptionsForAccountScopes_HappyPath(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		return &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}, nil
	}

	opts, err := optionsForAccountScopes(context.Background(), "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestOptionsForAccount_HappyPath(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		return &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}, nil
	}

	opts, err := optionsForAccount(context.Background(), googleauth.ServiceDrive, "a@b.com")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestOptionsForAccountScopes_ServiceAccountPreferred(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	saPath, err := config.ServiceAccountPath("a@b.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}

	if _, ensureErr := config.EnsureDir(); ensureErr != nil {
		t.Fatalf("EnsureDir: %v", ensureErr)
	}

	if writeErr := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write sa: %v", writeErr)
	}

	origRead := readClientCredentials
	origOpen := openSecretsStore
	origSA := newServiceAccountTokenSource

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
		newServiceAccountTokenSource = origSA
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		t.Fatalf("readClientCredentials should not be called")
		return config.ClientCredentials{}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		t.Fatalf("openSecretsStore should not be called")
		return nil, errBoom
	}

	called := false
	newServiceAccountTokenSource = func(_ context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
		called = true

		if subject != "a@b.com" {
			t.Fatalf("unexpected subject: %q", subject)
		}

		if len(scopes) != 1 || scopes[0] != "s1" {
			t.Fatalf("unexpected scopes: %#v", scopes)
		}

		if string(keyJSON) == "" {
			t.Fatalf("expected keyJSON")
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	}

	opts, err := optionsForAccountScopes(context.Background(), "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !called {
		t.Fatalf("expected service account token source used")
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func extractHTTPClientFromOption(t *testing.T, opt any) *http.Client {
	t.Helper()

	apply := reflect.ValueOf(opt).MethodByName("Apply")
	if !apply.IsValid() {
		t.Fatalf("option does not implement Apply: %T", opt)
	}

	if apply.Type().NumIn() != 1 {
		t.Fatalf("unexpected Apply signature for %T", opt)
	}

	settingsArg := apply.Type().In(0)
	if settingsArg.Kind() != reflect.Pointer {
		t.Fatalf("unexpected Apply argument kind for %T: %s", opt, settingsArg.Kind())
	}

	settings := reflect.New(settingsArg.Elem())
	apply.Call([]reflect.Value{settings})

	httpClientField := settings.Elem().FieldByName("HTTPClient")
	if !httpClientField.IsValid() {
		t.Fatalf("dial settings missing HTTPClient field")
	}

	if httpClientField.IsNil() {
		t.Fatalf("HTTPClient was not set by option %T", opt)
	}

	httpClient, ok := httpClientField.Interface().(*http.Client)
	if !ok {
		t.Fatalf("HTTPClient field has unexpected type: %T", httpClientField.Interface())
	}

	return httpClient
}

func TestOptionsForAccountScopes_ConfiguresProxyOnCustomBaseTransport(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		return &stubStore{tok: secrets.Token{Email: "a@b.com", RefreshToken: "rt"}}, nil
	}

	opts, err := optionsForAccountScopes(context.Background(), "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}

	httpClient := extractHTTPClientFromOption(t, opts[0])

	retryTransport, ok := httpClient.Transport.(*RetryTransport)
	if !ok {
		t.Fatalf("expected RetryTransport, got %T", httpClient.Transport)
	}

	oauthTransport, ok := retryTransport.Base.(*oauth2.Transport)
	if !ok {
		t.Fatalf("expected oauth2.Transport, got %T", retryTransport.Base)
	}

	baseTransport, ok := oauthTransport.Base.(*http.Transport)
	if !ok {
		t.Fatalf("expected http.Transport, got %T", oauthTransport.Base)
	}

	if baseTransport.Proxy == nil {
		t.Fatalf("expected base transport proxy function")
	}

	if got, want := reflect.ValueOf(baseTransport.Proxy).Pointer(), reflect.ValueOf(http.ProxyFromEnvironment).Pointer(); got != want {
		t.Fatalf("unexpected proxy function: got %v want %v", got, want)
	}
}
