package googleauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

var errTestStoreBoom = errors.New("boom")

func TestHandleAccountsPage(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{})
	ms.csrfToken = "csrf123"
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
	req = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	ms.handleAccountsPage(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for bad method, got %d", rec.Code)
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
	ms := newTestManagerApplication(t, ManagerOptions{}, ManagerDependencies{
		ReadCredentials: func(string) (config.ClientCredentials, error) {
			return config.ClientCredentials{}, errTestStoreBoom
		},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_ScopesError(t *testing.T) {
	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{Service("nope")},
	}, ManagerDependencies{})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_ExchangeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))

	t.Cleanup(srv.Close)

	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_MissingRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))

	t.Cleanup(srv.Close)

	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestManageServerHandleOAuthCallback_FetchEmailError(t *testing.T) {
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

	ms := newTestManagerApplication(t, ManagerOptions{
		Services: []Service{ServiceGmail},
	}, ManagerDependencies{
		FetchIdentity: func(context.Context, *oauth2.Token) (Identity, error) {
			return Identity{}, errTestStoreBoom
		},
		OAuthEndpoint: oauth2.Endpoint{AuthURL: "http://example.com/auth", TokenURL: srv.URL},
	})
	ms.addOAuthState("state1", testCodeVerifier)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/oauth2/callback?state=state1&code=abc", nil)
	ms.handleOAuthCallback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestStartManageServerOpenStoreError(t *testing.T) {
	launcher := newTestManagerLauncher(t, func(deps *ManagerLauncherDependencies) {
		deps.OpenTokens = func(context.Context) (secrets.Store, error) {
			return nil, errTestStoreBoom
		}
	})

	if err := launcher.Start(context.Background(), ManageServerOptions{Timeout: time.Second}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestManageCredentialsReaderUsesContext(t *testing.T) {
	origRead := readClientCredentials

	t.Cleanup(func() { readClientCredentials = origRead })
	readClientCredentials = nil

	ctx := authclient.WithCredentialsReader(context.Background(), func(client string) (config.ClientCredentials, error) {
		if client != "work" {
			t.Fatalf("client = %q", client)
		}

		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	})

	credentials, err := manageCredentialsReader(ctx, nil)("work")
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}

	if credentials.ClientID != "id" || credentials.ClientSecret != "secret" {
		t.Fatalf("credentials = %#v", credentials)
	}
}

func TestManageCredentialsReaderPreservesOverride(t *testing.T) {
	called := false
	reader := manageCredentialsReader(context.Background(), func(client string) (config.ClientCredentials, error) {
		called = true
		return config.ClientCredentials{ClientID: client}, nil
	})

	credentials, err := reader("custom")
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}

	if !called || credentials.ClientID != "custom" {
		t.Fatalf("called = %v, credentials = %#v", called, credentials)
	}
}
