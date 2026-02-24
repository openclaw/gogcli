package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/tracking"
)

func setupTrackingEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")
}

func TestGmailTrackSetupAndStatus(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		errOut := captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "--no-input", "gmail", "track", "setup", "--worker-url", "https://example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
		if !strings.Contains(errOut, "Next steps") {
			t.Fatalf("expected next steps in stderr: %q", errOut)
		}
	})
	if !strings.Contains(out, "configured\ttrue") {
		t.Fatalf("unexpected setup output: %q", out)
	}

	statusOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(statusOut, "configured\ttrue") {
		t.Fatalf("unexpected status output: %q", statusOut)
	}
}

func TestGmailTrackStatus_NotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "status"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "configured\tfalse") {
		t.Fatalf("unexpected status output: %q", out)
	}
}

func TestGmailTrackOpens(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/q/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tracking_id": "tid",
				"recipient":   "user@example.com",
				"sent_at":     "2025-01-01T00:00:00Z",
				"total_opens": 2,
				"human_opens": 1,
				"first_human_open": map[string]any{
					"at": "2025-01-01T02:00:00Z",
					"location": map[string]any{
						"city":    "SF",
						"region":  "CA",
						"country": "US",
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/opens"):
			if r.Header.Get("Authorization") != "Bearer adminkey" {
				t.Fatalf("unexpected auth: %q", r.Header.Get("Authorization"))
			}
			if r.URL.Query().Get("recipient") != "user@example.com" {
				t.Fatalf("unexpected recipient: %q", r.URL.RawQuery)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{
					{
						"tracking_id":  "tid",
						"recipient":    "user@example.com",
						"subject_hash": "hash",
						"sent_at":      "2025-01-01T00:00:00Z",
						"opened_at":    "2025-01-01T01:00:00Z",
						"is_bot":       false,
						"location":     map[string]any{"city": "SF", "region": "CA", "country": "US"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "tid"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "tracking_id\ttid") {
		t.Fatalf("unexpected tracking id output: %q", out)
	}
	if !strings.Contains(out, "first_human_open\t2025-01-01T02:00:00Z") {
		t.Fatalf("unexpected first open output: %q", out)
	}
	if !strings.Contains(out, "first_human_open_location\tSF, CA") {
		t.Fatalf("unexpected first open location output: %q", out)
	}

	adminOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com", "--since", "2025-01-01"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(adminOut, "tid\tuser@example.com") {
		t.Fatalf("unexpected admin output: %q", adminOut)
	}

	if _, err := parseTrackingSince("not-a-date"); err == nil {
		t.Fatalf("expected parseTrackingSince error")
	}
}

func TestGmailTrackOpens_JSON(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/q/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tracking_id": "tid",
				"recipient":   "user@example.com",
				"sent_at":     "2025-01-01T00:00:00Z",
				"total_opens": 2,
				"human_opens": 1,
			})
			return
		case strings.Contains(r.URL.Path, "/opens"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{
					{
						"tracking_id":  "tid",
						"recipient":    "user@example.com",
						"subject_hash": "hash",
						"sent_at":      "2025-01-01T00:00:00Z",
						"opened_at":    "2025-01-01T01:00:00Z",
						"is_bot":       false,
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	trackOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "track", "opens", "tid"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(trackOut, "\"tracking_id\"") {
		t.Fatalf("unexpected track json output: %q", trackOut)
	}

	adminOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(adminOut, "\"opens\"") {
		t.Fatalf("unexpected admin json output: %q", adminOut)
	}

	if parsed, err := parseTrackingSince("24h"); err != nil || parsed == "" {
		t.Fatalf("unexpected parseTrackingSince duration result: %q err=%v", parsed, err)
	}
	if parsed, err := parseTrackingSince("2025-01-01"); err != nil || parsed == "" {
		t.Fatalf("unexpected parseTrackingSince date result: %q err=%v", parsed, err)
	}
}

func TestGmailTrackOpens_AdminEmpty(t *testing.T) {
	setupTrackingEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/opens") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"opens": []map[string]any{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   srv.URL,
		TrackingKey: "trackkey",
		AdminKey:    "adminkey",
	}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens", "--to", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "opens\t0") {
		t.Fatalf("unexpected empty admin output: %q", out)
	}
}

func TestGmailTrackOpens_NotConfigured(t *testing.T) {
	setupTrackingEnv(t)

	cfg := &tracking.Config{Enabled: false}
	if err := tracking.SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	if err := Execute([]string{"--account", "a@b.com", "gmail", "track", "opens"}); err == nil {
		t.Fatalf("expected error for unconfigured tracking")
	}
}

func TestParseTrackingSince_FlexibleFormats(t *testing.T) {
	t.Parallel()

	if parsed, err := parseTrackingSince("2026-02-13T10:20"); err != nil || parsed == "" {
		t.Fatalf("unexpected local datetime parse: %q err=%v", parsed, err)
	}

	parsedNano, err := parseTrackingSince("2026-02-13T10:20:30.123456789Z")
	if err != nil {
		t.Fatalf("unexpected RFC3339Nano parse error: %v", err)
	}
	if !strings.Contains(parsedNano, ".123456789Z") {
		t.Fatalf("expected nano precision output, got %q", parsedNano)
	}
}
