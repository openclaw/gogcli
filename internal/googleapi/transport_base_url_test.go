package googleapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/openclaw/gogcli/internal/authclient"
)

type recordingRoundTripper struct {
	req *http.Request
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.req = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

func TestAPIBaseURLTransportRewritesOnlyGoogleAPIs(t *testing.T) {
	for _, tt := range []struct {
		in, want, forwardedHost string
	}{
		{"https://gmail.googleapis.com/gmail/v1/users/me/messages?q=a%20b", "http://127.0.0.1:18080/gmail/v1/users/me/messages?q=a%20b", "gmail.googleapis.com"},
		{"https://GoogleAPIs.com/v1/x", "http://127.0.0.1:18080/v1/x", "GoogleAPIs.com"},
		{"https://googleapis.com.evil.test/v1/x", "https://googleapis.com.evil.test/v1/x", ""},
		{"https://lh3.googleusercontent.com/x", "https://lh3.googleusercontent.com/x", ""},
		{"http://www.googleapis.com/v1/x", "http://www.googleapis.com/v1/x", ""},
	} {
		rec := &recordingRoundTripper{}
		transport := &apiBaseURLTransport{base: rec, baseURL: &url.URL{Scheme: "http", Host: "127.0.0.1:18080"}}

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, tt.in, nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()

		if got := rec.req.URL.String(); got != tt.want {
			t.Errorf("%s: URL = %q, want %q", tt.in, got, tt.want)
		}

		if got := rec.req.Header.Get("X-Forwarded-Host"); got != tt.forwardedHost {
			t.Errorf("%s: X-Forwarded-Host = %q, want %q", tt.in, got, tt.forwardedHost)
		}

		if req.URL.String() != tt.in {
			t.Errorf("%s: original request mutated to %q", tt.in, req.URL)
		}
	}
}

func TestAPIBaseURLClientsSendNoCredentialsAndRefuseCrossOriginRedirects(t *testing.T) {
	var stolen atomic.Int32

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stolen.Add(1)
	}))
	defer other.Close()

	var relayed atomic.Int32

	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayed.Add(1)

		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("relay got Authorization %q", got)
		}

		if got := r.URL.Query().Get("key"); got != "" {
			t.Errorf("relay got API key %q", got)
		}

		http.Redirect(w, r, other.URL+"/collect", http.StatusFound)
	}))
	defer relay.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctx = authclient.WithAccessToken(ctx, "test-token")

	ctx, err := WithAPIBaseURL(ctx, relay.URL)
	if err != nil {
		t.Fatal(err)
	}

	adcCtx := WithAuthDependencies(ctx, AuthDependencies{
		Mode: AuthModeADC,
		ADCTokenSource: func(context.Context, ...string) (oauth2.TokenSource, error) {
			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}), nil
		},
	})

	//nolint:wrapcheck // errors.Is checks the refusal through each client unchanged
	for name, call := range map[string]func() error{
		"raw_access_token": func() error {
			client, err := NewHTTPClientForScopes(ctx, "svc", "a@b.com", []string{"s1"})
			if err != nil {
				return err
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://gmail.googleapis.com/gmail/v1/users/me/profile", nil)
			if err != nil {
				return err
			}

			resp, err := client.Do(req)
			if resp != nil {
				_ = resp.Body.Close()
			}

			return err
		},
		"generated_adc": func() error {
			svc, err := NewTasks(adcCtx, "a@b.com")
			if err != nil {
				return err
			}
			_, err = svc.Tasklists.List().Context(adcCtx).Do()

			return err
		},
		"youtube_api_key": func() error {
			svc, err := NewYouTubeWithAPIKey(ctx, "test-key")
			if err != nil {
				return err
			}
			_, err = svc.Channels.List([]string{"id"}).Id("c1").Context(ctx).Do()

			return err
		},
		"service_account_scopes": func() error {
			svc, err := NewCloudIdentityGroups(ctx, "a@b.com")
			if err != nil {
				return err
			}
			_, err = svc.Groups.Get("groups/g1").Context(ctx).Do()

			return err
		},
		"keep_service_account": func() error {
			svc, err := newKeepWithServiceAccount(ctx, t.TempDir()+"/missing-key.json", "a@b.com", func(context.Context, []byte, string, []string) (oauth2.TokenSource, error) {
				return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "sa-token"}), nil
			})
			if err != nil {
				return err
			}
			_, err = svc.Notes.List().Context(ctx).Do()

			return err
		},
	} {
		before := relayed.Load()

		if err := call(); !errors.Is(err, errCrossOriginRedirect) {
			t.Errorf("%s: error = %v, want cross-origin redirect refusal", name, err)
		}

		if relayed.Load() == before {
			t.Errorf("%s: request did not reach the base URL", name)
		}
	}

	if n := stolen.Load(); n != 0 {
		t.Fatalf("redirect target received %d requests", n)
	}

	if _, err := NewYouTubeWithAPIKey(ctx, ""); err != nil {
		t.Fatalf("NewYouTubeWithAPIKey without key: %v", err)
	}
}

func TestAPIBaseURLTransportFollowsSameOriginRedirects(t *testing.T) {
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Forwarded-Host"); got != "www.googleapis.com" {
			t.Errorf("%s: X-Forwarded-Host = %q", r.URL.Path, got)
		}

		switch r.URL.Path {
		case "/v1/relative":
			http.Redirect(w, r, "/v1/absolute", http.StatusFound)
		case "/v1/absolute":
			http.Redirect(w, r, "http://"+r.Host+"/v1/done", http.StatusFound)
		}
	}))
	defer relay.Close()

	ctx, err := WithAPIBaseURL(context.Background(), relay.URL)
	if err != nil {
		t.Fatal(err)
	}

	client, ok := apiBaseURLHTTPClient(ctx)
	if !ok {
		t.Fatal("base URL client not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/v1/relative", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK || resp.Request.URL.String() != "https://www.googleapis.com/v1/done" {
		t.Fatalf("status = %d, final URL = %s", resp.StatusCode, resp.Request.URL)
	}
}

func TestAPIBaseURLReadOnlyPOSTFollowsRelayRedirect(t *testing.T) {
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "" {
			http.Redirect(w, r, "http://"+r.Host+r.URL.Path+"?moved=1", http.StatusTemporaryRedirect)
		}
	}))
	defer relay.Close()

	ctx, err := WithAPIBaseURL(WithReadOnly(context.Background(), true), relay.URL)
	if err != nil {
		t.Fatal(err)
	}

	client, ok := apiBaseURLHTTPClient(ctx)
	if !ok {
		t.Fatal("base URL client not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.googleapis.com/calendar/v3/freeBusy", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("read-only POST through relay redirect: %v", err)
	}
	_ = resp.Body.Close()
}

func TestWithAPIBaseURLRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1:18080",
		"localhost",
		"ftp://127.0.0.1",
		"http://",
		"http://127.0.0.1/prefix",
		"http://127.0.0.1/?x=1",
		"http://leaky-user@127.0.0.1",
		"http://:18080",
		"http://127.0.0.1:leaky-port",
	} {
		_, err := WithAPIBaseURL(context.Background(), raw)
		if err == nil {
			t.Errorf("WithAPIBaseURL(%q) succeeded, want error", raw)
		} else if strings.Contains(err.Error(), "leaky") || strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("WithAPIBaseURL(%q) error repeats the value: %v", raw, err)
		}
	}
}
