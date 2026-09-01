package googleapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/authclient"
)

func TestNewHTTPClientForScopes_SetsQuotaProjectHeader(t *testing.T) {
	var gotQuotaProject, gotAuthorization string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuotaProject = r.Header.Get("X-Goog-User-Project")
		gotAuthorization = r.Header.Get("Authorization")

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := authclient.WithAccessToken(context.Background(), "ya29.test-access-token")
	ctx = authclient.WithQuotaProject(ctx, "my-quota-project")

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

	if gotQuotaProject != "my-quota-project" {
		t.Fatalf("X-Goog-User-Project = %q, want my-quota-project", gotQuotaProject)
	}

	if gotAuthorization == "" {
		t.Fatal("expected Authorization header alongside quota project header")
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

func TestQuotaProjectTransport_PassesBaseErrorThroughUnchanged(t *testing.T) {
	ctx := authclient.WithQuotaProject(context.Background(), "my-quota-project")
	transport := quotaProjectTransportFromContext(ctx, &mockTransport{errors: []error{errBoom}})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}

	if !errors.Is(err, errBoom) {
		t.Fatalf("error = %v, want base error", err)
	}

	if err.Error() != errBoom.Error() {
		t.Fatalf("error message = %q, want %q unchanged", err.Error(), errBoom.Error())
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
