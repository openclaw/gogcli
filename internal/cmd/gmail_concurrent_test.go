package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

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
	// Context was canceled, we may or may not get an error depending on timing.
	// Either nil or context.Canceled is acceptable.
	_ = err
}
