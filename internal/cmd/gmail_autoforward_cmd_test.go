package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func TestGmailAutoForwardGetCmd_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/autoForwarding") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      true,
				"emailAddress": "a@example.com",
				"disposition":  "leaveInInbox",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	var out bytes.Buffer
	ctx := withGmailTestService(
		newCmdRuntimeOutputContext(t, &out, io.Discard),
		newGmailServiceFromServer(t, srv),
	)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &GmailAutoForwardGetCmd{}
	if err := runKong(t, cmd, []string{}, ctx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(out.String(), "enabled\ttrue") || !strings.Contains(out.String(), "email_address\ta@example.com") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestGmailAutoForwardUpdateCmd_JSONAndValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/settings/autoForwarding") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      false,
				"emailAddress": "old@example.com",
				"disposition":  "leaveInInbox",
			})
			return
		}
		if strings.Contains(r.URL.Path, "/settings/autoForwarding") && r.Method == http.MethodPut {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":      true,
				"emailAddress": "new@example.com",
				"disposition":  "archive",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	failServiceCtx := withGmailTestServiceFactory(
		newCmdRuntimeOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*gmail.Service, error) {
			t.Fatal("gmail service should not be created during validation")
			return nil, context.Canceled
		},
	)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &GmailAutoForwardUpdateCmd{}
	err := runKong(t, cmd, []string{"--disposition", "nope"}, failServiceCtx, flags)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}

	cmdConflict := &GmailAutoForwardUpdateCmd{}
	err = runKong(t, cmdConflict, []string{"--enable", "--disable"}, failServiceCtx, flags)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}

	cmdEmail := &GmailAutoForwardUpdateCmd{}
	err = runKong(t, cmdEmail, []string{"--enable", "--email", "nope"}, failServiceCtx, flags)
	if err == nil || !strings.Contains(err.Error(), "invalid --email") {
		t.Fatalf("expected email validation error, got %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}

	var jsonOut bytes.Buffer
	jsonCtx := withGmailTestService(
		newCmdRuntimeJSONOutputContext(t, &jsonOut, io.Discard),
		newGmailServiceFromServer(t, srv),
	)
	cmd2 := &GmailAutoForwardUpdateCmd{}
	if err := runKong(t, cmd2, []string{"--enable", "--email", "new@example.com", "--disposition", "archive"}, jsonCtx, flags); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var parsed struct {
		AutoForwarding struct {
			Enabled      bool   `json:"enabled"`
			EmailAddress string `json:"emailAddress"`
			Disposition  string `json:"disposition"`
		} `json:"autoForwarding"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if !parsed.AutoForwarding.Enabled || parsed.AutoForwarding.EmailAddress != "new@example.com" {
		t.Fatalf("unexpected json: %#v", parsed.AutoForwarding)
	}
}

func TestGmailAutoForwardUpdate_InvalidEmailFailsBeforeDryRun(t *testing.T) {
	result := executeWithTestRuntime(t,
		[]string{"--account", "a@b.com", "--dry-run", "gmail", "autoforward", "update", "--enable", "--email", "nope"},
		&app.Runtime{Services: app.Services{Gmail: func(context.Context, string) (*gmail.Service, error) {
			t.Fatal("expected validation to fail before creating gmail service")
			return nil, errors.New("unexpected gmail service call")
		}}},
	)
	var exitErr *ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code != 2 || !strings.Contains(result.err.Error(), "invalid --email") {
		t.Fatalf("unexpected err: %v", result.err)
	}
}
