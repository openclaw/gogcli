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

	"github.com/steipete/gogcli/internal/outfmt"
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

func TestAlertsListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta1/alerts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alerts": []map[string]any{
				{
					"alertId":    "alert-json-1",
					"type":       "SUSPICIOUS_LOGIN",
					"source":     "DRIVE",
					"createTime": "2026-01-02T00:00:00Z",
					"updateTime": "2026-01-02T00:00:00Z",
					"deleted":    false,
				},
			},
		})
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsListCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "alert-json-1") || !strings.Contains(out, "SUSPICIOUS_LOGIN") {
		t.Fatalf("expected JSON output with alert data, got: %s", out)
	}
}

func TestAlertsListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta1/alerts") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alerts": []map[string]any{},
		})
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsListCmd{}

	// No error expected, just "no alerts" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAlertsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-get-1") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alertId":    "alert-get-1",
			"type":       "PHISHING",
			"source":     "GMAIL",
			"createTime": "2026-01-03T00:00:00Z",
			"updateTime": "2026-01-03T01:00:00Z",
			"deleted":    false,
			"startTime":  "2026-01-03T00:00:00Z",
			"endTime":    "2026-01-03T02:00:00Z",
		})
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsGetCmd{AlertID: "alert-get-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "alert-get-1") || !strings.Contains(out, "PHISHING") {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, "Start Time:") || !strings.Contains(out, "End Time:") {
		t.Fatalf("expected start/end times in output: %s", out)
	}
}

func TestAlertsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-get-json") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"alertId":    "alert-get-json",
			"type":       "MALWARE",
			"source":     "DRIVE",
			"createTime": "2026-01-04T00:00:00Z",
			"updateTime": "2026-01-04T01:00:00Z",
			"deleted":    false,
		})
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsGetCmd{AlertID: "alert-get-json"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "alert-get-json") || !strings.Contains(out, "MALWARE") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAlertsDeleteCmd(t *testing.T) {
	deleted := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-delete-1") {
			deleted = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com", Force: true}
	cmd := &AlertsDeleteCmd{AlertID: "alert-delete-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !deleted {
		t.Fatal("expected delete API call")
	}
	if !strings.Contains(out, "Deleted alert") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAlertsDeleteCmd_RequiresConfirmation(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com", NoInput: true}
	cmd := &AlertsDeleteCmd{AlertID: "alert-delete-1"}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when NoInput is set without Force")
	}
}

func TestAlertsUndeleteCmd(t *testing.T) {
	undeleted := false
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-undelete-1:undelete") {
			undeleted = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"alertId": "alert-undelete-1",
				"deleted": false,
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsUndeleteCmd{AlertID: "alert-undelete-1"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !undeleted {
		t.Fatal("expected undelete API call")
	}
	if !strings.Contains(out, "Undeleted alert") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAlertsFeedbackListCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-fb-list/feedback") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"feedback": []map[string]any{
					{
						"feedbackId": "fb-1",
						"type":       "VERY_USEFUL",
						"email":      "user@example.com",
						"createTime": "2026-01-05T00:00:00Z",
					},
					{
						"feedbackId": "fb-2",
						"type":       "NOT_USEFUL",
						"email":      "user2@example.com",
						"createTime": "2026-01-05T01:00:00Z",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsFeedbackListCmd{AlertID: "alert-fb-list"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContext(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "fb-1") || !strings.Contains(out, "VERY_USEFUL") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAlertsFeedbackListCmd_RequiresAlertID(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsFeedbackListCmd{AlertID: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when AlertID is empty")
	}
}

func TestAlertsFeedbackListCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-fb-json/feedback") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"feedback": []map[string]any{
					{
						"feedbackId": "fb-json-1",
						"type":       "SOMEWHAT_USEFUL",
						"email":      "json@example.com",
						"createTime": "2026-01-06T00:00:00Z",
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsFeedbackListCmd{AlertID: "alert-fb-json"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "fb-json-1") || !strings.Contains(out, "SOMEWHAT_USEFUL") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAlertsFeedbackListCmd_Empty(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-fb-empty/feedback") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"feedback": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsFeedbackListCmd{AlertID: "alert-fb-empty"}

	// No error expected, just "no feedback" message
	if err := cmd.Run(testContext(t), flags); err != nil {
		t.Fatalf("Run: %v", err)
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

func TestAlertsFeedbackCreateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/v1beta1/alerts/alert-create-json/feedback") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"feedbackId": "fb-created-json",
				"type":       "NOT_USEFUL",
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsFeedbackCreateCmd{AlertID: "alert-create-json", Type: "NOT_USEFUL"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "fb-created-json") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAlertsSettingsGetCmd(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1beta1/settings") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"notifications": []map[string]any{
					{
						"cloudPubsubTopic": map[string]any{
							"topicName": "projects/my-project/topics/alerts",
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsSettingsGetCmd{}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "Notifications:") || !strings.Contains(out, "projects/my-project/topics/alerts") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAlertsSettingsGetCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/v1beta1/settings") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"notifications": []map[string]any{
					{
						"cloudPubsubTopic": map[string]any{
							"topicName": "projects/test/topics/test-alerts",
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsSettingsGetCmd{}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "projects/test/topics/test-alerts") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func TestAlertsSettingsUpdateCmd(t *testing.T) {
	var gotTopics []string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/v1beta1/settings") {
			var payload struct {
				Notifications []struct {
					CloudPubsubTopic struct {
						TopicName string `json:"topicName"`
					} `json:"cloudPubsubTopic"`
				} `json:"notifications"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, n := range payload.Notifications {
				gotTopics = append(gotTopics, n.CloudPubsubTopic.TopicName)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"notifications": payload.Notifications,
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsSettingsUpdateCmd{Notifications: "projects/p1/topics/t1,projects/p2/topics/t2"}

	out := captureStdout(t, func() {
		if err := cmd.Run(testContextWithStdout(t), flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if len(gotTopics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(gotTopics))
	}
	if gotTopics[0] != "projects/p1/topics/t1" || gotTopics[1] != "projects/p2/topics/t2" {
		t.Fatalf("unexpected topics: %v", gotTopics)
	}
	if !strings.Contains(out, "Updated alert settings") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestAlertsSettingsUpdateCmd_RequiresNotifications(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsSettingsUpdateCmd{Notifications: ""}

	err := cmd.Run(testContext(t), flags)
	if err == nil {
		t.Fatal("expected error when Notifications is empty")
	}
}

func TestAlertsSettingsUpdateCmd_JSON(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/v1beta1/settings") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"notifications": []map[string]any{
					{
						"cloudPubsubTopic": map[string]any{
							"topicName": "projects/json/topics/alerts",
						},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	stubAlertCenter(t, h)

	flags := &RootFlags{Account: "admin@example.com"}
	cmd := &AlertsSettingsUpdateCmd{Notifications: "projects/json/topics/alerts"}

	ctx := testContext(t)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	if !strings.Contains(out, "projects/json/topics/alerts") {
		t.Fatalf("expected JSON output, got: %s", out)
	}
}

func stubAlertCenter(t *testing.T, handler http.Handler) {
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
}
