package googleauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

var (
	errBoom               = errors.New("boom")
	errBrowserUnavailable = errors.New("browser unavailable")
)

func newTestManagerApplication(
	t *testing.T,
	opts ManagerOptions,
	deps ManagerDependencies,
) *ManagerApplication {
	t.Helper()

	if deps.Tokens == nil {
		deps.Tokens = &fakeStore{}
	}

	if store, ok := deps.Tokens.(*fakeStore); ok {
		for i := range store.tokens {
			if store.tokens[i].Client == "" {
				store.tokens[i].Client = config.DefaultClientName
			}
		}
	}

	if deps.ReadCredentials == nil {
		deps.ReadCredentials = func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		}
	}

	if deps.UpdateEmailReferences == nil {
		deps.UpdateEmailReferences = func(string, string) error { return nil }
	}

	if deps.FetchIdentity == nil {
		deps.FetchIdentity = func(context.Context, *oauth2.Token) (Identity, error) {
			return Identity{Email: "me@example.com"}, nil
		}
	}

	if deps.EnsureKeychainAccess == nil {
		deps.EnsureKeychainAccess = func(context.Context) error { return nil }
	}

	if deps.Random == nil {
		deps.Random = bytes.NewReader(make([]byte, 4096))
	}

	if deps.OAuthEndpoint.AuthURL == "" {
		deps.OAuthEndpoint = oauth2.Endpoint{
			AuthURL:  "https://example.com/auth",
			TokenURL: "https://example.com/token",
		}
	}

	if opts.RedirectURI == "" {
		opts.RedirectURI = "http://127.0.0.1:8080/oauth2/callback"
	}

	app, err := NewManagerApplication(opts, deps)
	if err != nil {
		t.Fatalf("NewManagerApplication: %v", err)
	}

	return app
}

func newTestManagerLauncher(
	t *testing.T,
	edit func(*ManagerLauncherDependencies),
) *ManagerLauncher {
	t.Helper()

	deps := ManagerLauncherDependencies{
		OpenTokens: func(context.Context) (secrets.Store, error) {
			return &fakeStore{}, nil
		},
		ReadCredentials: func(context.Context, string) (config.ClientCredentials, error) {
			return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
		},
		UpdateEmailReferences: func(context.Context, string, string) error { return nil },
		FetchIdentity:         FetchUserIdentity,
		EnsureKeychainAccess:  func(context.Context) error { return nil },
		OpenBrowser:           func(context.Context, string) error { return nil },
		Out:                   io.Discard,
		Listen: func(ctx context.Context, network, address string) (net.Listener, error) {
			return (&net.ListenConfig{}).Listen(ctx, network, address)
		},
		Random: bytes.NewReader(make([]byte, 4096)),
		OAuthEndpoint: oauth2.Endpoint{
			AuthURL:  "https://example.com/auth",
			TokenURL: "https://example.com/token",
		},
	}
	if edit != nil {
		edit(&deps)
	}

	launcher, err := NewManagerLauncher(deps)
	if err != nil {
		t.Fatalf("NewManagerLauncher: %v", err)
	}

	return launcher
}

func managerRandom(stateByte byte, verifierByte byte) (*bytes.Reader, string, string) {
	stateBytes := bytes.Repeat([]byte{stateByte}, 32)
	verifierBytes := bytes.Repeat([]byte{verifierByte}, 32)
	random := make([]byte, 0, 32+len(stateBytes)+len(verifierBytes))
	random = append(random, make([]byte, 32)...)
	random = append(random, stateBytes...)
	random = append(random, verifierBytes...)

	return bytes.NewReader(random),
		base64.RawURLEncoding.EncodeToString(stateBytes),
		base64.RawURLEncoding.EncodeToString(verifierBytes)
}

type fakeStore struct {
	tokens       []secrets.Token
	defaultEmail string

	setTokenEmail       string
	setTokenClient      string
	setTokenValue       secrets.Token
	setTokenErr         error
	setDefaultCalled    string
	setDefaultClient    string
	setDefaultErr       error
	deleteCalled        string
	deleteClient        string
	deleteErr           error
	deleteDefault       bool
	deleteDefaultCalled string
	deleteDefaultErr    error
	listErr             error
	ops                 []string
}

func (s *fakeStore) Keys() ([]string, error) { return nil, nil }
func (s *fakeStore) SetToken(client string, email string, tok secrets.Token) error {
	s.setTokenClient = client
	s.setTokenEmail = email
	s.setTokenValue = tok
	s.ops = append(s.ops, "set:"+email)

	if s.setTokenErr != nil {
		return s.setTokenErr
	}

	return nil
}
func (s *fakeStore) GetToken(string, string) (secrets.Token, error) { return secrets.Token{}, nil }
func (s *fakeStore) DeleteToken(client string, email string) error {
	s.deleteClient = client
	s.deleteCalled = email
	s.ops = append(s.ops, "delete:"+email)

	if s.deleteErr != nil {
		return s.deleteErr
	}

	return nil
}

func (s *fakeStore) DeleteTokenAlias(client string, email string) error {
	return s.DeleteToken(client, email)
}

func (s *fakeStore) DeleteDefaultAccount(client string) error {
	s.deleteDefault = true
	s.deleteDefaultCalled = client
	s.defaultEmail = ""

	if s.deleteDefaultErr != nil {
		return s.deleteDefaultErr
	}

	return nil
}

func (s *fakeStore) ListTokens() ([]secrets.Token, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	return append([]secrets.Token(nil), s.tokens...), nil
}
func (s *fakeStore) GetDefaultAccount(string) (string, error) { return s.defaultEmail, nil }
func (s *fakeStore) SetDefaultAccount(client string, email string) error {
	s.setDefaultClient = client
	s.setDefaultCalled = email
	s.defaultEmail = email

	if s.setDefaultErr != nil {
		return s.setDefaultErr
	}

	return nil
}

func TestManageServer_HandleAccountsPage(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.csrfToken = "csrf"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	ms.handleAccountsPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}

	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type: %q", ct)
	}

	if body := rr.Body.String(); strings.TrimSpace(body) == "" {
		tmpl, err := template.New("accounts").Parse(accountsTemplate)
		if err != nil {
			t.Fatalf("expected body, parse err=%v", err)
		}
		var buf bytes.Buffer
		execErr := tmpl.Execute(&buf, struct{ CSRFToken string }{CSRFToken: "csrf"})

		t.Fatalf("expected body; handler wrote 0 bytes; direct execute bytes=%d err=%v", buf.Len(), execErr)
	} else {
		if !strings.Contains(body, "csrfToken") || !strings.Contains(body, "const csrfToken") {
			t.Fatalf("expected csrf js in body")
		}

		if !strings.Contains(body, "'csrf'") && !strings.Contains(body, "\"csrf\"") {
			excerpt := body
			if len(excerpt) > 200 {
				excerpt = excerpt[:200]
			}

			t.Fatalf("expected rendered token, body excerpt=%q", excerpt)
		}

		if !strings.Contains(body, "/auth/start?csrf=") || !strings.Contains(body, "&csrf=") {
			t.Fatalf("expected csrf token in auth redirect URLs")
		}
	}
}

func TestNewManagerApplicationRequiresDependencies(t *testing.T) {
	valid := ManagerDependencies{
		Tokens:                &fakeStore{},
		ReadCredentials:       func(string) (config.ClientCredentials, error) { return config.ClientCredentials{}, nil },
		UpdateEmailReferences: func(string, string) error { return nil },
		FetchIdentity:         func(context.Context, *oauth2.Token) (Identity, error) { return Identity{}, nil },
		EnsureKeychainAccess:  func(context.Context) error { return nil },
		Random:                bytes.NewReader(make([]byte, 32)),
		OAuthEndpoint:         oauth2.Endpoint{AuthURL: "https://example.com/auth", TokenURL: "https://example.com/token"},
	}
	opts := ManagerOptions{RedirectURI: "http://127.0.0.1/oauth2/callback"}

	tests := []struct {
		name string
		edit func(*ManagerOptions, *ManagerDependencies)
		want error
	}{
		{name: "tokens", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.Tokens = nil }, want: errManagerTokensRequired},
		{name: "credentials", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.ReadCredentials = nil }, want: errCredentialsReaderRequired},
		{name: "updater", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.UpdateEmailReferences = nil }, want: errEmailReferenceUpdaterRequired},
		{name: "identity", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.FetchIdentity = nil }, want: errManagerIdentityRequired},
		{name: "keychain", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.EnsureKeychainAccess = nil }, want: errManagerKeychainRequired},
		{name: "random", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.Random = nil }, want: errManagerRandomRequired},
		{name: "endpoint", edit: func(_ *ManagerOptions, deps *ManagerDependencies) { deps.OAuthEndpoint = oauth2.Endpoint{} }, want: errManagerOAuthEndpointInvalid},
		{name: "redirect", edit: func(opts *ManagerOptions, _ *ManagerDependencies) { opts.RedirectURI = "" }, want: errManagerRedirectURIRequired},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotOpts := opts
			gotDeps := valid
			tc.edit(&gotOpts, &gotDeps)

			_, err := NewManagerApplication(gotOpts, gotDeps)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestManagerApplicationHandlerRoutes(t *testing.T) {
	app := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestManageServer_HandleListAccounts_DefaultFirst(t *testing.T) {
	store := &fakeStore{
		tokens: []secrets.Token{
			{Email: "a@b.com", Services: []string{"gmail"}},
			{Email: "c@d.com", Services: []string{"drive"}},
		},
	}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)
	ms.handleListAccounts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d", rr.Code)
	}
	var parsed struct {
		Accounts []AccountInfo `json:"accounts"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}

	if len(parsed.Accounts) != 2 || !parsed.Accounts[0].IsDefault || parsed.Accounts[1].IsDefault {
		t.Fatalf("unexpected defaults: %#v", parsed.Accounts)
	}
}

func TestManageServer_HandleListAccounts_DefaultExplicit(t *testing.T) {
	store := &fakeStore{
		tokens: []secrets.Token{
			{Email: "a@b.com", Services: []string{"gmail"}},
			{Email: "c@d.com", Services: []string{"drive"}},
		},
		defaultEmail: "c@d.com",
	}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)
	ms.handleListAccounts(rr, req)

	var parsed struct {
		Accounts []AccountInfo `json:"accounts"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}

	if len(parsed.Accounts) != 2 || parsed.Accounts[0].IsDefault || !parsed.Accounts[1].IsDefault {
		t.Fatalf("unexpected defaults: %#v", parsed.Accounts)
	}
}

func TestManageServer_HandleListAccounts_StaleDefaultFallsBackToFirst(t *testing.T) {
	store := &fakeStore{
		tokens:       []secrets.Token{{Email: "a@b.com"}, {Email: "c@d.com"}},
		defaultEmail: "missing@example.com",
	}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)
	ms.handleListAccounts(rr, req)

	var parsed struct {
		Accounts []AccountInfo `json:"accounts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}

	if len(parsed.Accounts) != 2 || !parsed.Accounts[0].IsDefault || parsed.Accounts[1].IsDefault {
		t.Fatalf("unexpected defaults: %#v", parsed.Accounts)
	}
}

func TestManageServer_OAuthStatesAreIndependent(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.addOAuthState("state1", "verifier1")
	ms.addOAuthState("state2", "verifier2")

	if verifier, ok := ms.consumeOAuthState("state1"); !ok || verifier != "verifier1" {
		t.Fatalf("expected first state accepted")
	}

	if _, ok := ms.consumeOAuthState("state1"); ok {
		t.Fatalf("expected consumed state rejected")
	}

	if verifier, ok := ms.consumeOAuthState("state2"); !ok || verifier != "verifier2" {
		t.Fatalf("expected second state accepted")
	}

	if len(ms.oauthStates) != 0 {
		t.Fatalf("expected all states consumed, got %#v", ms.oauthStates)
	}
}

func TestManagerApplicationOAuthStateRegistryConcurrent(t *testing.T) {
	app := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	const count = 100

	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)

		go func() {
			defer wg.Done()
			state := strconv.Itoa(i)
			app.addOAuthState(state, "verifier-"+state)
		}()
	}

	wg.Wait()

	for i := range count {
		wg.Add(1)

		go func() {
			defer wg.Done()

			state := strconv.Itoa(i)
			if verifier, ok := app.consumeOAuthState(state); !ok || verifier != "verifier-"+state {
				t.Errorf("state %s: verifier=%q ok=%v", state, verifier, ok)
			}
		}()
	}

	wg.Wait()
}

func TestManageServer_HandleOAuthCallback_ErrorAndValidation(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.csrfToken = "csrf"
	ms.addOAuthState("state1", testCodeVerifier)

	t.Run("cancelled", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?error=access_denied", nil)
		ms.handleOAuthCallback(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("state mismatch", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=nope&code=abc", nil)
		ms.handleOAuthCallback(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("missing code", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1", nil)
		ms.handleOAuthCallback(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status: %d", rr.Code)
		}
	})
}

func TestManageServer_HandleSetDefault_AndRemove(t *testing.T) {
	store := &fakeStore{
		tokens: []secrets.Token{{Email: "a@b.com"}},
	}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"

	t.Run("set-default csrf", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/set-default", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
		req.Header.Set("X-CSRF-Token", "nope")
		ms.handleSetDefault(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("set-default ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/set-default", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleSetDefault(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
		}

		if store.setDefaultCalled != "a@b.com" {
			t.Fatalf("expected setDefaultCalled")
		}
	})

	t.Run("set-default bad method", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/set-default", nil)
		ms.handleSetDefault(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("set-default bad json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/set-default", bytes.NewReader([]byte(`{`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleSetDefault(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("set-default store error", func(t *testing.T) {
		store.setDefaultErr = errBoom

		t.Cleanup(func() { store.setDefaultErr = nil })
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/set-default", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleSetDefault(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("set-default missing account", func(t *testing.T) {
		store.setDefaultErr = nil
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/set-default", bytes.NewReader([]byte(`{"email":"missing@example.com"}`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleSetDefault(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("remove ok", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/remove-account", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleRemoveAccount(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status: %d", rr.Code)
		}

		if store.deleteCalled != "a@b.com" {
			t.Fatalf("expected deleteCalled")
		}
	})

	t.Run("remove bad method", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/remove-account", nil)
		ms.handleRemoveAccount(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("remove bad json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/remove-account", bytes.NewReader([]byte(`{`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleRemoveAccount(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status: %d", rr.Code)
		}
	})

	t.Run("remove store error", func(t *testing.T) {
		store.deleteErr = errBoom

		t.Cleanup(func() { store.deleteErr = nil })
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/remove-account", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
		req.Header.Set("X-CSRF-Token", "csrf")
		ms.handleRemoveAccount(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status: %d", rr.Code)
		}
	})
}

func TestManageServer_HandleRemoveAccountResetsDefault(t *testing.T) {
	store := &fakeStore{
		tokens:       []secrets.Token{{Email: "a@b.com"}, {Email: "c@d.com"}},
		defaultEmail: "a@b.com",
	}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/remove-account", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
	req.Header.Set("X-CSRF-Token", "csrf")
	ms.handleRemoveAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}

	if store.setDefaultCalled != "c@d.com" {
		t.Fatalf("expected default moved, got %q", store.setDefaultCalled)
	}
}

func TestManageServer_HandleRemoveAccountClearsLastDefault(t *testing.T) {
	store := &fakeStore{
		tokens:       []secrets.Token{{Email: "a@b.com"}},
		defaultEmail: "a@b.com",
	}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/remove-account", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
	req.Header.Set("X-CSRF-Token", "csrf")
	ms.handleRemoveAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}

	if !store.deleteDefault || store.deleteDefaultCalled != config.DefaultClientName {
		t.Fatalf("expected default cleared for default client, called=%v client=%q", store.deleteDefault, store.deleteDefaultCalled)
	}

	if store.defaultEmail != "" {
		t.Fatalf("expected defaultEmail cleared, got %q", store.defaultEmail)
	}
}

func TestManageServer_HandleListAccounts_Error(t *testing.T) {
	store := &fakeStore{listErr: errBoom}
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{Tokens: store})
	ms.csrfToken = "csrf"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/accounts", nil)
	ms.handleListAccounts(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	token, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("generateCSRFToken: %v", err)
	}

	if len(token) != 64 {
		t.Fatalf("unexpected token length: %d", len(token))
	}

	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("token not hex: %v", err)
	}
}

func TestRenderSuccessPageWithDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	renderSuccessPageWithDetails(rr, "me@example.com", []string{"gmail", "drive"})

	if body := rr.Body.String(); !strings.Contains(body, "me@example.com") {
		t.Fatalf("expected email in body")
	} else {
		if !strings.Contains(body, "gmail") || !strings.Contains(body, "drive") {
			t.Fatalf("expected services in body")
		}

		if !strings.Contains(body, strconv.Itoa(postSuccessDisplaySeconds)) {
			t.Fatalf("expected countdown in body")
		}
	}
}

func TestManageServer_HandleAuthStart(t *testing.T) {
	random, expectedState, expectedVerifier := managerRandom(1, 2)
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{
		Random: random,
		OAuthEndpoint: oauth2.Endpoint{
			AuthURL:  "http://example.com/auth",
			TokenURL: "http://example.com/token",
		},
	})
	ms.csrfToken = "csrf"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/start?csrf=csrf", nil)
	ms.handleAuthStart(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}
	loc := rr.Header().Get("Location")

	parsed, parseErr := url.Parse(loc)
	if parseErr != nil {
		t.Fatalf("parse location: %v", parseErr)
	}

	if parsed.Host != "example.com" {
		t.Fatalf("unexpected host: %q", parsed.Host)
	}

	if state := parsed.Query().Get("state"); state != expectedState {
		t.Fatalf("unexpected state: %q", state)
	}

	if verifier := ms.oauthStates[expectedState]; verifier != expectedVerifier {
		t.Fatalf("unexpected stored verifier: %q", verifier)
	}

	if got := parsed.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("expected S256 challenge method, got %q", got)
	}

	if got, want := parsed.Query().Get("code_challenge"), oauth2.S256ChallengeFromVerifier(expectedVerifier); got != want {
		t.Fatalf("unexpected code_challenge: got %q want %q", got, want)
	}

	if got := parsed.Query().Get("code_verifier"); got != "" {
		t.Fatalf("code_verifier must not be exposed in auth URL, got %q", got)
	}

	if redirectURI := parsed.Query().Get("redirect_uri"); !strings.Contains(redirectURI, "127.0.0.1:") {
		t.Fatalf("expected redirect uri, got %q", redirectURI)
	}

	scope := parsed.Query().Get("scope")
	if scope == "" {
		t.Fatalf("expected scope query param")
	}
	required := map[string]bool{
		scopeOpenID:        false,
		scopeEmail:         false,
		scopeUserinfoEmail: false,
	}

	for _, s := range strings.Fields(scope) {
		if _, ok := required[s]; ok {
			required[s] = true
		}
	}

	for s, ok := range required {
		if !ok {
			t.Fatalf("expected %q scope, got %q", s, scope)
		}
	}
}

func TestManageServer_HandleAuthStart_RedirectURIOverride(t *testing.T) {
	random, _, _ := managerRandom(1, 2)
	ms := newTestManagerApplication(t, ManagerOptions{
		RedirectURI: "https://gog.example.com/oauth2/callback",
	}, ManagerDependencies{
		Random: random,
		OAuthEndpoint: oauth2.Endpoint{
			AuthURL:  "http://example.com/auth",
			TokenURL: "http://example.com/token",
		},
	})
	ms.csrfToken = "csrf"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/start?csrf=csrf", nil)
	ms.handleAuthStart(rr, req)

	loc := rr.Header().Get("Location")

	parsed, parseErr := url.Parse(loc)
	if parseErr != nil {
		t.Fatalf("parse location: %v", parseErr)
	}

	if got := parsed.Query().Get("redirect_uri"); got != "https://gog.example.com/oauth2/callback" {
		t.Fatalf("unexpected redirect uri: %q", got)
	}
}

func TestManageServer_HandleAuthStart_CredentialsError(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/start?csrf=csrf", nil)
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{}, errBoom
		},
	})
	ms.csrfToken = "csrf"
	ms.handleAuthStart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestManageServer_HandleAuthStart_RejectsBadRequest(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.csrfToken = "csrf"
	assertManageServerRejectsBadRequest(t, ms.handleAuthStart,
		"/auth/start?csrf=csrf", "/auth/start", "/auth/start?csrf=bad")
}

func TestManageServer_HandleOAuthCallback_Success(t *testing.T) {
	// Mock userinfo endpoint
	userinfoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/v2/userinfo" {
			t.Fatalf("unexpected path: %s", r.URL.Path)

			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer token" {
			t.Fatalf("expected Bearer token, got %q", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"email": "me@example.com"})
	}))
	defer userinfoSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		if r.Form.Get("code") != "abc" {
			t.Fatalf("expected code=abc, got %q", r.Form.Get("code"))
		}

		if got := r.Form.Get("code_verifier"); got != testCodeVerifier {
			t.Fatalf("expected code_verifier, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "token",
			"refresh_token": "refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	store := &fakeStore{}
	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		Tokens: store,
		FetchIdentity: func(ctx context.Context, tok *oauth2.Token) (Identity, error) {
			return fetchUserIdentityWithURL(ctx, tok.AccessToken, userinfoSrv.URL+"/oauth2/v2/userinfo")
		},
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}

	if store.setTokenEmail != "me@example.com" {
		t.Fatalf("expected token stored for me@example.com, got %q", store.setTokenEmail)
	}

	if store.setTokenValue.RefreshToken != "refresh" {
		t.Fatalf("expected refresh token stored")
	}

	if !strings.Contains(rr.Body.String(), "me@example.com") {
		t.Fatalf("expected body to include email")
	}
}

func TestManageServer_HandleOAuthCallback_MigratesAndDeletesAliasAfterSetToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "token",
			"refresh_token": "refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	store := &fakeStore{
		tokens: []secrets.Token{{
			Client:       config.DefaultClientName,
			Email:        "old@example.com",
			Subject:      "sub-123",
			RefreshToken: "old-refresh",
		}},
	}

	configStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if writeErr := configStore.Write(config.File{
		AccountAliases: map[string]string{"work": "old@example.com"},
	}); writeErr != nil {
		t.Fatalf("write config: %v", writeErr)
	}
	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
		Client:   config.DefaultClientName,
	}, ManagerDependencies{
		Tokens: store,
		FetchIdentity: func(context.Context, *oauth2.Token) (Identity, error) {
			return Identity{Subject: "sub-123", Email: "new@example.com"}, nil
		},
		UpdateEmailReferences: configStore.MigrateAccountEmailReferences,
		OAuthEndpoint:         oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}

	wantOps := []string{"set:new@example.com", "delete:old@example.com"}
	if len(store.ops) != len(wantOps) || store.ops[0] != wantOps[0] || store.ops[1] != wantOps[1] {
		t.Fatalf("unexpected store ops: %#v", store.ops)
	}

	cfg, err := configStore.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if cfg.AccountAliases["work"] != "new@example.com" {
		t.Fatalf("account aliases = %#v", cfg.AccountAliases)
	}
}

func TestStartManageServerRejectsNonLoopbackListenAddr(t *testing.T) {
	launcher := newTestManagerLauncher(t, nil)

	err := launcher.Start(context.Background(), ManageServerOptions{
		ListenAddr: "0.0.0.0:0",
		Timeout:    50 * time.Millisecond,
	})
	if err == nil {
		t.Fatalf("expected non-loopback listen addr error")
	}

	if !errors.Is(err, errNonLoopbackManageAddr) {
		t.Fatalf("expected errNonLoopbackManageAddr, got %v", err)
	}
}

func TestNewManagerLauncherRequiresDependencies(t *testing.T) {
	valid := ManagerLauncherDependencies{
		OpenTokens: func(context.Context) (secrets.Store, error) { return &fakeStore{}, nil },
		ReadCredentials: func(context.Context, string) (config.ClientCredentials, error) {
			return config.ClientCredentials{}, nil
		},
		UpdateEmailReferences: func(context.Context, string, string) error { return nil },
		FetchIdentity:         FetchUserIdentity,
		EnsureKeychainAccess:  func(context.Context) error { return nil },
		OpenBrowser:           func(context.Context, string) error { return nil },
		Out:                   io.Discard,
		Listen: func(ctx context.Context, network, address string) (net.Listener, error) {
			return (&net.ListenConfig{}).Listen(ctx, network, address)
		},
		Random:        bytes.NewReader(make([]byte, 32)),
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "https://example.com/auth", TokenURL: "https://example.com/token"},
	}

	tests := []struct {
		name string
		edit func(*ManagerLauncherDependencies)
		want error
	}{
		{name: "tokens", edit: func(deps *ManagerLauncherDependencies) { deps.OpenTokens = nil }, want: errManagerTokenOpenerRequired},
		{name: "credentials", edit: func(deps *ManagerLauncherDependencies) { deps.ReadCredentials = nil }, want: errCredentialsReaderRequired},
		{name: "updater", edit: func(deps *ManagerLauncherDependencies) { deps.UpdateEmailReferences = nil }, want: errManagerConfigUpdateRequired},
		{name: "identity", edit: func(deps *ManagerLauncherDependencies) { deps.FetchIdentity = nil }, want: errManagerIdentityRequired},
		{name: "keychain", edit: func(deps *ManagerLauncherDependencies) { deps.EnsureKeychainAccess = nil }, want: errManagerKeychainRequired},
		{name: "browser", edit: func(deps *ManagerLauncherDependencies) { deps.OpenBrowser = nil }, want: errManagerBrowserRequired},
		{name: "output", edit: func(deps *ManagerLauncherDependencies) { deps.Out = nil }, want: errManagerOutputRequired},
		{name: "listener", edit: func(deps *ManagerLauncherDependencies) { deps.Listen = nil }, want: errManagerListenerRequired},
		{name: "random", edit: func(deps *ManagerLauncherDependencies) { deps.Random = nil }, want: errManagerRandomRequired},
		{name: "endpoint", edit: func(deps *ManagerLauncherDependencies) { deps.OAuthEndpoint = oauth2.Endpoint{} }, want: errManagerOAuthEndpointInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := valid
			tc.edit(&deps)

			_, err := NewManagerLauncher(deps)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestManagerLauncherBindsApplicationDependenciesToStartContext(t *testing.T) {
	type contextKey struct{}
	startCtx := context.WithValue(context.Background(), contextKey{}, "start")
	launcher := newTestManagerLauncher(t, func(deps *ManagerLauncherDependencies) {
		deps.ReadCredentials = func(ctx context.Context, _ string) (config.ClientCredentials, error) {
			if ctx.Value(contextKey{}) != "start" {
				t.Fatalf("credentials context = %v", ctx.Value(contextKey{}))
			}

			return config.ClientCredentials{}, nil
		}
		deps.UpdateEmailReferences = func(ctx context.Context, _, _ string) error {
			if ctx.Value(contextKey{}) != "start" {
				t.Fatalf("updater context = %v", ctx.Value(contextKey{}))
			}

			return nil
		}
		deps.EnsureKeychainAccess = func(ctx context.Context) error {
			if ctx.Value(contextKey{}) != "start" {
				t.Fatalf("keychain context = %v", ctx.Value(contextKey{}))
			}

			return nil
		}
	})

	deps := launcher.applicationDependencies(startCtx, &fakeStore{})
	if _, err := deps.ReadCredentials(""); err != nil {
		t.Fatalf("ReadCredentials: %v", err)
	}

	if err := deps.UpdateEmailReferences("", ""); err != nil {
		t.Fatalf("UpdateEmailReferences: %v", err)
	}

	if err := deps.EnsureKeychainAccess(context.Background()); err != nil {
		t.Fatalf("EnsureKeychainAccess: %v", err)
	}
}

func TestManageServer_HandleOAuthCallback_NoKeychainPreflight(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "token",
			"refresh_token": "refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	store := &fakeStore{}
	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		Tokens: store,
		FetchIdentity: func(context.Context, *oauth2.Token) (Identity, error) {
			return Identity{Email: "me@example.com"}, nil
		},
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}

	if store.setTokenEmail != "me@example.com" {
		t.Fatalf("expected token stored for me@example.com, got %q", store.setTokenEmail)
	}
}

func TestManageServer_HandleOAuthCallback_Success_IDTokenEmail(t *testing.T) {
	idToken := strings.Join([]string{
		base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"email":"me@example.com"}`)),
		"",
	}, ".")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "token",
			"refresh_token": "refresh",
			"id_token":      idToken,
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	store := &fakeStore{}
	preflightCalled := false
	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		Tokens: store,
		EnsureKeychainAccess: func(context.Context) error {
			preflightCalled = true
			return nil
		},
		FetchIdentity: FetchUserIdentity,
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}

	if store.setTokenEmail != "me@example.com" {
		t.Fatalf("expected token stored for me@example.com, got %q", store.setTokenEmail)
	}

	if !preflightCalled {
		t.Fatal("expected keychain preflight")
	}
}

func TestFetchUserEmail(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
				t.Fatalf("expected Bearer test-token, got %q", auth)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"email": "user@test.com"})
		}))
		defer srv.Close()

		email, err := fetchUserEmailWithURL(context.Background(), "test-token", srv.URL)
		if err != nil {
			t.Fatalf("fetchUserEmail: %v", err)
		}

		if email != "user@test.com" {
			t.Fatalf("expected user@test.com, got %q", email)
		}
	})

	t.Run("empty email", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"email": ""})
		}))
		defer srv.Close()

		_, err := fetchUserEmailWithURL(context.Background(), "test-token", srv.URL)
		if err == nil {
			t.Fatal("expected error for empty email")
		}

		if !errors.Is(err, errNoEmailInResponse) {
			t.Fatalf("expected errNoEmailInResponse, got %v", err)
		}
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		_, err := fetchUserEmailWithURL(context.Background(), "test-token", srv.URL)
		if err == nil {
			t.Fatal("expected error for 401")
		}

		if !errors.Is(err, errUserinfoRequestFailed) {
			t.Fatalf("expected errUserinfoRequestFailed, got %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{invalid"))
		}))
		defer srv.Close()

		_, err := fetchUserEmailWithURL(context.Background(), "test-token", srv.URL)
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
	})
}

func TestEmailFromIDToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		idToken := strings.Join([]string{
			base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
			base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"sub-123","email":"me@example.com"}`)),
			"",
		}, ".")

		email, err := emailFromIDToken(idToken)
		if err != nil {
			t.Fatalf("emailFromIDToken: %v", err)
		}

		if email != "me@example.com" {
			t.Fatalf("expected me@example.com, got %q", email)
		}

		identity, err := IdentityFromIDToken(idToken)
		if err != nil {
			t.Fatalf("IdentityFromIDToken: %v", err)
		}

		if identity.Subject != "sub-123" || identity.Email != "me@example.com" {
			t.Fatalf("unexpected identity: %#v", identity)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := emailFromIDToken("nope")
		if err == nil {
			t.Fatal("expected error")
		}

		if !errors.Is(err, errInvalidIDToken) {
			t.Fatalf("expected errInvalidIDToken, got %v", err)
		}
	})

	t.Run("missing email", func(t *testing.T) {
		idToken := strings.Join([]string{
			base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)),
			base64.RawURLEncoding.EncodeToString([]byte(`{}`)),
			"",
		}, ".")

		_, err := emailFromIDToken(idToken)
		if err == nil {
			t.Fatal("expected error")
		}

		if !errors.Is(err, errNoEmailInIDToken) {
			t.Fatalf("expected errNoEmailInIDToken, got %v", err)
		}
	})
}

func TestStartManageServer_Timeout(t *testing.T) {
	var opened string
	launcher := newTestManagerLauncher(t, func(deps *ManagerLauncherDependencies) {
		deps.OpenBrowser = func(_ context.Context, url string) error {
			opened = url
			return nil
		}
	})

	ctx := context.Background()
	if err := launcher.Start(ctx, ManageServerOptions{Timeout: 50 * time.Millisecond}); err != nil {
		t.Fatalf("ManagerLauncher.Start: %v", err)
	}

	if !strings.Contains(opened, "http://127.0.0.1:") {
		t.Fatalf("expected browser URL, got %q", opened)
	}
}

func TestManagerLauncherRoutesOutputAndClosesListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var output bytes.Buffer
	var opened string
	client := &http.Client{Timeout: time.Second}
	launcher := newTestManagerLauncher(t, func(deps *ManagerLauncherDependencies) {
		deps.Out = &output
		deps.OpenBrowser = func(browserCtx context.Context, url string) error {
			opened = url

			for _, path := range []string{"/", "/accounts"} {
				req, err := http.NewRequestWithContext(browserCtx, http.MethodGet, url+path, nil)
				if err != nil {
					t.Fatalf("create GET %s: %v", path, err)
				}

				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("GET %s: %v", path, err)
				}

				_ = resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					t.Fatalf("GET %s status = %d", path, resp.StatusCode)
				}
			}

			cancel()

			return errBrowserUnavailable
		}
	})

	if err := launcher.Start(ctx, ManageServerOptions{Timeout: time.Second}); err != nil {
		t.Fatalf("ManagerLauncher.Start: %v", err)
	}

	if !strings.Contains(output.String(), "If the browser doesn't open, visit: "+opened) {
		t.Fatalf("missing fallback URL output: %q", output.String())
	}

	if !strings.Contains(output.String(), "Failed to open browser: "+errBrowserUnavailable.Error()) {
		t.Fatalf("missing browser error output: %q", output.String())
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, opened+"/accounts", nil)
	if err != nil {
		t.Fatalf("create closed-listener request: %v", err)
	}

	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err == nil {
		t.Fatal("expected listener to be closed")
	}
}

func TestManageServer_HandleAuthUpgrade(t *testing.T) {
	random, expectedState, expectedVerifier := managerRandom(2, 3)
	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		Random: random,
		OAuthEndpoint: oauth2.Endpoint{
			AuthURL:  "http://example.com/auth",
			TokenURL: "http://example.com/token",
		},
	})
	ms.csrfToken = "csrf"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/upgrade?email=test@example.com&csrf=csrf", nil)
	ms.handleAuthUpgrade(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status: %d", rr.Code)
	}

	loc := rr.Header().Get("Location")

	parsed, parseErr := url.Parse(loc)
	if parseErr != nil {
		t.Fatalf("parse location: %v", parseErr)
	}

	if parsed.Host != "example.com" {
		t.Fatalf("unexpected host: %q", parsed.Host)
	}

	if state := parsed.Query().Get("state"); state != expectedState {
		t.Fatalf("unexpected state: %q", state)
	}

	if verifier := ms.oauthStates[expectedState]; verifier != expectedVerifier {
		t.Fatalf("unexpected stored verifier: %q", verifier)
	}

	if got := parsed.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("expected S256 challenge method, got %q", got)
	}

	if got, want := parsed.Query().Get("code_challenge"), oauth2.S256ChallengeFromVerifier(expectedVerifier); got != want {
		t.Fatalf("unexpected code_challenge: got %q want %q", got, want)
	}

	scope := parsed.Query().Get("scope")

	expectedScopes, err := ScopesForManage([]Service{ServiceGmail})
	if err != nil {
		t.Fatalf("ScopesForManage: %v", err)
	}

	scopeSet := make(map[string]bool, len(expectedScopes))
	for _, s := range strings.Fields(scope) {
		scopeSet[s] = true
	}

	for _, s := range expectedScopes {
		if !scopeSet[s] {
			t.Fatalf("expected scope %q in %q", s, scope)
		}
	}

	if scopeSet["https://www.googleapis.com/auth/keep.readonly"] {
		t.Fatalf("unexpected keep scope in %q", scope)
	}

	// Check for login_hint (pre-selects the email)
	if loginHint := parsed.Query().Get("login_hint"); loginHint != "test@example.com" {
		t.Fatalf("expected login_hint=test@example.com, got %q", loginHint)
	}

	// Check for prompt=consent (forces consent screen)
	if prompt := parsed.Query().Get("prompt"); prompt != "consent" {
		t.Fatalf("expected prompt=consent, got %q", prompt)
	}
}

func TestManageServer_HandleAuthUpgrade_MissingEmail(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.csrfToken = "csrf"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/upgrade?csrf=csrf", nil)
	ms.handleAuthUpgrade(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestManageServer_HandleAuthUpgrade_CredentialsError(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/upgrade?email=test@example.com&csrf=csrf", nil)
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{}, errBoom
		},
	})
	ms.csrfToken = "csrf"
	ms.handleAuthUpgrade(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServer_HandleAuthUpgrade_RejectsBadRequest(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.csrfToken = "csrf"
	assertManageServerRejectsBadRequest(t, ms.handleAuthUpgrade,
		"/auth/upgrade?email=test@example.com&csrf=csrf",
		"/auth/upgrade?email=test@example.com",
		"/auth/upgrade?email=test@example.com&csrf=bad")
}

func assertManageServerRejectsBadRequest(
	t *testing.T,
	handler http.HandlerFunc,
	validTarget, missingCSRFTarget, badCSRFTarget string,
) {
	t.Helper()
	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{name: "bad method", method: http.MethodPost, target: validTarget, want: http.StatusMethodNotAllowed},
		{name: "missing csrf", method: http.MethodGet, target: missingCSRFTarget, want: http.StatusForbidden},
		{name: "bad csrf", method: http.MethodGet, target: badCSRFTarget, want: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.target, nil)
			handler(rr, req)

			if rr.Code != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, rr.Code)
			}
		})
	}
}
