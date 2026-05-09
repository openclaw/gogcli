package googleapi

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/authclient"
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

	setClient string
	setEmail  string
	lastSet   secrets.Token
	setCalls  int
	setErr    error

	deleteClient string
	deleteEmail  string
	deleteCalls  int
	deleteErr    error
}

func (s *stubStore) Keys() ([]string, error) { return nil, nil }
func (s *stubStore) SetToken(client string, email string, tok secrets.Token) error {
	s.setClient = client
	s.setEmail = email
	s.lastSet = tok
	s.setCalls++

	if s.setErr != nil {
		return s.setErr
	}

	s.tok = tok

	return nil
}

func (s *stubStore) DeleteToken(client string, email string) error {
	s.deleteClient = client
	s.deleteEmail = email
	s.deleteCalls++

	if s.deleteErr != nil {
		return s.deleteErr
	}

	return nil
}

func (s *stubStore) DeleteTokenAlias(client string, email string) error {
	return s.DeleteToken(client, email)
}

func (s *stubStore) ListTokens() ([]secrets.Token, error)     { return nil, nil }
func (s *stubStore) GetDefaultAccount(string) (string, error) { return "", nil }
func (s *stubStore) SetDefaultAccount(string, string) error   { return nil }
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

func TestPersistingTokenSource_PersistsRotatedRefreshToken(t *testing.T) {
	stored := secrets.Token{
		Client:       config.DefaultClientName,
		Email:        "a@b.com",
		RefreshToken: "old-refresh-token",
		Services:     []string{"gmail"},
		Scopes:       []string{"s1"},
		CreatedAt:    time.Unix(1735689600, 0).UTC(),
	}

	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", RefreshToken: "new-refresh-token"})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "A@B.COM", stored)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.setClient != config.DefaultClientName {
		t.Fatalf("unexpected client: %q", store.setClient)
	}

	if store.setEmail != "A@B.COM" {
		t.Fatalf("unexpected email: %q", store.setEmail)
	}

	if store.lastSet.RefreshToken != "new-refresh-token" {
		t.Fatalf("expected rotated refresh token to persist, got %q", store.lastSet.RefreshToken)
	}

	if !reflect.DeepEqual(store.lastSet.Services, stored.Services) {
		t.Fatalf("services changed unexpectedly: %#v", store.lastSet.Services)
	}

	if !reflect.DeepEqual(store.lastSet.Scopes, stored.Scopes) {
		t.Fatalf("scopes changed unexpectedly: %#v", store.lastSet.Scopes)
	}

	if !store.lastSet.CreatedAt.Equal(stored.CreatedAt) {
		t.Fatalf("createdAt changed unexpectedly: %v", store.lastSet.CreatedAt)
	}
}

func TestPersistingTokenSource_NoRotationDoesNotPersist(t *testing.T) {
	stored := secrets.Token{Email: "a@b.com", RefreshToken: "same-token"}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", RefreshToken: "same-token"})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 0 {
		t.Fatalf("expected no SetToken calls, got %d", store.setCalls)
	}
}

func TestPersistingTokenSource_BackfillsSubjectFromIDToken(t *testing.T) {
	stored := secrets.Token{Email: "a@b.com", RefreshToken: "same-token"}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"id_token": unsignedIDTokenForTest(t, "sub-123", "a@b.com"),
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.setEmail != "a@b.com" {
		t.Fatalf("unexpected email: %q", store.setEmail)
	}

	if store.lastSet.Subject != "sub-123" {
		t.Fatalf("expected subject to persist, got %q", store.lastSet.Subject)
	}

	if store.lastSet.RefreshToken != "same-token" {
		t.Fatalf("refresh token changed unexpectedly: %q", store.lastSet.RefreshToken)
	}
}

func TestPersistingTokenSource_MigratesRenamedEmailFromIDToken(t *testing.T) {
	stored := secrets.Token{Email: "old@example.com", RefreshToken: "same-token", Subject: "sub-123"}
	store := &stubStore{tok: stored}
	base := oauth2.StaticTokenSource((&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "same-token",
	}).WithExtra(map[string]any{
		"id_token": unsignedIDTokenForTest(t, "sub-123", "new@example.com"),
	}))
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "old@example.com", stored)

	if _, err := ts.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken call, got %d", store.setCalls)
	}

	if store.setEmail != "new@example.com" {
		t.Fatalf("expected new email to persist, got %q", store.setEmail)
	}

	if store.lastSet.Email != "new@example.com" || store.lastSet.Subject != "sub-123" {
		t.Fatalf("unexpected stored token: %#v", store.lastSet)
	}

	if store.deleteCalls != 1 || store.deleteClient != config.DefaultClientName || store.deleteEmail != "old@example.com" {
		t.Fatalf("expected old alias delete, got calls=%d client=%q email=%q", store.deleteCalls, store.deleteClient, store.deleteEmail)
	}

	pts, ok := ts.(*persistingTokenSource)
	if !ok {
		t.Fatalf("expected persistingTokenSource")
	}

	if pts.email != "new@example.com" {
		t.Fatalf("expected source email updated, got %q", pts.email)
	}
}

func TestPersistingTokenSource_PersistFailureIsNonFatal(t *testing.T) {
	stored := secrets.Token{Email: "a@b.com", RefreshToken: "old-token"}
	store := &stubStore{tok: stored, setErr: errBoom}
	base := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access", RefreshToken: "new-token"})
	ts := newPersistingTokenSource(base, store, config.DefaultClientName, "a@b.com", stored)

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}

	if tok.AccessToken != "access" {
		t.Fatalf("unexpected access token: %q", tok.AccessToken)
	}

	if store.setCalls != 1 {
		t.Fatalf("expected 1 SetToken attempt, got %d", store.setCalls)
	}

	if store.tok.RefreshToken != "old-token" {
		t.Fatalf("store should keep old token on persist error, got %q", store.tok.RefreshToken)
	}
}

func unsignedIDTokenForTest(t *testing.T, subject string, email string) string {
	t.Helper()

	header, err := json.Marshal(map[string]string{"alg": "none"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	payload, err := json.Marshal(map[string]string{"sub": subject, "email": email})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
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

func TestOptionsForAccountScopes_AccessTokenBypassesStoredAuth(t *testing.T) {
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		t.Fatal("readClientCredentials should not be called when access token is provided")
		return config.ClientCredentials{}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		t.Fatal("openSecretsStore should not be called when access token is provided")
		return nil, errBoom
	}

	ctx := authclient.WithAccessToken(context.Background(), "ya29.test-access-token")

	opts, err := optionsForAccountScopes(ctx, "svc", "a@b.com", []string{"s1"})
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

func TestIsADCMode(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "")

	if IsADCMode() {
		t.Fatalf("expected false when GOG_AUTH_MODE is empty")
	}

	t.Setenv("GOG_AUTH_MODE", "adc")

	if !IsADCMode() {
		t.Fatalf("expected true when GOG_AUTH_MODE=adc")
	}

	t.Setenv("GOG_AUTH_MODE", "oauth")

	if IsADCMode() {
		t.Fatalf("expected false when GOG_AUTH_MODE=oauth")
	}
}

func TestOptionsForAccountScopes_ADCMode(t *testing.T) {
	t.Setenv("GOG_AUTH_MODE", "adc")

	origADC := newADCTokenSource
	origRead := readClientCredentials
	origOpen := openSecretsStore

	t.Cleanup(func() {
		newADCTokenSource = origADC
		readClientCredentials = origRead
		openSecretsStore = origOpen
	})

	called := false
	newADCTokenSource = func(_ context.Context, scopes ...string) (oauth2.TokenSource, error) {
		called = true

		if len(scopes) != 1 || scopes[0] != "https://www.googleapis.com/auth/spreadsheets.readonly" {
			t.Fatalf("unexpected scopes: %v", scopes)
		}

		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}), nil
	}

	// Should NOT call keyring or readClientCredentials.
	readClientCredentials = func(string) (config.ClientCredentials, error) {
		t.Fatalf("readClientCredentials should not be called in ADC mode")
		return config.ClientCredentials{}, nil
	}
	openSecretsStore = func() (secrets.Store, error) {
		t.Fatalf("openSecretsStore should not be called in ADC mode")
		return nil, errBoom
	}

	opts, err := optionsForAccountScopes(context.Background(), "sheets", "adc", []string{
		"https://www.googleapis.com/auth/spreadsheets.readonly",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if !called {
		t.Fatalf("expected ADC token source to be called")
	}

	if len(opts) == 0 {
		t.Fatalf("expected client options")
	}
}

func TestNewBaseTransport_RespectsProxyAndTLSMinimum(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8888")

	transport := newBaseTransport()
	if transport == nil {
		t.Fatalf("expected transport")
		return
	}

	if transport.Proxy == nil {
		t.Fatalf("expected proxy func")
	}

	if transport.TLSClientConfig == nil {
		t.Fatalf("expected TLS config")
	}

	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("expected TLS min version >= 1.2, got %d", transport.TLSClientConfig.MinVersion)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://www.googleapis.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("proxy lookup: %v", err)
	}

	if proxyURL == nil || !strings.Contains(proxyURL.String(), "127.0.0.1:8888") {
		t.Fatalf("expected HTTPS proxy to be honored, got: %v", proxyURL)
	}
}

func TestNewBaseTransport_SetsResponseHeaderTimeout(t *testing.T) {
	transport := newBaseTransport()
	if transport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Fatalf("expected ResponseHeaderTimeout=%v, got %v", responseHeaderTimeout, transport.ResponseHeaderTimeout)
	}
}

func TestOptionsForAccountScopes_NoClientTimeout(t *testing.T) {
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

	// The http.Client returned by optionsForAccountScopes must not set a
	// hard Timeout so that large file downloads (Drive videos, etc.) are
	// not interrupted. Server responsiveness is instead guarded by the
	// transport-level ResponseHeaderTimeout.
	//
	// We cannot easily extract the http.Client from option.ClientOption,
	// so we verify the transport layer instead.
	transport := newBaseTransport()
	if transport.ResponseHeaderTimeout == 0 {
		t.Fatalf("expected ResponseHeaderTimeout to be set on transport")
	}
}

func TestNewHTTPClient_NoRedirectPolicy(t *testing.T) {
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

	client, err := NewHTTPClient(context.Background(), googleauth.ServiceDocs, "a@b.com")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}

	if client.Transport == nil {
		t.Fatal("expected Transport to be set on client")
	}

	if client.CheckRedirect != nil {
		t.Fatal("expected no CheckRedirect on bare client")
	}
}
