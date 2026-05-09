package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func TestSanitizeGmailBody(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		isHTML bool
		want   string
	}{
		{
			name:   "html strips scripts and visible urls",
			body:   `<script>fetch("https://tracker.example/open")</script><p>Hello https://phish.example/login</p>`,
			isHTML: true,
			want:   "Hello [url removed]",
		},
		{
			name:   "plain decodes entity-obfuscated url",
			body:   `open &#104;ttps://evil.example/path now`,
			isHTML: false,
			want:   "open [url removed] now",
		},
		{
			name:   "html keeps link text but drops href target",
			body:   `<p>Click <a href="https://evil.example">here</a></p>`,
			isHTML: true,
			want:   "Click here",
		},
		{
			name:   "style block removed",
			body:   `<style>body{background:url(https://tracker.example)}</style><p>Visible</p>`,
			isHTML: true,
			want:   "Visible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeGmailBody(tt.body, tt.isHTML); got != tt.want {
				t.Fatalf("sanitizeGmailBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGmailGetCmd_SanitizeContent_JSONUsesSafeEnvelope(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<html><body><script>fetch("https://tracker.example/open")</script><p>Hello https://phish.example/login</p></body></html>`,
	))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "m1",
			"threadId":     "t1",
			"labelIds":     []string{"INBOX"},
			"snippet":      "snippet https://snippet.example",
			"internalDate": "1766743200000",
			"payload": map[string]any{
				"mimeType": "text/html",
				"body":     map[string]any{"data": htmlBody},
				"headers": []map[string]any{
					{"name": "From", "value": "a@example.com"},
					{"name": "To", "value": "b@example.com"},
					{"name": "Subject", "value": "Visit https://evil.example now"},
					{"name": "Date", "value": "Fri, 26 Dec 2025 10:00:00 +0000"},
					{"name": "List-Unsubscribe", "value": "<https://unsub.example.com>"},
				},
			},
		})
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
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "get", "m1", "--sanitize-content"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.Contains(out, "https://") || strings.Contains(out, "tracker.example") || strings.Contains(out, htmlBody) {
		t.Fatalf("sanitized JSON leaked unsafe content: %s", out)
	}
	if strings.Contains(out, "payload") || strings.Contains(out, "unsubscribe") {
		t.Fatalf("sanitized JSON should not expose raw Gmail payload/unsubscribe: %s", out)
	}
	var parsed struct {
		Body    string `json:"body"`
		Message struct {
			ID      string            `json:"id"`
			Headers map[string]string `json:"headers"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if parsed.Body != "Hello [url removed]" {
		t.Fatalf("unexpected body: %q", parsed.Body)
	}
	if parsed.Message.Headers["subject"] != "Visit [url removed] now" {
		t.Fatalf("unexpected sanitized subject: %#v", parsed.Message.Headers)
	}
}

func TestGmailGetCmd_SanitizeContentRejectsRaw(t *testing.T) {
	err := Execute([]string{"--account", "a@b.com", "gmail", "get", "m1", "--format", "raw", "--sanitize-content"})
	if err == nil || !strings.Contains(err.Error(), "--sanitize-content cannot be used with --format raw") {
		t.Fatalf("expected raw/sanitize usage error, got: %v", err)
	}
}

func TestGmailThreadGet_SanitizeContent_JSONUsesSafeEnvelope(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	htmlBody := base64.RawURLEncoding.EncodeToString([]byte(
		`<style>.x{background:url(https://tracker.example)}</style><p>Hello https://phish.example/login</p>`,
	))
	threadResp := map[string]any{
		"id": "t1",
		"messages": []map[string]any{
			{
				"id":       "m1",
				"threadId": "t1",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "a@example.com"},
						{"name": "To", "value": "b@example.com"},
						{"name": "Subject", "value": "Check https://evil.example now"},
						{"name": "Date", "value": "Mon, 1 Jan 2025 00:00:00 +0000"},
						{"name": "List-Unsubscribe", "value": "<https://unsub.example.com>"},
					},
					"mimeType": "text/html",
					"body":     map[string]any{"data": htmlBody},
				},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		if r.Method == http.MethodGet && path == "/users/me/threads/t1" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(threadResp)
			return
		}
		http.NotFound(w, r)
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
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			err := Execute([]string{"--json", "--account", "a@b.com", "gmail", "thread", "get", "t1", "--sanitize-content"})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.Contains(out, "https://") || strings.Contains(out, "tracker.example") || strings.Contains(out, htmlBody) {
		t.Fatalf("sanitized thread JSON leaked unsafe content: %s", out)
	}
	if strings.Contains(out, "payload") || strings.Contains(out, "unsubscribe") {
		t.Fatalf("sanitized thread JSON should not expose raw Gmail payload/unsubscribe: %s", out)
	}
	var parsed struct {
		Thread struct {
			Messages []gmailSanitizedMessageOutput `json:"messages"`
		} `json:"thread"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(parsed.Thread.Messages) != 1 {
		t.Fatalf("unexpected messages: %#v", parsed.Thread.Messages)
	}
	if got := parsed.Thread.Messages[0].Body; got != "Hello [url removed]" {
		t.Fatalf("unexpected body: %q", got)
	}
}
