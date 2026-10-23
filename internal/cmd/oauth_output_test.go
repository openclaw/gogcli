package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	gogapi "github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/secrets"
)

const oauthOutputSecret = "synthetic-oauth-output-secret"

type oauthOutputTokenSourceFunc func() (*oauth2.Token, error)

func (f oauthOutputTokenSourceFunc) Token() (*oauth2.Token, error) { return f() }

func executeOAuthOutputFixture(t *testing.T, args []string, body string) executeTestResult {
	t.Helper()
	setTestConfigHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	client := &http.Client{Transport: gogapi.NewRetryTransport(&oauth2.Transport{
		Source: cfg.TokenSource(t.Context(), &oauth2.Token{RefreshToken: "synthetic-refresh-token"}),
	})}
	svc, err := gmail.NewService(t.Context(), option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return executeWithGmailTestService(t, args, svc)
}

func TestExecute_OAuthOutput(t *testing.T) {
	for _, fixture := range []struct {
		name, body string
		code       int
	}{
		{"revoked", `{"error":"invalid_grant","error_description":"` + oauthOutputSecret + `","error_uri":"https://example.invalid/` + oauthOutputSecret + `"}`, exitCodeAuthRequired},
		{"client", `{"error":"invalid_client","error_description":"` + oauthOutputSecret + `"}`, 1},
		{"unknown-code", `{"error":"` + oauthOutputSecret + `"}`, 1},
		{"raw-body", oauthOutputSecret, 1},
	} {
		for _, mode := range []string{"text", "plain", "json"} {
			t.Run(fixture.name+"/"+mode, func(t *testing.T) {
				args := []string{"--readonly", "--no-input", "--account", "reader@example.invalid", "gmail", "labels", "list"}
				if mode != "text" {
					args = append([]string{"--" + mode}, args...)
				}
				result := executeOAuthOutputFixture(t, args, fixture.body)
				if got := ExitCode(result.err); got != fixture.code {
					t.Fatalf("exit code = %d, want %d", got, fixture.code)
				}
				var cause *oauth2.RetrieveError
				if !errors.As(result.err, &cause) || string(cause.Body) != fixture.body {
					t.Fatal("original OAuth cause/body must remain available to callers")
				}
				output := result.stdout + result.stderr
				if strings.Contains(output, oauthOutputSecret) || !strings.Contains(output, "OAuth token") {
					t.Fatalf("unsafe or missing OAuth diagnostic: %s", output)
				}
				if mode == "json" {
					assertOnlyJSONError(t, result.stdout, fixture.code)
				}
			})
		}
	}
}

func TestOAuthOutputMCPChild(t *testing.T) {
	if os.Getenv("GOG_TEST_OAUTH_OUTPUT_CHILD") != "1" {
		return
	}
	result := executeOAuthOutputFixture(t, []string{"--json", "--readonly", "--no-input", "--account", "reader@example.invalid", "gmail", "labels", "list"}, `{"error":"invalid_grant","error_description":"`+oauthOutputSecret+`"}`)
	_, _ = fmt.Fprint(os.Stdout, result.stdout)
	_, _ = fmt.Fprint(os.Stderr, result.stderr)
	os.Exit(ExitCode(result.err))
}

func TestMCP_OAuthOutput(t *testing.T) {
	t.Setenv("GOG_TEST_OAUTH_OUTPUT_CHILD", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result := mcpRunGogTool(t.Context(), mcpRunOptions{
		self: self, tool: findMCPTool(t, "gmail_list_labels"),
		baseArgs: []string{"-test.run=^TestOAuthOutputMCPChild$"}, timeout: 10 * time.Second, maxOutputBytes: 8192,
	})
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var commandResult mcpCommandResult
	if err := json.Unmarshal(encoded, &commandResult); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || commandResult.ExitCode != exitCodeAuthRequired || strings.Contains(string(encoded), oauthOutputSecret) || !strings.Contains(string(encoded), "invalid_grant") {
		t.Fatalf("unexpected MCP result: %s", encoded)
	}
}

func TestAuthHealth_OAuthOutput(t *testing.T) {
	for _, command := range []string{"list", "doctor"} {
		for _, mode := range []string{"text", "json"} {
			t.Run(command+"/"+mode, func(t *testing.T) {
				setTestConfigHome(t)
				store := newMemSecretsStore()
				if err := store.SetToken(config.DefaultClientName, "reader@example.invalid", secrets.Token{RefreshToken: "synthetic-refresh-token"}); err != nil {
					t.Fatal(err)
				}
				runtime := &app.Runtime{Auth: app.AuthOperations{
					OpenSecretsStore: func() (secrets.Store, error) { return store, nil },
					CheckRefreshToken: func(context.Context, string, string, []string, time.Duration) error {
						return &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: oauthOutputSecret}
					},
				}}
				args := []string{"auth", command, "--check"}
				if mode == "json" {
					args = append([]string{"--json"}, args...)
				}
				result := executeWithTestRuntime(t, args, runtime)
				output := result.stdout + result.stderr
				if result.err != nil || strings.Contains(output, oauthOutputSecret) || !strings.Contains(output, "invalid_grant") {
					t.Fatalf("unexpected auth health result: %v; %s", result.err, output)
				}
			})
		}
	}
}

func TestExecute_OAuthLogOutput(t *testing.T) {
	setTestConfigHome(t)
	cause := &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: oauthOutputSecret}
	result := executeWithTestRuntime(t, []string{"--verbose", "--no-input", "--account", "reader@example.invalid", "gmail", "labels", "list"}, &app.Runtime{Services: app.Services{
		Gmail: func(context.Context, string) (*gmail.Service, error) {
			// Auth retry paths log errors before returning them to the formatter.
			slog.Debug("synthetic auth refresh failure", "err", fmt.Errorf("refresh: %w", cause))
			return nil, cause
		},
	}})
	if ExitCode(result.err) != exitCodeAuthRequired || !strings.Contains(result.stderr, "synthetic auth refresh failure") || strings.Contains(result.stdout+result.stderr, oauthOutputSecret) {
		t.Fatalf("unexpected OAuth log output: %s%s", result.stdout, result.stderr)
	}
}

func TestExecute_OAuthPerItemOutput(t *testing.T) {
	for _, command := range []string{"archive", "labels"} {
		for _, mode := range []string{"text", "plain", "json"} {
			t.Run(command+"/"+mode, func(t *testing.T) {
				setTestConfigHome(t)
				var tokenCalls, labelCalls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.Method == http.MethodPost && r.URL.Path == "/token":
						tokenCalls.Add(1)
						w.WriteHeader(http.StatusBadRequest)
						_, _ = fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, oauthOutputSecret)
					case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/me/labels"):
						labelCalls.Add(1)
						_, _ = fmt.Fprint(w, `{"labels":[{"id":"INBOX","name":"INBOX"}]}`)
					default:
						t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				defer srv.Close()
				cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
				refresh := cfg.TokenSource(t.Context(), &oauth2.Token{RefreshToken: "synthetic-refresh-token"})
				// Labels modification resolves label names before iterating threads.
				// Permit that first request, then drive real refresh failures for
				// each item through oauth2.Transport and RetryTransport.
				first := true
				source := oauthOutputTokenSourceFunc(func() (*oauth2.Token, error) {
					if first && command == "labels" {
						first = false
						return &oauth2.Token{AccessToken: "synthetic-access-token"}, nil
					}
					return refresh.Token()
				})
				client := &http.Client{Transport: gogapi.NewRetryTransport(&oauth2.Transport{Source: source})}
				svc, err := gmail.NewService(t.Context(), option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
				if err != nil {
					t.Fatal(err)
				}
				args := []string{"--no-input", "--account", "reader@example.invalid", "gmail"}
				wantCode := 0
				if command == "archive" {
					args = append(args, "archive", "--thread", "t1", "t2")
					wantCode = 1 // Preserve the existing partial-failure summary.
				} else {
					args = append(args, "labels", "modify", "t1", "t2", "--remove", "INBOX")
				}
				if mode != "text" {
					args = append([]string{"--" + mode}, args...)
				}
				result := executeWithGmailTestService(t, args, svc)
				if got := ExitCode(result.err); got != wantCode {
					t.Fatalf("exit code = %d, want %d; error: %v", got, wantCode, result.err)
				}
				output := result.stdout + result.stderr
				if strings.Contains(output, oauthOutputSecret) || strings.Count(output, "invalid_grant") != 2 {
					t.Fatalf("unsafe or missing per-item diagnostics: %s", output)
				}
				if tokenCalls.Load() != 2 || (command == "labels" && labelCalls.Load() != 1) {
					t.Fatalf("unexpected token/label calls: %d/%d", tokenCalls.Load(), labelCalls.Load())
				}
				if mode == "json" {
					var decoded struct {
						Results []struct {
							ThreadID string `json:"threadId"`
							Success  bool   `json:"success"`
							Error    string `json:"error"`
						} `json:"results"`
					}
					if err := json.Unmarshal([]byte(result.stdout), &decoded); err != nil {
						t.Fatal(err)
					}
					if len(decoded.Results) != 2 {
						t.Fatalf("result count = %d, want 2", len(decoded.Results))
					}
					for i, row := range decoded.Results {
						if row.ThreadID != fmt.Sprintf("t%d", i+1) || row.Success || !strings.Contains(row.Error, "invalid_grant") {
							t.Fatalf("unexpected per-item result: %+v", row)
						}
					}
				}
			})
		}
	}
}

func TestBackup_OAuthPerItemOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/token" {
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, oauthOutputSecret)
	}))
	defer srv.Close()
	cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	client := &http.Client{Transport: gogapi.NewRetryTransport(&oauth2.Transport{
		Source: cfg.TokenSource(t.Context(), &oauth2.Token{RefreshToken: "synthetic-refresh-token"}),
	})}
	svc, err := drive.NewService(t.Context(), option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	rows := fetchBackupDriveFileCollaboration(t.Context(), svc, "synthetic-file")
	if len(rows.Permissions) != 1 || len(rows.Comments) != 1 || len(rows.Revisions) != 1 {
		t.Fatalf("missing backup error rows: %+v", rows)
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), oauthOutputSecret) || strings.Count(string(encoded), "invalid_grant") != 3 {
		t.Fatalf("unsafe or missing backup diagnostics: %s", encoded)
	}
	_, callErr := svc.Files.Get("synthetic-file").Context(t.Context()).Do()
	if callErr == nil {
		t.Fatal("expected refresh failure")
	}
	snapshot, err := buildBackupServiceErrorSnapshot("drive", "synthetic-account", callErr)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Shards) != 1 || snapshot.Shards[0].Rows != 1 {
		t.Fatal("missing service failure snapshot")
	}
	text := string(snapshot.Shards[0].Plaintext)
	if strings.Contains(text, oauthOutputSecret) || !strings.Contains(text, "invalid_grant") {
		t.Fatalf("unsafe or missing service failure diagnostic: %s", text)
	}
	var cause *oauth2.RetrieveError
	if !errors.As(callErr, &cause) || cause.ErrorDescription != oauthOutputSecret {
		t.Fatal("snapshot formatting changed the original cause")
	}
}
