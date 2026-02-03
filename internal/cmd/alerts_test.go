package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	alertcenter "google.golang.org/api/alertcenter/v1beta1"
	"google.golang.org/api/option"
)

func TestAlertsListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta1/alerts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alerts": []map[string]any{
				{
					"alertId":    "alert-1",
					"type":       "BAD_LOGIN",
					"source":     "GMAIL",
					"createTime": "2026-01-01T00:00:00Z",
					"updateTime": "2026-01-01T00:00:00Z",
					"deleted":    false,
				},
			},
		})
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsListCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "alert-1") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAlertsFeedbackCreateCmd(t *testing.T) {
	var gotType string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-1/feedback"):
			var payload struct {
				Type string `json:"type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotType = payload.Type
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"feedbackId": "fb-1",
				"type":       payload.Type,
			})
			return
		default:
			http.NotFound(w, r)
		}
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsFeedbackCreateCmd{AlertID: "alert-1", Type: "VERY_USEFUL"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if gotType != "VERY_USEFUL" {
		t.Fatalf("expected feedback type VERY_USEFUL, got %q", gotType)
	}
	if !strings.Contains(out, "Created feedback") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func stubAlertCenter(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	orig := newAlertCenterService
	svc, err := alertcenter.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("new alertcenter service: %v", err)
	}
	newAlertCenterService = func(context.Context, string) (*alertcenter.Service, error) { return svc, nil }
	t.Cleanup(func() {
		newAlertCenterService = orig
		srv.Close()
	})
	return srv
}
