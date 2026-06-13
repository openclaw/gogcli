package googleauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

var (
	errBoom          = errors.New("boom")
	errShouldNotCall = errors.New("should not call")
)

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
	ms := &ManageServer{
		csrfToken: "csrf",
	}
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

func TestManageServer_HandleListAccounts_DefaultFirst(t *testing.T) {
	store := &fakeStore{
		tokens: []secrets.Token{
			{Email: "a@b.com", Services: []string{"gmail"}},
			{Email: "c@d.com", Services: []string{"drive"}},
		},
	}
	ms := &ManageServer{
		csrfToken: "csrf",
		store:     store,
	}

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
	ms := &ManageServer{
		csrfToken: "csrf",
		store:     store,
	}

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
	ms := &ManageServer{
		csrfToken: "csrf",
		store:     store,
	}

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
	ms := &ManageServer{}
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

	if ms.oauthState != "" || ms.oauthVerifier != "" {
		t.Fatalf("expected consumed current verifier to be cleared, got state=%q verifier=%q", ms.oauthState, ms.oauthVerifier)
	}
}

func TestManageServer_HandleOAuthCallback_ErrorAndValidation(t *testing.T) {
	ms := &ManageServer{
		csrfToken:     "csrf",
		oauthState:    "state1",
		oauthVerifier: testCodeVerifier,
	}
	// Need a listener for redirectURI generation even though we don't reach exchange.
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })
	ms.listener = ln

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
	ms := &ManageServer{
		csrfToken: "csrf",
		store:     store,
	}

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
	ms := &ManageServer{csrfToken: "csrf", store: store}

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
	ms := &ManageServer{csrfToken: "csrf", store: store}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/remove-account", bytes.NewReader([]byte(`{"email":"a@b.com"}`)))
	req.Header.Set("X-CSRF-Token", "csrf")
	ms.handleRemoveAccount(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}

	if !store.deleteDefault || store.deleteDefaultCalled != "" {
		t.Fatalf("expected default cleared for empty client, called=%v client=%q", store.deleteDefault, store.deleteDefaultCalled)
	}

	if store.defaultEmail != "" {
		t.Fatalf("expected defaultEmail cleared, got %q", store.defaultEmail)
	}
}

func TestManageServer_HandleListAccounts_Error(t *testing.T) {
	store := &fakeStore{listErr: errBoom}
	ms := &ManageServer{csrfToken: "csrf", store: store}
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
	origRead := readClientCredentials
	origState := randomStateFn
	origEndpoint := oauthEndpoint
	origVerifier := generateVerifierFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		randomStateFn = origState
		oauthEndpoint = origEndpoint
		generateVerifierFn = origVerifier
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }
	generateVerifierFn = func() string { return testCodeVerifier }
	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: "http://example.com/token"}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{listener: ln, csrfToken: "csrf"}
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

	if state := parsed.Query().Get("state"); state != "state123" {
		t.Fatalf("unexpected state: %q", state)
	}

	if ms.oauthState != "state123" {
		t.Fatalf("expected oauthState set")
	}

	if ms.oauthVerifier != testCodeVerifier {
		t.Fatalf("expected oauthVerifier set")
	}

	if got := parsed.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("expected S256 challenge method, got %q", got)
	}

	if got, want := parsed.Query().Get("code_challenge"), pkceChallengeForTest(); got != want {
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
	origRead := readClientCredentials
	origState := randomStateFn
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		randomStateFn = origState
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }
	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: "http://example.com/token"}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		listener:  ln,
		csrfToken: "csrf",
		opts:      ManageServerOptions{RedirectURI: "https://gog.example.com/oauth2/callback"},
	}
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
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{}, errBoom
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/start?csrf=csrf", nil)
	ms := &ManageServer{csrfToken: "csrf"}
	ms.handleAuthStart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status: %d", rr.Code)
	}
}

func TestManageServer_HandleAuthStart_RejectsBadRequest(t *testing.T) {
	ms := &ManageServer{csrfToken: "csrf"}

	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{name: "bad method", method: http.MethodPost, target: "/auth/start?csrf=csrf", want: http.StatusMethodNotAllowed},
		{name: "missing csrf", method: http.MethodGet, target: "/auth/start", want: http.StatusForbidden},
		{name: "bad csrf", method: http.MethodGet, target: "/auth/start?csrf=bad", want: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.target, nil)
			ms.handleAuthStart(rr, req)

			if rr.Code != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, rr.Code)
			}
		})
	}
}

func TestManageServer_HandleOAuthCallback_Success(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

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

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	store := &fakeStore{}
	ms := &ManageServer{
		oauthState:    "state1",
		oauthVerifier: testCodeVerifier,
		listener:      ln,
		store:         store,
		fetchIdentity: func(ctx context.Context, tok *oauth2.Token) (Identity, error) {
			return fetchUserIdentityWithURL(ctx, tok.AccessToken, userinfoSrv.URL+"/oauth2/v2/userinfo")
		},
		opts: ManageServerOptions{Services: []Service{ServiceGmail}},
	}

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

	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origResolve := resolveKeyringBackendInfo
	origEnsure := ensureKeychainAccess

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		resolveKeyringBackendInfo = origResolve
		ensureKeychainAccess = origEnsure
	})

	resolveKeyringBackendInfo = func() (secrets.KeyringBackendInfo, error) {
		return secrets.KeyringBackendInfo{Value: "file", Source: "env"}, nil
	}
	ensureKeychainAccess = func() error {
		return errShouldNotCall
	}
	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

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

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

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
	ms := &ManageServer{
		oauthState:    "state1",
		oauthVerifier: testCodeVerifier,
		listener:      ln,
		store:         store,
		fetchIdentity: func(context.Context, *oauth2.Token) (Identity, error) {
			return Identity{Subject: "sub-123", Email: "new@example.com"}, nil
		},
		opts: ManageServerOptions{
			Services:              []Service{ServiceGmail},
			UpdateEmailReferences: configStore.MigrateAccountEmailReferences,
		},
		client: config.DefaultClientName,
	}

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
	err := StartManageServer(context.Background(), ManageServerOptions{
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

func TestStartManageServerRequiresEmailReferenceUpdater(t *testing.T) {
	err := StartManageServer(context.Background(), ManageServerOptions{
		ListenAddr: "127.0.0.1:0",
		Timeout:    50 * time.Millisecond,
	})
	if !errors.Is(err, errEmailReferenceUpdaterRequired) {
		t.Fatalf("error = %v, want updater-required", err)
	}
}

func TestManageServer_HandleOAuthCallback_FileBackendSkipsKeychain(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origResolve := resolveKeyringBackendInfo
	origEnsure := ensureKeychainAccess

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		resolveKeyringBackendInfo = origResolve
		ensureKeychainAccess = origEnsure
	})

	resolveKeyringBackendInfo = func() (secrets.KeyringBackendInfo, error) {
		return secrets.KeyringBackendInfo{Value: "file", Source: "env"}, nil
	}
	ensureKeychainAccess = func() error {
		return errShouldNotCall
	}

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

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

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	store := &fakeStore{}
	ms := &ManageServer{
		oauthState:    "state1",
		oauthVerifier: testCodeVerifier,
		listener:      ln,
		store:         store,
		fetchIdentity: func(ctx context.Context, tok *oauth2.Token) (Identity, error) {
			return Identity{Email: "me@example.com"}, nil
		},
		opts: ManageServerOptions{Services: []Service{ServiceGmail}},
	}

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
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

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

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	store := &fakeStore{}
	ms := &ManageServer{
		oauthState:    "state1",
		oauthVerifier: testCodeVerifier,
		listener:      ln,
		store:         store,
		opts:          ManageServerOptions{Services: []Service{ServiceGmail}},
	}

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
	origStore := openDefaultStore
	origOpen := openBrowserFn

	t.Cleanup(func() {
		openDefaultStore = origStore
		openBrowserFn = origOpen
	})

	openDefaultStore = func() (secrets.Store, error) { return &fakeStore{}, nil }
	var opened string
	openBrowserFn = func(url string) error {
		opened = url
		return nil
	}

	ctx := context.Background()
	if err := StartManageServer(ctx, ManageServerOptions{
		Timeout:               50 * time.Millisecond,
		UpdateEmailReferences: func(string, string) error { return nil },
	}); err != nil {
		t.Fatalf("StartManageServer: %v", err)
	}

	if !strings.Contains(opened, "http://127.0.0.1:") {
		t.Fatalf("expected browser URL, got %q", opened)
	}
}

func TestManageServer_HandleAuthUpgrade(t *testing.T) {
	origRead := readClientCredentials
	origState := randomStateFn
	origEndpoint := oauthEndpoint
	origVerifier := generateVerifierFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		randomStateFn = origState
		oauthEndpoint = origEndpoint
		generateVerifierFn = origVerifier
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state456", nil }
	generateVerifierFn = func() string { return testCodeVerifier }
	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: "http://example.com/token"}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		listener:  ln,
		csrfToken: "csrf",
		opts:      ManageServerOptions{Services: []Service{ServiceGmail}},
	}
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

	if state := parsed.Query().Get("state"); state != "state456" {
		t.Fatalf("unexpected state: %q", state)
	}

	if ms.oauthState != "state456" {
		t.Fatalf("expected oauthState set")
	}

	if ms.oauthVerifier != testCodeVerifier {
		t.Fatalf("expected oauthVerifier set")
	}

	if got := parsed.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("expected S256 challenge method, got %q", got)
	}

	if got, want := parsed.Query().Get("code_challenge"), pkceChallengeForTest(); got != want {
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
	ms := &ManageServer{csrfToken: "csrf"}
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/upgrade?csrf=csrf", nil)
	ms.handleAuthUpgrade(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestManageServer_HandleAuthUpgrade_CredentialsError(t *testing.T) {
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{}, errBoom
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/upgrade?email=test@example.com&csrf=csrf", nil)
	ms := &ManageServer{csrfToken: "csrf"}
	ms.handleAuthUpgrade(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServer_HandleAuthUpgrade_RejectsBadRequest(t *testing.T) {
	ms := &ManageServer{csrfToken: "csrf"}

	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{name: "bad method", method: http.MethodPost, target: "/auth/upgrade?email=test@example.com&csrf=csrf", want: http.StatusMethodNotAllowed},
		{name: "missing csrf", method: http.MethodGet, target: "/auth/upgrade?email=test@example.com", want: http.StatusForbidden},
		{name: "bad csrf", method: http.MethodGet, target: "/auth/upgrade?email=test@example.com&csrf=bad", want: http.StatusForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.target, nil)
			ms.handleAuthUpgrade(rr, req)

			if rr.Code != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, rr.Code)
			}
		})
	}
}
