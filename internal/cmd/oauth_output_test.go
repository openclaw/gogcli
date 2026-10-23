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
	"testing"
	"time"

	"golang.org/x/oauth2"
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
