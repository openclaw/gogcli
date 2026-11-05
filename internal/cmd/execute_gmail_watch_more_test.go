package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
)

func TestExecute_GmailWatchServeEnvTokens(t *testing.T) {
	for _, command := range [][]string{{"gmail", "watch", "serve"}, {"gmail", "settings", "watch", "serve"}} {
		for _, save := range []bool{false, true} {
			name := strings.Join(command, " ")
			if save {
				name += " save-hook"
			}
			t.Run(name, func(t *testing.T) {
				setWatchTestConfigHome(t)
				t.Setenv("GOG_ACCOUNT", "a@b.com")
				t.Setenv("TEST_WATCH_PUSH", "synthetic-push-secret")
				t.Setenv("TEST_WATCH_HOOK", "synthetic-hook-secret")
				hookCalls := 0
				hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hookCalls++
					if r.Header.Get("Authorization") != "Bearer synthetic-hook-secret" {
						t.Error("hook did not receive the environment token")
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				defer hook.Close()
				store := newGmailWatchTestStore(t, "a@b.com")
				if err := store.SetHook(&gmailWatchHook{
					URL: hook.URL, Token: "stored-token", IncludeBody: true, MaxBytes: 123,
				}, time.Now()); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(store.Path())
				if err != nil {
					t.Fatal(err)
				}
				origListen := listenAndServe
				t.Cleanup(func() { listenAndServe = origListen })
				listened := false
				listenAndServe = func(srv *http.Server) error {
					listened = true
					server, ok := srv.Handler.(*gmailWatchServer)
					if !ok {
						t.Fatal("unexpected watch handler")
					}
					if server.cfg.SharedToken != "synthetic-push-secret" || server.cfg.HookToken != "synthetic-hook-secret" {
						t.Fatal("environment tokens did not reach the server")
					}
					if server.cfg.HookURL != hook.URL || !server.cfg.IncludeBody || server.cfg.MaxBodyBytes != 123 {
						t.Fatal("stored hook defaults were lost")
					}
					for _, token := range []string{"", "wrong-token", "synthetic-push-secret"} {
						request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/gmail-pubsub", nil)
						request.Header.Set("x-gog-token", token)
						if server.authorize(request) != (token == "synthetic-push-secret") {
							t.Fatal("push authentication did not use the environment token")
						}
					}
					return server.deliverHook(context.Background(), &gmailHookPayload{}).Err
				}
				args := append([]string{}, command...)
				args = append(args, "--bind", "0.0.0.0", "--token-env", "TEST_WATCH_PUSH", "--hook-token-env", "TEST_WATCH_HOOK")
				if save {
					args = append(args, "--save-hook")
				}
				result := executeWithGmailTestService(t, args, &gmail.Service{})
				if result.err != nil {
					t.Fatal(result.err)
				}
				if !listened || hookCalls != 1 {
					t.Fatal("watch server and hook delivery were not exercised")
				}
				for _, secret := range []string{"synthetic-push-secret", "synthetic-hook-secret"} {
					if strings.Contains(result.stdout+result.stderr, secret) || strings.Contains(strings.Join(args, " "), secret) {
						t.Fatal("secret appeared in output or arguments")
					}
				}
				after, err := os.ReadFile(store.Path())
				if err != nil {
					t.Fatal(err)
				}
				if !save && !bytes.Equal(before, after) {
					t.Fatal("hook settings changed without --save-hook")
				}
				if save {
					state := loadGmailWatchTestStore(t, "a@b.com").Get()
					if state.Hook == nil || state.Hook.Token != "synthetic-hook-secret" {
						t.Fatal("resolved hook token was not saved")
					}
					info, statErr := os.Stat(store.Path())
					if statErr != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
						t.Fatal("saved hook state is not mode 0600")
					}
				}
				if strings.Contains(string(after), "synthetic-push-secret") || strings.Contains(string(after), "TEST_WATCH_") {
					t.Fatal("watch state contains a push token or environment source name")
				}
			})
		}
	}
}

func TestExecute_GmailWatch_MoreCommands(t *testing.T) {
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

	svc := newGmailServiceFromServer(t, srv)
	run := func(args ...string) executeTestResult {
		return executeWithGmailTestService(t, args, svc)
	}

	if result := run("--json", "gmail", "watch", "start", "--topic", "projects/p/topics/t", "--label", "INBOX"); result.err != nil {
		t.Fatalf("start: %v", result.err)
	}
	if watchCalls != 1 {
		t.Fatalf("expected watch call, got %d", watchCalls)
	}

	if result := run("--json", "gmail", "watch", "status"); result.err != nil {
		t.Fatalf("status: %v", result.err)
	}
	if result := run("--json", "gmail", "watch", "renew", "--ttl", "10"); result.err != nil {
		t.Fatalf("renew: %v", result.err)
	}
	if watchCalls != 2 {
		t.Fatalf("expected second watch call, got %d", watchCalls)
	}

	// Serve validations should fail before ListenAndServe.
	if result := run("gmail", "watch", "serve", "--path", "nope"); result.err == nil || !strings.Contains(result.err.Error(), "--path must start") {
		t.Fatalf("expected path validation error, got: %v", result.err)
	}
	if result := run("gmail", "watch", "serve", "--port", "0"); result.err == nil || !strings.Contains(result.err.Error(), "--port must be > 0") {
		t.Fatalf("expected port validation error, got: %v", result.err)
	}
	if result := run("gmail", "watch", "serve", "--bind", "0.0.0.0", "--path", "/x"); result.err == nil || !strings.Contains(result.err.Error(), "--verify-oidc or --token required") {
		t.Fatalf("expected bind validation error, got: %v", result.err)
	}

	if result := run("--json", "--force", "gmail", "watch", "stop"); result.err != nil {
		t.Fatalf("stop: %v", result.err)
	}

	if !stopCalled {
		t.Fatalf("expected stop called")
	}
	// State file removed.
	p, err := gmailWatchStatePath(gmailWatchTestLayout(t), "a@b.com")
	if err != nil {
		t.Fatalf("state path: %v", err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected watch state removed: %s", p)
	}

	// Ensure the account-scoped file is under the watch state directory shape.
	newWatchDir := filepath.Join("gogcli", "gmail-watch")
	legacyWatchDir := filepath.Join("gogcli", "state", "gmail-watch")
	if !strings.Contains(p, newWatchDir) && !strings.Contains(p, legacyWatchDir) {
		t.Fatalf("unexpected state path: %s", p)
	}
}
