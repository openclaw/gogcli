package googleauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/steipete/gogcli/internal/config"
)

var errMissingRedirectState = errors.New("missing redirect/state")

const testRedirectURI = "http://127.0.0.1:55555/oauth2/callback"

func useManualRedirectURI(t *testing.T) {
	t.Helper()

	orig := manualRedirectURIFn
	manualRedirectURIFn = func(context.Context) (string, error) { return testRedirectURI, nil }

	t.Cleanup(func() {
		manualRedirectURIFn = orig
	})
}

func useStdinPipe(t *testing.T) *os.File {
	t.Helper()

	orig := os.Stdin

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r

	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})

	return w
}

func newTokenServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}

		if r.Form.Get("grant_type") != "authorization_code" {
			http.Error(w, "bad grant_type", http.StatusBadRequest)
			return
		}

		if r.Form.Get("code") == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "at",
			"refresh_token": "rt",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
}

func useTempManualStatePath(t *testing.T) {
	t.Helper()

	origDir := manualStateDirFn
	dir := t.TempDir()
	manualStateDirFn = func() (string, error) { return dir, nil }

	t.Cleanup(func() {
		manualStateDirFn = origDir
	})
}

func TestAuthorize_MissingScopes(t *testing.T) {
	_, err := Authorize(context.Background(), AuthorizeOptions{})
	if err == nil || !strings.Contains(err.Error(), "missing scopes") {
		t.Fatalf("expected missing scopes error, got: %v", err)
	}
}

func TestAuthorize_Manual_Success(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})

	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }

	useManualRedirectURI(t)

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	w := useStdinPipe(t)
	_, _ = w.WriteString("http://127.0.0.1:55555/oauth2/callback?code=abc&state=state123\n")
	_ = w.Close()

	rt, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Manual:  true,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if rt != "rt" {
		t.Fatalf("unexpected refresh token: %q", rt)
	}
}

func TestAuthorize_Manual_Success_NoNewline(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }

	useManualRedirectURI(t)

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	w := useStdinPipe(t)
	_, _ = w.WriteString("http://127.0.0.1:55555/oauth2/callback?code=abc&state=state123")
	_ = w.Close()

	rt, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Manual:  true,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if rt != "rt" {
		t.Fatalf("unexpected refresh token: %q", rt)
	}
}

func TestAuthorize_Manual_CancelEOF(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }

	useManualRedirectURI(t)

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	w := useStdinPipe(t)
	_ = w.Close()

	_, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Manual:  true,
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestAuthorize_Manual_StateMismatch(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	randomStateFn = func() (string, error) { return "state123", nil }

	useManualRedirectURI(t)

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	w := useStdinPipe(t)
	_, _ = w.WriteString("http://127.0.0.1:55555/oauth2/callback?code=abc&state=DIFFERENT\n")
	_ = w.Close()

	if _, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Manual:  true,
		Timeout: 2 * time.Second,
	}); err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got: %v", err)
	}
}

func TestAuthorize_Manual_AuthCode(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origState := randomStateFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		randomStateFn = origState
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	if err := saveManualState("", []string{"s1"}, false, "state123", testRedirectURI); err != nil {
		t.Fatalf("save manual state: %v", err)
	}

	stateCalled := false
	randomStateFn = func() (string, error) {
		stateCalled = true
		return "state123", nil
	}

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	rt, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:   []string{"s1"},
		Manual:   true,
		AuthCode: "abc",
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if rt != "rt" {
		t.Fatalf("unexpected refresh token: %q", rt)
	}

	if stateCalled {
		t.Fatalf("unexpected state generation in auth-code flow")
	}
}

func TestAuthorize_Manual_AuthURL_RequireStateMissing(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")

	_, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:       []string{"s1"},
		Manual:       true,
		AuthURL:      "http://127.0.0.1:55555/oauth2/callback?code=abc",
		RequireState: true,
		Client:       "default",
		Timeout:      2 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, errMissingState) {
		t.Fatalf("expected missing state error, got: %v", err)
	}
}

func TestAuthorize_Manual_AuthURL_RequireStateMissingCache(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")

	_, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:       []string{"s1"},
		Manual:       true,
		AuthURL:      "http://127.0.0.1:55555/oauth2/callback?code=abc&state=state123",
		RequireState: true,
		Client:       "default",
		Timeout:      2 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, errManualStateMissing) {
		t.Fatalf("expected manual state missing error, got: %v", err)
	}
}

func TestAuthorize_Manual_AuthURL_RequireStateMissingForDifferentState(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
	})
	useTempManualStatePath(t)

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}
	oauthEndpoint = oauth2EndpointForTest("http://example.com")

	if err := saveManualState("default", []string{"s1"}, false, "state123", "http://127.0.0.1:55555/oauth2/callback"); err != nil {
		t.Fatalf("save manual state: %v", err)
	}

	_, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:       []string{"s1"},
		Manual:       true,
		AuthURL:      "http://127.0.0.1:55555/oauth2/callback?code=abc&state=DIFFERENT",
		RequireState: true,
		Client:       "default",
		Timeout:      2 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, errManualStateMissing) {
		t.Fatalf("expected manual state missing error, got: %v", err)
	}
}

func TestAuthorize_ServerFlow_Success(t *testing.T) {
	origRead := readClientCredentials
	origEndpoint := oauthEndpoint
	origOpen := openBrowserFn

	t.Cleanup(func() {
		readClientCredentials = origRead
		oauthEndpoint = origEndpoint
		openBrowserFn = origOpen
	})

	readClientCredentials = func(string) (config.ClientCredentials, error) {
		return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
	}

	tokenSrv := newTokenServer(t)
	defer tokenSrv.Close()
	oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

	openBrowserFn = func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return fmt.Errorf("parse auth url: %w", err)
		}
		q := u.Query()
		redirect := q.Get("redirect_uri")

		var state string

		if s := q.Get("state"); redirect == "" || s == "" {
			return errMissingRedirectState
		} else {
			state = s
		}
		cb := redirect + "?code=abc&state=" + url.QueryEscape(state)

		var req *http.Request

		if r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, cb, nil); err != nil {
			return fmt.Errorf("build callback request: %w", err)
		} else {
			req = r
		}
		var resp *http.Response

		if r, err := http.DefaultClient.Do(req); err != nil {
			return fmt.Errorf("send callback request: %w", err)
		} else {
			resp = r
		}
		_ = resp.Body.Close()

		return nil
	}

	rt, err := Authorize(context.Background(), AuthorizeOptions{
		Scopes:  []string{"s1"},
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if rt != "rt" {
		t.Fatalf("unexpected refresh token: %q", rt)
	}
}

func TestAuthorize_ServerFlow_CallbackErrors(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantText string
	}{
		{name: "missing_code", query: "state=%s", wantText: "missing code"},
		{name: "state_mismatch", query: "code=abc&state=WRONG", wantText: "state mismatch"},
		{name: "oauth_error", query: "error=access_denied&state=%s", wantText: "authorization error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origRead := readClientCredentials
			origEndpoint := oauthEndpoint
			origOpen := openBrowserFn

			t.Cleanup(func() {
				readClientCredentials = origRead
				oauthEndpoint = origEndpoint
				openBrowserFn = origOpen
			})

			readClientCredentials = func(string) (config.ClientCredentials, error) {
				return config.ClientCredentials{ClientID: "id", ClientSecret: "secret"}, nil
			}

			tokenSrv := newTokenServer(t)
			defer tokenSrv.Close()
			oauthEndpoint = oauth2EndpointForTest(tokenSrv.URL)

			openBrowserFn = func(authURL string) error {
				u, err := url.Parse(authURL)
				if err != nil {
					return fmt.Errorf("parse auth url: %w", err)
				}
				q := u.Query()
				redirect := q.Get("redirect_uri")

				var state string

				if s := q.Get("state"); redirect == "" || s == "" {
					return errMissingRedirectState
				} else {
					state = s
				}
				var query string

				if q := tt.query; strings.Contains(q, "%s") {
					query = fmtSprintf(q, url.QueryEscape(state))
				} else {
					query = q
				}
				cb := redirect + "?" + query

				var req *http.Request

				if r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, cb, nil); err != nil {
					return fmt.Errorf("build callback request: %w", err)
				} else {
					req = r
				}
				var resp *http.Response

				if r, err := http.DefaultClient.Do(req); err != nil {
					return fmt.Errorf("send callback request: %w", err)
				} else {
					resp = r
				}
				_ = resp.Body.Close()

				return nil
			}

			_, err := Authorize(context.Background(), AuthorizeOptions{
				Scopes:  []string{"s1"},
				Timeout: 2 * time.Second,
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("expected %q error, got: %v", tt.wantText, err)
			}
		})
	}
}

// oauth2.Endpoint is a plain struct; keep construction centralized.
func oauth2EndpointForTest(base string) oauth2.Endpoint {
	return oauth2.Endpoint{
		AuthURL:  base + "/auth",
		TokenURL: base + "/token",
	}
}

// Minimal sprintf to avoid importing fmt just for one small helper in tests.
func fmtSprintf(format string, v string) string {
	return strings.ReplaceAll(format, "%s", v)
}
