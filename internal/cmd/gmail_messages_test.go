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
	long := strings.Repeat("€", 210)
	got := sanitizeMessageBody(long, false)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
	if len([]rune(got)) != 200 {
		t.Fatalf("expected 200 runes, got %d", len([]rune(got)))
	}
}

func TestSanitizeMessageBody_FullSkipsTruncation(t *testing.T) {
	long := strings.Repeat("€", 210)
	got := sanitizeMessageBody(long, true)
	if strings.HasSuffix(got, "...") {
		t.Fatalf("expected no truncation with full=true, got %q", got)
	}
	if len([]rune(got)) != 210 {
		t.Fatalf("expected 210 runes, got %d", len([]rune(got)))
	}
}

func TestSanitizeMessageBody_StripsHTML(t *testing.T) {
	got := sanitizeMessageBody("<html><body>Hi</body></html>", false)
	if got != "Hi" {
		t.Fatalf("unexpected sanitized body: %q", got)
	}
}

func TestFetchMessageDetails_NoRetryOnError(t *testing.T) {
	var calls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&calls, 1)
		if strings.Contains(r.URL.Path, "/users/me/messages/m1") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
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
	_, err = fetchMessageDetails(context.Background(), svc, messages, map[string]string{}, time.UTC, false)
	if err == nil || !strings.Contains(err.Error(), "message m1") {
		t.Fatalf("expected message error, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}
