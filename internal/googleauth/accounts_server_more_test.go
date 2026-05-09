package googleauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

var errTestStoreBoom = errors.New("boom")

func TestHandleAccountsPage(t *testing.T) {
	ms := &ManageServer{csrfToken: "csrf123"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	ms.handleAccountsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "csrf123") {
		t.Fatalf("expected csrf token in page")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nope", nil)
	ms.handleAccountsPage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for bad path")
	}
}

func TestFetchUserEmailDefault(t *testing.T) {
	if _, err := fetchUserEmailDefault(context.TODO(), nil); err == nil {
		t.Fatalf("expected missing token error")
	}

	if _, err := fetchUserEmailDefault(context.TODO(), &oauth2.Token{}); err == nil {
		t.Fatalf("expected missing access token error")
	}

	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"a@b.com"}`))
	idToken := "x." + payload + ".y"
	tok := &oauth2.Token{AccessToken: "access"}
	tok = tok.WithExtra(map[string]any{"id_token": idToken})

	email, err := fetchUserEmailDefault(context.TODO(), tok)
	if err != nil {
		t.Fatalf("fetchUserEmailDefault: %v", err)
	}

	if email != "a@b.com" {
		t.Fatalf("unexpected email: %q", email)
	}
}

func TestReadHTTPBodySnippet(t *testing.T) {
	out := readHTTPBodySnippet(strings.NewReader(""), 10)
	if out != "" {
		t.Fatalf("expected empty snippet")
	}

	out = readHTTPBodySnippet(strings.NewReader("access_token=secret"), 100)
	if !strings.Contains(out, "response_sha256=") {
		t.Fatalf("expected redacted hash, got: %q", out)
	}
}

func TestRenderSuccessPageWithDetails_More(t *testing.T) {
	rec := httptest.NewRecorder()
	renderSuccessPageWithDetails(rec, "a@b.com", []string{"gmail"})

	if !strings.Contains(rec.Body.String(), "a@b.com") {
		t.Fatalf("expected email in success page")
	}
}

func TestManageServerHandleOAuthCallback_ReadCredsError(t *testing.T) {
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{}, errTestStoreBoom
	}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		oauthState: "state1",
		listener:   ln,
		store:      &fakeStore{},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_ScopesError(t *testing.T) {
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		oauthState: "state1",
		listener:   ln,
		store:      &fakeStore{},
		opts:       ManageServerOptions{Services: []Service{Service("nope")}},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_ExchangeError(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))

	t.Cleanup(srv.Close)

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		oauthState: "state1",
		listener:   ln,
		store:      &fakeStore{},
		opts:       ManageServerOptions{Services: []Service{ServiceGmail}},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_MissingRefreshToken(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))

	t.Cleanup(srv.Close)

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		oauthState: "state1",
		listener:   ln,
		store:      &fakeStore{},
		opts:       ManageServerOptions{Services: []Service{ServiceGmail}},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_FetchEmailError(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})

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

	t.Cleanup(srv.Close)

	oauthEndpoint = oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	ms := &ManageServer{
		oauthState: "state1",
		listener:   ln,
		store:      &fakeStore{},
		fetchIdentity: func(context.Context, *oauth2.Token) (Identity, error) {
			return Identity{}, errTestStoreBoom
		},
		opts: ManageServerOptions{Services: []Service{ServiceGmail}},
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestStartManageServerOpenStoreError(t *testing.T) {
	origStore := openDefaultStore

	t.Cleanup(func() { openDefaultStore = origStore })

	openDefaultStore = func() (secrets.Store, error) {
		return nil, errTestStoreBoom
	}

	if err := StartManageServer(context.Background(), ManageServerOptions{Timeout: time.Second}); err == nil {
		t.Fatalf("expected error")
	}
}
