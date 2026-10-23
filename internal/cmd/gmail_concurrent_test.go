package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestGmailSearchRejectsPartialThreadDetails(t *testing.T) {
	var failedCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gmail/v1/users/me/threads":
			_, _ = io.WriteString(w, `{"threads":[{"id":"ok"},{"id":"fail"}]}`)
		case "/gmail/v1/users/me/labels":
			_, _ = io.WriteString(w, `{"labels":[]}`)
		case "/gmail/v1/users/me/threads/ok":
			_, _ = io.WriteString(w, `{"id":"ok","messages":[]}`)
		case "/gmail/v1/users/me/threads/fail":
			if failedCalls.Add(1) == 1 {
				http.Error(w, "thread detail unavailable", http.StatusInternalServerError)
				return
			}
			_, _ = io.WriteString(w, `{"id":"fail","messages":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	svc := newGoogleTestServiceWithEndpoint(t, server.Client(), server.URL+"/", gmail.NewService)
	var output bytes.Buffer
	ctx := withGmailTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc)
	err := runKong(t, &GmailSearchCmd{}, []string{"in:inbox", "--fail-empty"}, ctx, &RootFlags{Account: "me@example.com"})
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusInternalServerError {
		t.Fatalf("error = %v, want the thread detail HTTP 500", err)
	}
	if output.Len() != 0 || failedCalls.Load() != 1 {
		t.Fatalf("partial output = %q, failed thread calls = %d", output.String(), failedCalls.Load())
	}
}

func TestFetchThreadDetails_Empty(t *testing.T) {
	items, err := fetchThreadDetails(context.Background(), nil, nil, nil, false, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty items, got %d", len(items))
	}
}

func TestFetchThreadDetails_Concurrent(t *testing.T) {
	// Create a mock server that returns thread data
	mux := http.NewServeMux()

	// Track calls to verify concurrency (atomic for thread safety)
	var callCount atomic.Int32

	mux.HandleFunc("/gmail/v1/users/me/threads/", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		threadID := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/threads/")

		response := fmt.Sprintf(`{
			"id": "%s",
			"messages": [{
				"id": "msg_%s",
				"labelIds": ["INBOX"],
				"payload": {
					"headers": [
						{"name": "From", "value": "test@example.com"},
						{"name": "Subject", "value": "Test Subject %s"},
						{"name": "Date", "value": "Mon, 01 Jan 2024 10:00:00 +0000"}
					]
				}
			}]
		}`, threadID, threadID, threadID)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Create Gmail service pointing to mock server
	svc, err := gmail.NewService(context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(http.DefaultClient),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	threads := []*gmail.Thread{
		{Id: "thread1"},
		{Id: "thread2"},
		{Id: "thread3"},
	}

	idToName := map[string]string{
		"INBOX": "Inbox",
	}

	items, err := fetchThreadDetails(context.Background(), svc, threads, idToName, false, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Verify all threads were fetched
	if callCount.Load() != 3 {
		t.Errorf("expected 3 API calls, got %d", callCount.Load())
	}

	// Verify items have correct data (order should be preserved)
	for i, item := range items {
		expectedID := fmt.Sprintf("thread%d", i+1)
		if item.ID != expectedID {
			t.Errorf("item %d: expected ID %s, got %s", i, expectedID, item.ID)
		}
		if item.From != "test@example.com" {
			t.Errorf("item %d: expected From test@example.com, got %s", i, item.From)
		}
		if !strings.Contains(item.Subject, "Test Subject") {
			t.Errorf("item %d: expected Subject to contain 'Test Subject', got %s", i, item.Subject)
		}
		if len(item.Labels) != 1 || item.Labels[0] != "Inbox" {
			t.Errorf("item %d: expected Labels [Inbox], got %v", i, item.Labels)
		}
	}
}

func TestFetchThreadDetails_DateSelection(t *testing.T) {
	mux := http.NewServeMux()
	older := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC)
	mux.HandleFunc("/gmail/v1/users/me/threads/", func(w http.ResponseWriter, r *http.Request) {
		response := fmt.Sprintf(`{
			"id": "thread1",
			"messages": [{
				"id": "msg_new",
				"internalDate": "%d",
				"payload": {
					"headers": [
						{"name": "From", "value": "new@example.com"},
						{"name": "Subject", "value": "New Subject"},
						{"name": "Date", "value": "%s"}
					]
				}
			}, {
				"id": "msg_old",
				"internalDate": "%d",
				"payload": {
					"headers": [
						{"name": "From", "value": "old@example.com"},
						{"name": "Subject", "value": "Old Subject"},
						{"name": "Date", "value": "%s"}
					]
				}
			}]
		}`, newer.UnixMilli(), newer.Format(time.RFC1123Z), older.UnixMilli(), older.Format(time.RFC1123Z))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(http.DefaultClient),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	threads := []*gmail.Thread{{Id: "thread1"}}

	itemsNewest, err := fetchThreadDetails(context.Background(), svc, threads, nil, false, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(itemsNewest) != 1 {
		t.Fatalf("expected 1 item, got %d", len(itemsNewest))
	}
	expectedNewest := formatGmailDateInLocation(newer.Format(time.RFC1123Z), time.UTC)
	if itemsNewest[0].Date != expectedNewest {
		t.Errorf("expected newest date %s, got %s", expectedNewest, itemsNewest[0].Date)
	}

	itemsOldest, err := fetchThreadDetails(context.Background(), svc, threads, nil, true, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(itemsOldest) != 1 {
		t.Fatalf("expected 1 item, got %d", len(itemsOldest))
	}
	expectedOldest := formatGmailDateInLocation(older.Format(time.RFC1123Z), time.UTC)
	if itemsOldest[0].Date != expectedOldest {
		t.Errorf("expected oldest date %s, got %s", expectedOldest, itemsOldest[0].Date)
	}
}

func TestFetchThreadDetails_SkipsEmptyIDs(t *testing.T) {
	mux := http.NewServeMux()
	var callCount atomic.Int32

	mux.HandleFunc("/gmail/v1/users/me/threads/", func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		threadID := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/threads/")
		response := fmt.Sprintf(`{"id": "%s", "messages": []}`, threadID)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc, _ := gmail.NewService(context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(http.DefaultClient),
	)

	threads := []*gmail.Thread{
		{Id: ""},        // Should be skipped
		{Id: "thread1"}, // Should be processed
		{Id: ""},        // Should be skipped
	}

	items, err := fetchThreadDetails(context.Background(), svc, threads, nil, false, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 API call should be made (for thread1)
	if callCount.Load() != 1 {
		t.Errorf("expected 1 API call (skipping empty IDs), got %d", callCount.Load())
	}

	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestFetchThreadDetails_ConcurrentError(t *testing.T) {
	var calls atomic.Int32
	var failCalls atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/gmail/v1/users/me/threads/", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		threadID := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/threads/")
		if threadID == "thread-fail" && failCalls.Add(1) == 1 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		response := fmt.Sprintf(`{"id": "%s", "messages": []}`, threadID)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(server.Client()),
		option.WithEndpoint(server.URL+"/"),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	threads := []*gmail.Thread{
		{Id: "thread-ok"},
		{Id: ""},
		{Id: "thread-fail"},
		{Id: "thread-ok2"},
	}

	items, err := fetchThreadDetails(context.Background(), svc, threads, nil, false, time.UTC)
	if err == nil {
		t.Fatalf("expected first concurrent error, got %d items", len(items))
	}
	if items != nil {
		t.Fatalf("expected nil items on error, got %d", len(items))
	}
	if got := calls.Load(); got > 3 {
		t.Errorf("expected at most 3 concurrent calls and no sequential retry, got %d", got)
	}
}

func TestFetchThreadDetails_ContextCanceled(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/gmail/v1/users/me/threads/", func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		select {
		case <-r.Context().Done():
			return
		default:
			response := `{"id": "thread1", "messages": []}`
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(response))
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc, _ := gmail.NewService(context.Background(),
		option.WithEndpoint(server.URL),
		option.WithHTTPClient(http.DefaultClient),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	threads := []*gmail.Thread{{Id: "thread1"}}

	_, err := fetchThreadDetails(ctx, svc, threads, nil, false, time.UTC)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// newFirstRequestFailsGmailService serves Gmail detail requests. The first one
// fails with 403 rateLimitExceeded; every later one stays in flight until the
// client cancels it, so no slot is freed by anything but the fan-out itself.
// The handlers are released at cleanup, which lets a fan-out that never
// cancels finish after its test has failed.
func newFirstRequestFailsGmailService(t *testing.T) (*gmail.Service, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":403,"message":"User-rate limit exceeded.","errors":[{"domain":"usageLimits","reason":"rateLimitExceeded"}]}}`)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		id := path.Base(r.URL.Path)
		_, _ = fmt.Fprintf(w, `{"id":%q,"threadId":%q,"messages":[]}`, id, id)
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	return newGoogleTestServiceWithEndpoint(t, server.Client(), server.URL+"/", gmail.NewService), &calls
}

// awaitFanOut fails the test when fetch is still running after the first
// failure has had ample time to cancel it; it never decides a passing run.
func awaitFanOut(t *testing.T, fetch func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- fetch() }()

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the fan-out kept waiting on detail requests after its first failure")
		return nil
	}
}

func TestFetchThreadDetails_StopsAfterFirstError(t *testing.T) {
	svc, calls := newFirstRequestFailsGmailService(t)
	threads := make([]*gmail.Thread, 30)
	for i := range threads {
		threads[i] = &gmail.Thread{Id: fmt.Sprintf("thread-%02d", i)}
	}

	err := awaitFanOut(t, func() error {
		_, err := fetchThreadDetails(context.Background(), svc, threads, nil, false, time.UTC)
		return err
	})
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusForbidden {
		t.Fatalf("error = %v, want the thread detail HTTP 403", err)
	}
	// Only the failed request frees a slot before cancellation: at most the
	// 10 requests in flight plus one that took that slot ever reach the server.
	if got := calls.Load(); got > 11 {
		t.Fatalf("fetched %d of 30 threads after the search had failed, want at most 11", got)
	}
}

func TestFetchMessageDetails_StopsAfterFirstError(t *testing.T) {
	svc, calls := newFirstRequestFailsGmailService(t)
	messages := make([]*gmail.Message, 30)
	for i := range messages {
		messages[i] = &gmail.Message{Id: fmt.Sprintf("message-%02d", i)}
	}

	err := awaitFanOut(t, func() error {
		_, err := fetchMessageDetails(context.Background(), svc, messages, map[string]string{}, time.UTC, false, gmailMessageBodyFormatText, false, false, false, false)
		return err
	})
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) || apiErr.Code != http.StatusForbidden {
		t.Fatalf("error = %v, want the message detail HTTP 403", err)
	}
	// Only the failed request frees a slot before cancellation: at most the
	// 10 requests in flight plus one that took that slot ever reach the server.
	if got := calls.Load(); got > 11 {
		t.Fatalf("fetched %d of 30 messages after the search had failed, want at most 11", got)
	}
}
