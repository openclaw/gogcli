package googleapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/authclient"
)

func TestQuotaProjectClients_SetHeader(t *testing.T) {
	for _, tt := range []struct {
		name      string
		generated bool
		adc       bool
	}{
		{name: "raw_direct_token"},
		{name: "generated_direct_token", generated: true},
		{name: "generated_adc", generated: true, adc: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)

				if got := r.Header.Get("X-Goog-User-Project"); got != "my-quota-project" {
					t.Errorf("X-Goog-User-Project = %q, want my-quota-project", got)
				}

				if got := r.Header.Get("Authorization"); got != "Bearer quota-test-token" {
					t.Errorf("Authorization = %q, want Bearer quota-test-token", got)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			ctx = authclient.WithQuotaProject(ctx, "my-quota-project")
			if tt.adc {
				ctx = WithAuthDependencies(ctx, AuthDependencies{
					Mode: AuthModeADC,
					ADCTokenSource: func(context.Context, ...string) (oauth2.TokenSource, error) {
						return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "quota-test-token"}), nil
					},
				})
			} else {
				ctx = authclient.WithAccessToken(ctx, "quota-test-token")
			}

			if tt.generated {
				svc, err := NewTasks(ctx, "a@b.com")
				if err != nil {
					t.Fatalf("NewTasks: %v", err)
				}

				svc.BasePath = srv.URL + "/"
				if _, err := svc.Tasklists.List().Context(ctx).Do(); err != nil {
					t.Fatalf("tasklists list: %v", err)
				}
			} else {
				client, err := NewHTTPClientForScopes(ctx, "svc", "a@b.com", []string{"s1"})
				if err != nil {
					t.Fatalf("NewHTTPClientForScopes: %v", err)
				}

				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
				if err != nil {
					t.Fatalf("new request: %v", err)
				}

				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("do request: %v", err)
				}
				_ = resp.Body.Close()
			}

			if got := requests.Load(); got != 1 {
				t.Fatalf("requests = %d, want 1", got)
			}
		})
	}
}

func TestOptionsForServiceAccountScopes_ADCModeSetsQuotaProjectHeader(t *testing.T) {
	var gotQuotaProject string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuotaProject = r.Header.Get("X-Goog-User-Project")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ctx := WithAuthDependencies(context.Background(), AuthDependencies{
		Mode: AuthModeADC,
		ADCTokenSource: func(context.Context, ...string) (oauth2.TokenSource, error) {
			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}), nil
		},
	})
	ctx = authclient.WithQuotaProject(ctx, "my-quota-project")

	opts, err := optionsForServiceAccountScopes(ctx, "tasks", "adc", []string{"s1"})
	if err != nil {
		t.Fatalf("optionsForServiceAccountScopes: %v", err)
	}

	svc, err := tasks.NewService(ctx, append(opts, option.WithEndpoint(srv.URL))...)
	if err != nil {
		t.Fatalf("tasks.NewService: %v", err)
	}

	if _, err := svc.Tasklists.List().Do(); err != nil {
		t.Fatalf("tasklists list: %v", err)
	}

	if gotQuotaProject != "my-quota-project" {
		t.Fatalf("X-Goog-User-Project = %q, want my-quota-project", gotQuotaProject)
	}
}

func TestQuotaProjectTransport_ClonesRequestAndPreservesBaseError(t *testing.T) {
	ctx := authclient.WithQuotaProject(context.Background(), "my-quota-project")
	var forwarded *http.Request

	transport := quotaProjectTransportFromContext(ctx, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		forwarded = req
		req.Header.Set("X-Test", "changed")

		return nil, errBoom
	}))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.Header.Set("X-Test", "original")

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if err != errBoom { //nolint:err113,errorlint // Verify exact error identity, not just an unwrap match.
		t.Fatalf("error = %v, want unchanged base error", err)
	}

	if forwarded == nil || forwarded == req {
		t.Fatal("expected a cloned request at the base transport")
	}

	if forwarded.Context() != req.Context() {
		t.Fatal("request context changed")
	}

	if got := forwarded.Header.Get("X-Goog-User-Project"); got != "my-quota-project" {
		t.Fatalf("forwarded X-Goog-User-Project = %q, want my-quota-project", got)
	}

	if got := req.Header.Get("X-Goog-User-Project"); got != "" {
		t.Fatalf("original request gained X-Goog-User-Project = %q", got)
	}

	if got := req.Header.Get("X-Test"); got != "original" {
		t.Fatalf("original request header changed to %q", got)
	}
}

func TestNewHTTPClientForScopes_QuotaProjectPreservesRetryAndReadOnly(t *testing.T) {
	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := requests.Add(1)

		if got := r.Header.Get("X-Goog-User-Project"); got != "my-quota-project" {
			t.Errorf("X-Goog-User-Project = %q, want my-quota-project", got)
		}

		if attempt == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)

			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctx = authclient.WithAccessToken(ctx, "quota-test-token")
	ctx = authclient.WithQuotaProject(ctx, "my-quota-project")
	ctx = WithReadOnly(ctx, true)

	client, err := NewHTTPClientForScopes(ctx, "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("NewHTTPClientForScopes: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK || requests.Load() != 2 {
		t.Fatalf("status = %d, requests = %d, want 200 after 2 attempts", resp.StatusCode, requests.Load())
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodDelete, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err = client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("DELETE error = %v, want ErrReadOnly", err)
	}

	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2; read-only DELETE reached the server", got)
	}
}

func TestNewHTTPClientForScopes_KeepsCallerQuotaProjectHeader(t *testing.T) {
	var gotQuotaProject []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuotaProject = r.Header.Values("X-Goog-User-Project")

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := authclient.WithAccessToken(context.Background(), "ya29.test-access-token")
	ctx = authclient.WithQuotaProject(ctx, "context-project")

	client, err := NewHTTPClientForScopes(ctx, "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("NewHTTPClientForScopes: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.Header.Set("X-Goog-User-Project", "caller-project")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if len(gotQuotaProject) != 1 || gotQuotaProject[0] != "caller-project" {
		t.Fatalf("X-Goog-User-Project = %v, want [caller-project]", gotQuotaProject)
	}
}

func TestNewHTTPClientForScopes_NoQuotaProjectOmitsHeader(t *testing.T) {
	var sawHeader bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawHeader = r.Header["X-Goog-User-Project"]

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := authclient.WithAccessToken(context.Background(), "ya29.test-access-token")

	client, err := NewHTTPClientForScopes(ctx, "svc", "a@b.com", []string{"s1"})
	if err != nil {
		t.Fatalf("NewHTTPClientForScopes: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if sawHeader {
		t.Fatal("unexpected X-Goog-User-Project header without a quota project")
	}
}
