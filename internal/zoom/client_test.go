//nolint:wsl_v5
package zoom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type rewriteTransport struct {
	target string
	base   http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	targetURL, err := url.Parse(rt.target)
	if err != nil {
		return nil, fmt.Errorf("parse test target url: %w", err)
	}
	clone.URL.Scheme = targetURL.Scheme
	clone.URL.Host = targetURL.Host
	resp, err := rt.base.RoundTrip(clone)
	if err != nil {
		return nil, fmt.Errorf("round trip rewritten zoom request: %w", err)
	}
	return resp, nil
}

func newTestClient(t *testing.T, srv *httptest.Server, alias string, now time.Time) *Client {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "test-pass")
	client, err := NewClient(alias, Credentials{
		AccountID:    "acct",
		ClientID:     "client",
		ClientSecret: "secret",
	}, WithRoundTripper(rewriteTransport{target: srv.URL, base: srv.Client().Transport}), WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestClientTokenRefreshUsesCachedTokenUntilSkew(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	tokenRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/oauth/token":
			tokenRequests++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/users/me/meetings":
			if got := r.Header.Get("Authorization"); got != "Bearer tok" {
				t.Fatalf("Authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 123, "join_url": "https://example.zoom.us/j/123?pwd=abc"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "cache", now)
	if _, err := client.CreateMeeting(context.Background(), "me", CreateMeetingRequest{}); err != nil {
		t.Fatalf("CreateMeeting first: %v", err)
	}
	if _, err := client.CreateMeeting(context.Background(), "me", CreateMeetingRequest{}); err != nil {
		t.Fatalf("CreateMeeting cached: %v", err)
	}
	if tokenRequests != 1 {
		t.Fatalf("tokenRequests = %d, want 1", tokenRequests)
	}

	if err := StoreCachedToken("cache", CachedToken{AccessToken: "stale", ExpiresAt: now.Add(30 * time.Second)}); err != nil {
		t.Fatalf("StoreCachedToken: %v", err)
	}
	if _, err := client.CreateMeeting(context.Background(), "me", CreateMeetingRequest{}); err != nil {
		t.Fatalf("CreateMeeting refresh: %v", err)
	}
	if tokenRequests != 2 {
		t.Fatalf("tokenRequests = %d, want 2", tokenRequests)
	}
}

func TestClientDeleteMeetingIgnores404And410(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/token" {
					_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
					return
				}
				w.WriteHeader(status)
			}))
			defer srv.Close()
			client := newTestClient(t, srv, strings.ReplaceAll(http.StatusText(status), " ", "-"), time.Now())
			if err := client.DeleteMeeting(context.Background(), "123"); err != nil {
				t.Fatalf("DeleteMeeting: %v", err)
			}
		})
	}
}

func TestClientDeleteMeeting5xxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := newTestClient(t, srv, "fivehundred", time.Now())
	err := client.DeleteMeeting(context.Background(), "123")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("DeleteMeeting error = %v, want 500", err)
	}
}

func TestRedactZoomURL(t *testing.T) {
	t.Setenv("GOG_ZOOM_INCLUDE_PASSWORDS", "")
	got := RedactZoomURL("https://example.zoom.us/j/123?pwd=secret&x=1")
	if strings.Contains(got, "secret") || !strings.Contains(got, "pwd=REDACTED") {
		t.Fatalf("RedactZoomURL = %q", got)
	}
	t.Setenv("GOG_ZOOM_INCLUDE_PASSWORDS", "1")
	if !IncludePasswordsFromEnv() {
		t.Fatalf("expected include passwords env")
	}
}
