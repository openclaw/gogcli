package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	"google.golang.org/api/option"
)

func TestAnalyticsAccountsCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta/accounts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{{"name": "accounts/123", "displayName": "Acme"}},
		})
	})
	stubAnalytics(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &AnalyticsAccountsCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Acme") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAnalyticsPropertiesCmd(t *testing.T) {
	var gotFilter string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta/properties") {
			http.NotFound(w, r)
			return
		}
		gotFilter = r.URL.Query().Get("filter")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"properties": []map[string]any{{"name": "properties/123", "displayName": "Web", "timeZone": "UTC"}},
		})
	})
	stubAnalytics(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &AnalyticsPropertiesCmd{AccountID: "123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotFilter != "parent:accounts/123" {
		t.Fatalf("unexpected filter: %q", gotFilter)
	}
	if !strings.Contains(out, "properties/123") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAnalyticsDataStreamsCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta/properties/123/dataStreams") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"dataStreams": []map[string]any{{"name": "properties/123/dataStreams/1", "displayName": "Web", "type": "WEB_DATA_STREAM"}},
		})
	})
	stubAnalytics(t, h)

	flags := &RootFlags{Account: "user@example.com"}
	cmd := &AnalyticsDataStreamsCmd{Property: "123"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "dataStreams/1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubAnalytics(t *testing.T, handler http.Handler) {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newAnalyticsAdminService
	svc, err := analyticsadmin.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new analytics service: %v", err)
	}
	newAnalyticsAdminService = func(context.Context, string) (*analyticsadmin.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newAnalyticsAdminService = orig
		srv.Close()
	})
}
