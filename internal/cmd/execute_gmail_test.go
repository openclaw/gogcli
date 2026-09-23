package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	gogapi "github.com/openclaw/gogcli/internal/googleapi"
)

func TestExecute_GmailExpiredToken(t *testing.T) {
	setTestConfigHome(t)
	tokenCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/token" {
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		tokenCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	}))
	defer srv.Close()

	ctx := context.Background()
	cfg := &oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	client := &http.Client{Transport: gogapi.NewRetryTransport(&oauth2.Transport{
		Source: cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: "revoked-test-token"}),
	})}
	svc, err := gmail.NewService(ctx, option.WithHTTPClient(client), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	result := executeWithGmailTestService(t, []string{
		"--readonly", "--no-input", "--json", "--account", "test@example.com", "gmail", "labels", "list",
	}, svc)
	if got := ExitCode(result.err); got != exitCodeAuthRequired {
		t.Fatalf("exit code = %d, want %d; error: %v", got, exitCodeAuthRequired, result.err)
	}
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(result.err, &retrieveErr) || retrieveErr.ErrorCode != "invalid_grant" {
		t.Fatalf("missing OAuth cause: %v", result.err)
	}
	if !strings.Contains(result.err.Error(), "run 'gog auth add' to re-authorize") {
		t.Fatalf("missing recovery advice: %v", result.err)
	}
	if result.stdout != "" || tokenCalls != 1 {
		t.Fatalf("stdout = %q; token requests = %d, want empty output and one attempt", result.stdout, tokenCalls)
	}
}

func TestExecute_GmailSearch_JSON(t *testing.T) {
	srv := httptest.NewServer(gmailSearchTestHandler())
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "search", "newer_than:7d", "--max", "1", "--timezone", "UTC"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	var parsed struct {
		Threads []struct {
			ID           string   `json:"id"`
			Date         string   `json:"date"`
			From         string   `json:"from"`
			Subject      string   `json:"subject"`
			Labels       []string `json:"labels"`
			MessageCount int      `json:"messageCount"`
		} `json:"threads"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if parsed.NextPageToken != "npt" || len(parsed.Threads) != 1 {
		t.Fatalf("unexpected: %#v", parsed)
	}
	if parsed.Threads[0].ID != "t1" || parsed.Threads[0].Subject != "Hello" {
		t.Fatalf("unexpected thread: %#v", parsed.Threads[0])
	}
	if parsed.Threads[0].MessageCount != 1 {
		t.Fatalf("unexpected messageCount: %d", parsed.Threads[0].MessageCount)
	}
	if parsed.Threads[0].Date != "2006-01-02 22:04" {
		t.Fatalf("unexpected date: %q", parsed.Threads[0].Date)
	}
	if len(parsed.Threads[0].Labels) != 1 || parsed.Threads[0].Labels[0] != "INBOX" {
		t.Fatalf("unexpected labels: %#v", parsed.Threads[0].Labels)
	}
}

func TestExecute_GmailURL_JSON(t *testing.T) {
	result := executeWithTestRuntime(t, []string{"--json", "--account", "a@b.com", "gmail", "url", "t1"}, nil)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	var parsed struct {
		URLs []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"urls"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, result.stdout)
	}
	if len(parsed.URLs) != 1 || parsed.URLs[0].ID != "t1" || !strings.Contains(parsed.URLs[0].URL, "#all/t1") {
		t.Fatalf("unexpected urls: %#v", parsed.URLs)
	}
}
