package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// idsOnlyGmailServer serves list pages of two items each and records every
// other request, which --ids-only must never make.
func idsOnlyGmailServer(t *testing.T, resource string) (*httptest.Server, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var unexpected []string
	listPath := "/users/me/" + resource
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, listPath) {
			mu.Lock()
			unexpected = append(unexpected, r.URL.Path)
			mu.Unlock()
			http.NotFound(w, r)
			return
		}
		page := map[string]any{
			resource:        []map[string]any{{"id": "a1", "threadId": "t1"}, {"id": "a2", "threadId": "t2"}},
			"nextPageToken": "p2",
		}
		if r.URL.Query().Get("pageToken") == "p2" {
			page = map[string]any{resource: []map[string]any{{"id": "a3", "threadId": "t3"}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	t.Cleanup(srv.Close)

	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), unexpected...)
	}
}

func TestExecute_GmailMessagesSearch_IDsOnlySkipsMessageFetches(t *testing.T) {
	srv, unexpected := idsOnlyGmailServer(t, "messages")

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "list:example.com", "--all", "--ids-only"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if got := unexpected(); len(got) > 0 {
		t.Fatalf("--ids-only made non-list requests: %v", got)
	}

	var payload struct {
		Messages []messageItem `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode stdout %q: %v", result.stdout, err)
	}
	want := []messageItem{{ID: "a1", ThreadID: "t1"}, {ID: "a2", ThreadID: "t2"}, {ID: "a3", ThreadID: "t3"}}
	if len(payload.Messages) != len(want) {
		t.Fatalf("messages = %+v, want %+v", payload.Messages, want)
	}
	for i := range want {
		if payload.Messages[i].ID != want[i].ID || payload.Messages[i].ThreadID != want[i].ThreadID || payload.Messages[i].From != "" {
			t.Fatalf("messages = %+v, want %+v", payload.Messages, want)
		}
	}
}

func TestExecute_GmailMessagesSearch_IDsOnlyPlainPrintsIDColumns(t *testing.T) {
	srv, _ := idsOnlyGmailServer(t, "messages")

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "messages", "search", "list:example.com", "--ids-only"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if want := "ID\tTHREAD\na1\tt1\na2\tt2\n"; result.stdout != want {
		t.Fatalf("stdout = %q, want %q", result.stdout, want)
	}
}

func TestExecute_GmailMessagesSearch_IDsOnlyRejectsContentFlags(t *testing.T) {
	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "messages", "search", "list:example.com", "--ids-only", "--include-body"},
		nil,
	)
	if result.err == nil || !strings.Contains(result.err.Error(), "--ids-only cannot be used with") {
		t.Fatalf("expected --ids-only usage error, got %v", result.err)
	}
}

func TestExecute_GmailSearch_IDsOnlyRejectsOldest(t *testing.T) {
	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "search", "list:example.com", "--ids-only", "--oldest"},
		nil,
	)
	if result.err == nil || !strings.Contains(result.err.Error(), "--ids-only cannot be used with --oldest") {
		t.Fatalf("expected --ids-only usage error, got %v", result.err)
	}
}

func TestExecute_GmailSearch_IDsOnlySkipsThreadFetches(t *testing.T) {
	srv, unexpected := idsOnlyGmailServer(t, "threads")

	result := executeWithGmailTestService(
		t,
		[]string{"--json", "--account", "a@b.com", "gmail", "search", "list:example.com", "--all", "--ids-only"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if got := unexpected(); len(got) > 0 {
		t.Fatalf("--ids-only made non-list requests: %v", got)
	}

	var payload struct {
		Threads []threadItem `json:"threads"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode stdout %q: %v", result.stdout, err)
	}
	if len(payload.Threads) != 3 || payload.Threads[0].ID != "a1" || payload.Threads[2].ID != "a3" {
		t.Fatalf("threads = %+v, want a1, a2, a3", payload.Threads)
	}
}
