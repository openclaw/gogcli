package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/config"
)

// mockOriginalMessage returns a gmail.Message JSON payload that looks like a
// typical email with headers, plain text body, HTML body, and optionally an
// attachment.
func mockOriginalMessage(withAttachment bool) map[string]any {
	textBody := base64.RawURLEncoding.EncodeToString([]byte("Hello, this is the body."))
	htmlBody := base64.RawURLEncoding.EncodeToString([]byte("<p>Hello, this is the body.</p>"))

	parts := []map[string]any{
		{
			"mimeType": "text/plain",
			"body":     map[string]any{"data": textBody, "size": len(textBody)},
		},
		{
			"mimeType": "text/html",
			"body":     map[string]any{"data": htmlBody, "size": len(htmlBody)},
		},
	}

	if withAttachment {
		attData := base64.RawURLEncoding.EncodeToString([]byte("file contents"))
		parts = append(parts, map[string]any{
			"mimeType": "application/pdf",
			"filename": "report.pdf",
			"body":     map[string]any{"attachmentId": "att-123", "size": 100},
		})
		_ = attData // attachment data fetched separately
	}

	return map[string]any{
		"id":       "orig-msg-1",
		"threadId": "thread-1",
		"payload": map[string]any{
			"mimeType": "multipart/alternative",
			"headers": []map[string]any{
				{"name": "From", "value": "Alice <alice@example.com>"},
				{"name": "To", "value": "bob@example.com"},
				{"name": "Cc", "value": "carol@example.com"},
				{"name": "Date", "value": "Mon, 10 Mar 2026 09:00:00 -0400"},
				{"name": "Subject", "value": "Original Subject"},
				{"name": "Message-ID", "value": "<orig-123@example.com>"},
			},
			"parts": parts,
		},
	}
}

func TestExecute_GmailForward_Basic(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	var sentRaw string
	var sentThreadID string

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(mockOriginalMessage(false))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/sendAs"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{"sendAsEmail": "me@example.com", "displayName": "Me", "isPrimary": true, "verificationStatus": "accepted"},
				},
			})

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/send"):
			body, _ := io.ReadAll(r.Body)
			var msg gmail.Message
			_ = json.Unmarshal(body, &msg)
			raw, _ := base64.RawURLEncoding.DecodeString(msg.Raw)
			sentRaw = string(raw)
			sentThreadID = msg.ThreadId
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-1", "threadId": "thread-1"})

		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "me@example.com",
				"gmail", "forward", "orig-msg-1",
				"--to", "recipient@example.com",
				"--note", "FYI see below",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	// Verify subject.
	if !strings.Contains(sentRaw, "Subject: Fwd: Original Subject") {
		t.Errorf("expected Fwd: subject, got:\n%s", sentRaw)
	}

	// Verify forward separator.
	if !strings.Contains(sentRaw, "---------- Forwarded message ---------") {
		t.Errorf("expected forwarded message separator in body")
	}

	// Verify original headers in body.
	if !strings.Contains(sentRaw, "From: Alice <alice@example.com>") {
		t.Errorf("expected original From in forwarded body")
	}
	if !strings.Contains(sentRaw, "Date: Mon, 10 Mar 2026 09:00:00 -0400") {
		t.Errorf("expected original Date in forwarded body")
	}

	// Verify note text.
	if !strings.Contains(sentRaw, "FYI see below") {
		t.Errorf("expected note text in body")
	}

	// Verify original body content.
	if !strings.Contains(sentRaw, "Hello, this is the body.") {
		t.Errorf("expected original body content in forwarded message")
	}

	// Verify thread ID was set (stays in original thread).
	if sentThreadID != "thread-1" {
		t.Errorf("expected threadId=thread-1, got %q", sentThreadID)
	}

	// Verify In-Reply-To header references original Message-ID.
	if !strings.Contains(sentRaw, "In-Reply-To: <orig-123@example.com>") {
		t.Errorf("expected In-Reply-To header referencing original message")
	}
}

func TestExecute_GmailForward_WithAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentFetched := false

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1/attachments/att-123"):
			attachmentFetched = true
			data := base64.RawURLEncoding.EncodeToString([]byte("pdf-file-contents"))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "size": 100})

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(mockOriginalMessage(true))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/sendAs"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{"sendAsEmail": "me@example.com", "displayName": "Me", "isPrimary": true, "verificationStatus": "accepted"},
				},
			})

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/send"):
			body, _ := io.ReadAll(r.Body)
			var msg gmail.Message
			_ = json.Unmarshal(body, &msg)
			raw, _ := base64.RawURLEncoding.DecodeString(msg.Raw)
			// Verify attachment is present in MIME.
			if !strings.Contains(string(raw), "report.pdf") {
				t.Errorf("expected attachment filename in MIME message")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-2", "threadId": "thread-1"})

		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "me@example.com",
				"gmail", "forward", "orig-msg-1",
				"--to", "recipient@example.com",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !attachmentFetched {
		t.Error("expected attachment to be fetched for re-attachment")
	}
}

func TestExecute_GmailForward_SkipAttachments(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	attachmentFetched := false

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/attachments/"):
			attachmentFetched = true
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "", "size": 0})

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(mockOriginalMessage(true))

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/settings/sendAs"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{"sendAsEmail": "me@example.com", "displayName": "Me", "isPrimary": true, "verificationStatus": "accepted"},
				},
			})

		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/send"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-3", "threadId": "thread-1"})

		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "me@example.com",
				"gmail", "forward", "orig-msg-1",
				"--to", "recipient@example.com",
				"--skip-attachments",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if attachmentFetched {
		t.Error("expected attachments to NOT be fetched when --skip-attachments is set")
	}
}

func TestExecute_GmailForward_NoSendAccountBlocksBeforeSend(t *testing.T) {
	setTestConfigHome(t)
	if err := config.WriteConfig(config.File{
		NoSendAccounts: map[string]bool{"me@example.com": true},
	}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	err := Execute([]string{
		"--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
		"--to", "recipient@example.com",
	})
	if err == nil {
		t.Fatalf("expected no-send error")
	}
	if !strings.Contains(err.Error(), "no-send") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no Gmail API requests, got %d", requests)
	}
}

func TestBuildForwardSubject(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Original Subject", "Fwd: Original Subject"},
		{"Fwd: Already Forwarded", "Fwd: Already Forwarded"},
		{"Fwd: " + "Fwd: Double", "Fwd: Double"},
		{"FWD: CAPS", "Fwd: CAPS"},
		{"Fw: Short prefix", "Fwd: Short prefix"},
		{"", "Fwd: (no subject)"},
		{"  ", "Fwd: (no subject)"},
		{"Re: A reply", "Fwd: Re: A reply"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := buildForwardSubject(tt.input)
			if got != tt.want {
				t.Errorf("buildForwardSubject(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripForwardPrefix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Fwd: Subject", "Subject"},
		{"fwd: lowercase", "lowercase"},
		{"FWD: UPPER", "UPPER"},
		{"Fw: Short", "Short"},
		{"Re: Not a forward", "Re: Not a forward"},
		{"Fwd: " + "Fwd: Double", "Double"},
		{"No prefix", "No prefix"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripForwardPrefix(tt.input)
			if got != tt.want {
				t.Errorf("stripForwardPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatForwardedMessage(t *testing.T) {
	result := formatForwardedMessage(
		"See below",
		"Alice <alice@example.com>",
		"Mon, 10 Mar 2026 09:00:00 -0400",
		"Test Subject",
		"bob@example.com",
		"carol@example.com",
		"Body text here.",
	)

	checks := []string{
		"See below",
		"---------- Forwarded message ---------",
		"From: Alice <alice@example.com>",
		"Date: Mon, 10 Mar 2026 09:00:00 -0400",
		"Subject: Test Subject",
		"To: bob@example.com",
		"Cc: carol@example.com",
		"Body text here.",
	}
	for _, want := range checks {
		if !strings.Contains(result, want) {
			t.Errorf("formatForwardedMessage missing %q, got:\n%s", want, result)
		}
	}
}

func TestFormatForwardedMessage_NoNote(t *testing.T) {
	result := formatForwardedMessage("", "from@x.com", "", "Subj", "to@x.com", "", "Body.")
	if strings.HasPrefix(result, "\n\n------") {
		// Should not have leading blank lines when note is empty.
		t.Errorf("expected no leading blank lines when note is empty")
	}
	if !strings.HasPrefix(result, "---------- Forwarded message") {
		t.Errorf("expected message to start with separator when no note")
	}
}

func TestFormatForwardedMessageHTML(t *testing.T) {
	result := formatForwardedMessageHTML(
		"Check this out",
		"Alice <alice@example.com>",
		"Mon, 10 Mar 2026",
		"Test",
		"bob@example.com",
		"",
		"<p>Original content</p>",
	)

	if !strings.Contains(result, "Check this out") {
		t.Error("missing note in HTML")
	}
	if !strings.Contains(result, "Forwarded message") {
		t.Error("missing forward separator in HTML")
	}
	if !strings.Contains(result, "Alice") {
		t.Error("missing sender name in HTML")
	}
	if !strings.Contains(result, "<p>Original content</p>") {
		t.Error("missing original HTML content")
	}
}
