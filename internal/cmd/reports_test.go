package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	reports "google.golang.org/api/admin/reports/v1"
	"google.golang.org/api/option"
)

func TestReportsUserCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/activity/users/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        map[string]any{"time": "1700000000"},
					"actor":     map[string]any{"email": "sam@example.com"},
					"ipAddress": "1.2.3.4",
					"events": []map[string]any{
						{"name": "login"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUserCmd{Date: "2026-01-02"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "sam@example.com") || !strings.Contains(out, "login") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestReportsUsageCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/usage/dates/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"usageReports": []map[string]any{
				{
					"date": "2026-01-02",
					"entity": map[string]any{
						"type":       "CUSTOMER",
						"customerId": "my_customer",
					},
					"parameters": []map[string]any{
						{"name": "num_users", "intValue": "42"},
					},
				},
			},
		})
	})
	stubReports(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &ReportsUsageCmd{Application: "gmail", Date: "2026-01-02", Parameters: "num_users"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "num_users=42") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubReports(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newReportsService
	svc, err := reports.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new reports service: %v", err)
	}
	newReportsService = func(context.Context, string) (*reports.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newReportsService = orig
		srv.Close()
	})
	return srv
}
