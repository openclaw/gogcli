package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A history listing that spans two pages: every record must be returned, and
// the stored cursor may only be the historyId of the page without a
// nextPageToken ("If you receive no nextPageToken in the response, there are no
// updates to retrieve and you can store the returned historyId").
func TestGmailWatchSourceListHistoryReadsEveryPage(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/history" {
			http.NotFound(w, r)
			return
		}
		token := r.URL.Query().Get("pageToken")
		calls = append(calls, token)
		w.Header().Set("Content-Type", "application/json")
		switch token {
		case "":
			_, _ = w.Write([]byte(`{"history":[{"id":"101","messagesAdded":[{"message":{"id":"m1"}}]}],"nextPageToken":"p2","historyId":"300"}`))
		case "p2":
			_, _ = w.Write([]byte(`{"history":[{"id":"250","messagesAdded":[{"message":{"id":"m2"}}]}],"historyId":"300"}`))
		default:
			http.Error(w, "unexpected page token", http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	source := &gmailWatchSource{service: newGmailServiceFromServer(t, srv)}
	page, err := source.ListHistory(context.Background(), 100, 1, nil)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(calls) != 2 || calls[1] != "p2" {
		t.Fatalf("history.list calls = %q, want first page then p2", calls)
	}
	if len(page.Records) != 2 || page.Records[1].Added[0] != "m2" {
		t.Fatalf("records = %#v, want both pages", page.Records)
	}
	if page.HistoryID != "300" {
		t.Fatalf("historyId = %q, want 300", page.HistoryID)
	}
}

func TestGmailWatchSourceListHistoryRejectsRepeatedPageToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"history":[{"id":"101"}],"nextPageToken":"same","historyId":"300"}`))
	}))
	t.Cleanup(srv.Close)

	source := &gmailWatchSource{service: newGmailServiceFromServer(t, srv)}
	if _, err := source.ListHistory(context.Background(), 100, 1, nil); err == nil {
		t.Fatal("ListHistory accepted a repeating page token; the cursor would advance past unread history")
	}
}
