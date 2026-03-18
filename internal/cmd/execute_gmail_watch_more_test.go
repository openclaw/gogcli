package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestExecute_GmailWatch_MoreCommands(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	setWatchTestConfigHome(t)
	t.Setenv("GOG_ACCOUNT", "a@b.com")

	var stopCalled bool
	var watchCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/gmail/v1/users/me/labels") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX"},
					{"id": "Label_1", "name": "Custom"},
				},
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/watch") && r.Method == http.MethodPost:
			watchCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"historyId":  "123",
				"expiration": "1730000000000",
			})
			return
		case strings.Contains(path, "/gmail/v1/users/me/stop") && r.Method == http.MethodPost:
			stopCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc, newServiceErr := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if newServiceErr != nil {
		t.Fatalf("NewService: %v", newServiceErr)
	}
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if execErr := Execute([]string{"--json", "gmail", "watch", "start", "--topic", "projects/p/topics/t", "--label", "INBOX"}); execErr != nil {
				t.Fatalf("start: %v", execErr)
			}
		})
		if watchCalls != 1 {
			t.Fatalf("expected watch call, got %d", watchCalls)
		}

		_ = captureStdout(t, func() {
			if execErr := Execute([]string{"--json", "gmail", "watch", "status"}); execErr != nil {
				t.Fatalf("status: %v", execErr)
			}
		})
		_ = captureStdout(t, func() {
			if execErr := Execute([]string{"--json", "gmail", "watch", "renew", "--ttl", "10"}); execErr != nil {
				t.Fatalf("renew: %v", execErr)
			}
		})
		if watchCalls != 2 {
			t.Fatalf("expected second watch call, got %d", watchCalls)
		}

		// Serve validations (should error before ListenAndServe).
		if execErr := Execute([]string{"gmail", "watch", "serve", "--path", "nope"}); execErr == nil || !strings.Contains(execErr.Error(), "--path must start") {
			t.Fatalf("expected path validation error, got: %v", execErr)
		}
		if execErr := Execute([]string{"gmail", "watch", "serve", "--port", "0"}); execErr == nil || !strings.Contains(execErr.Error(), "--port must be > 0") {
			t.Fatalf("expected port validation error, got: %v", execErr)
		}
		if execErr := Execute([]string{"gmail", "watch", "serve", "--bind", "0.0.0.0", "--path", "/x"}); execErr == nil || !strings.Contains(execErr.Error(), "--verify-oidc or --token required") {
			t.Fatalf("expected bind validation error, got: %v", execErr)
		}

		_ = captureStdout(t, func() {
			if execErr := Execute([]string{"--json", "--force", "gmail", "watch", "stop"}); execErr != nil {
				t.Fatalf("stop: %v", execErr)
			}
		})
	})

	if !stopCalled {
		t.Fatalf("expected stop called")
	}
	// State file removed.
	p, err := gmailWatchStatePath("a@b.com")
	if err != nil {
		t.Fatalf("state path: %v", err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected watch state removed: %s", p)
	}

	// Ensure dir exists but file doesn't.
	if !strings.Contains(p, filepath.Join("gogcli", "state", "gmail-watch")) {
		t.Fatalf("unexpected state path: %s", p)
	}
}
