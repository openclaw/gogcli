package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestSanitizeMessageBody_TruncateUTF8(t *testing.T) {
	long := strings.Repeat("€", gmailDefaultTextBodyLimit+10)
	got := sanitizeMessageBody(long, false)
	if !strings.HasSuffix(got, gmailTextTruncationMarker) {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
	preview := strings.TrimSuffix(got, gmailTextTruncationMarker)
	if len([]rune(preview)) != gmailDefaultTextBodyLimit {
		t.Fatalf("expected %d preview runes, got %d", gmailDefaultTextBodyLimit, len([]rune(preview)))
	}
}

func TestSanitizeMessageBody_FullSkipsTruncation(t *testing.T) {
	long := strings.Repeat("€", gmailDefaultTextBodyLimit+10)
	got := sanitizeMessageBody(long, true)
	if strings.Contains(got, "[truncated") {
		t.Fatalf("expected no truncation with full=true, got %q", got)
	}
	if len([]rune(got)) != gmailDefaultTextBodyLimit+10 {
		t.Fatalf("expected %d runes, got %d", gmailDefaultTextBodyLimit+10, len([]rune(got)))
	}
}

func TestSanitizeMessageBody_DefaultShowsTypicalBody(t *testing.T) {
	body := strings.Repeat("ordinary message body ", 500)
	got := sanitizeMessageBody(body, false)
	if got != strings.TrimSpace(body) {
		t.Fatalf("expected ordinary body in full, got %q", got)
	}
}

func TestSanitizeMessageBody_StripsHTML(t *testing.T) {
	got := sanitizeMessageBody("<html><body>Hi</body></html>", false)
	if got != "Hi" {
		t.Fatalf("unexpected sanitized body: %q", got)
	}
}

func TestFetchMessageDetails_NoRetryOnError(t *testing.T) {
	var m1Calls, m2Calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/users/me/messages/m1") {
			atomic.AddInt32(&m1Calls, 1)
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		atomic.AddInt32(&m2Calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m2","threadId":"t2","payload":{"headers":[{"name":"From","value":"me@example.com"}]}}`))
	}))
	defer srv.Close()

	svc, err := gmail.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	messages := []*gmail.Message{{Id: "m1"}, {Id: "m2"}}
	_, err = fetchMessageDetails(context.Background(), svc, messages, map[string]string{}, time.UTC, false, gmailMessageBodyFormatText, false, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "message m1") {
		t.Fatalf("expected message error, got %v", err)
	}
	// The failed message is not retried. The first failure cancels the
	// remaining fetches, so m2 may or may not have been requested by then.
	if got := atomic.LoadInt32(&m1Calls); got != 1 {
		t.Fatalf("expected 1 call for m1, got %d", got)
	}
	if got := atomic.LoadInt32(&m2Calls); got > 1 {
		t.Fatalf("expected at most 1 call for m2, got %d", got)
	}
}
