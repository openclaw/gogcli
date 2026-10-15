package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/config"
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

// TestGmailForward_SubjectNoteAndNoThreading checks what both forward commands
// build around the forwarded block (gmail_forward_header_test.go checks the
// block itself): a Fwd: subject, the note and the original's body, and no claim
// on the original's thread.
func TestGmailForward_SubjectNoteAndNoThreading(t *testing.T) {
	for _, cmd := range forwardCommands {
		t.Run(cmd.name, func(t *testing.T) {
			raw, threadID := captureForwardRaw(t, forwardArgs(cmd.verb, "--note", "FYI see below"), cmd.finalizePath, mockForwardSourceMessage)
			if got := rawMessageHeader(t, raw, "Subject"); got != "Fwd: Original Subject" {
				t.Errorf("Subject = %q, want %q", got, "Fwd: Original Subject")
			}
			plain := rawTextPart(t, raw, "text/plain")
			for _, want := range []string{"FYI see below", "Hello, this is the body."} {
				if !strings.Contains(plain, want) {
					t.Errorf("text/plain missing %q:\n%s", want, plain)
				}
			}
			// A forward changes the subject, so it must not claim the original
			// Gmail thread or stamp reply headers.
			if threadID != "" {
				t.Errorf("forward claims thread %q, want none", threadID)
			}
			for _, h := range []string{"In-Reply-To:", "References:"} {
				if strings.Contains(raw, h) {
					t.Errorf("forward has a %s header:\n%s", h, raw)
				}
			}
		})
	}
}

func TestExecute_GmailForward_WithAttachments(t *testing.T) {
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
	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
		"--to", "recipient@example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if !attachmentFetched {
		t.Error("expected attachment to be fetched for re-attachment")
	}
}

func TestExecute_GmailForward_SkipAttachments(t *testing.T) {
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
	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
		"--to", "recipient@example.com",
		"--skip-attachments",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}

	if attachmentFetched {
		t.Error("expected attachments to NOT be fetched when --skip-attachments is set")
	}
}

func TestExecute_GmailForward_NoSendAccountBlocksBeforeSend(t *testing.T) {
	setTestConfigHome(t)
	if err := defaultConfigStoreForTest(t).Write(config.File{
		NoSendAccounts: map[string]bool{"me@example.com": true},
	}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()
	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
		"--to", "recipient@example.com",
	}, svc)
	if result.err == nil {
		t.Fatalf("expected no-send error")
	}
	if !strings.Contains(result.err.Error(), "no-send") {
		t.Fatalf("unexpected error: %v", result.err)
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

// TestExecute_GmailForward_CommaInDisplayName proves that a recipient whose
// display name contains a comma ("Smith, John" <john@example.com>) is parsed as
// a single recipient, not naively split on the comma — across --to, --cc, and
// --bcc (all emitted as real headers) and for both gmail forward and gmail
// drafts forward, which share GmailForwardOptions.
func TestExecute_GmailForward_CommaInDisplayName(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	cases := []struct {
		name         string
		verb         []string
		finalizePath string
	}{
		{name: "send", verb: []string{"gmail", "forward"}, finalizePath: "/gmail/v1/users/me/messages/send"},
		{name: "draft", verb: []string{"gmail", "drafts", "forward"}, finalizePath: "/gmail/v1/users/me/drafts"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--account", "me@example.com"}, tc.verb...)
			args = append(args, "orig-msg-1",
				"--to", `"Smith, John" <john@example.com>, other@example.com`,
				"--cc", `"Roe, Jane" <jane2@example.com>, z@y.com`,
				"--bcc", `"Loe, Jane" <jane3@example.com>, w@y.com`,
			)
			raw, _ := captureForwardRaw(t, args, tc.finalizePath, mockForwardSourceMessage)

			assertHeaderRecipients(t, raw, "To", []wantAddr{
				{name: "Smith, John", address: "john@example.com"},
				{address: "other@example.com"},
			})
			assertHeaderRecipients(t, raw, "Cc", []wantAddr{
				{name: "Roe, Jane", address: "jane2@example.com"},
				{address: "z@y.com"},
			})
			assertHeaderRecipients(t, raw, "Bcc", []wantAddr{
				{name: "Loe, Jane", address: "jane3@example.com"},
				{address: "w@y.com"},
			})
		})
	}
}

// TestExecute_GmailForward_OrdinaryMultiRecipient guards the regression case:
// a plain comma-separated list still yields the expected recipients.
func TestExecute_GmailForward_OrdinaryMultiRecipient(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	raw, _ := captureForwardRaw(t, []string{
		"--json", "--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
		"--to", "a@x.com, b@y.com",
	}, "/gmail/v1/users/me/messages/send", mockForwardSourceMessage)

	assertHeaderRecipients(t, raw, "To", []wantAddr{{address: "a@x.com"}, {address: "b@y.com"}})
}

// TestExecute_GmailForward_MalformedRecipientNoAPICall proves a malformed
// recipient on any of --to/--cc/--bcc surfaces a clear, flag-named error before
// any Gmail API request is made (no original-message fetch, no send/draft) — for
// both gmail forward and gmail drafts forward.
func TestExecute_GmailForward_MalformedRecipientNoAPICall(t *testing.T) {
	verbs := []struct {
		name string
		verb []string
	}{
		{name: "send", verb: []string{"gmail", "forward"}},
		{name: "draft", verb: []string{"gmail", "drafts", "forward"}},
	}
	flags := []string{"--to", "--cc", "--bcc"}
	for _, v := range verbs {
		for _, flag := range flags {
			t.Run(v.name+"/"+flag, func(t *testing.T) {
				requests := 0
				svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
					requests++
					http.NotFound(w, r)
				})
				defer cleanup()

				args := append([]string{"--account", "me@example.com"}, v.verb...)
				// A valid --to keeps the failure attributable to the flag under test
				// (the send path requires --to, which is checked after parsing).
				args = append(args, "orig-msg-1", "--to", "recipient@example.com", flag, "not an address <<>")
				result := executeWithGmailTestService(t, args, svc)
				if result.err == nil {
					t.Fatalf("expected error for malformed %s", flag)
				}
				if !strings.Contains(result.err.Error(), flag) {
					t.Fatalf("expected %s validation error, got: %v", flag, result.err)
				}
				if requests != 0 {
					t.Fatalf("expected no Gmail API requests, got %d", requests)
				}
			})
		}
	}
}

// TestExecute_GmailForward_DryRunReportsParsedRecipients proves the forward
// dry-run dict reports --to parsed address-aware (the display-name comma did not
// split into a third element) and that the dry-run makes no API call.
func TestExecute_GmailForward_DryRunReportsParsedRecipients(t *testing.T) {
	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--json", "--dry-run", "--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
		"--to", `"Smith, John" <john@example.com>, other@example.com`,
	}, svc)
	if code := ExitCode(result.err); code != 0 {
		t.Fatalf("expected clean dry-run exit (code 0), got code %d: %v", code, result.err)
	}
	assertDryRunRequestList(t, result.stdout, "to", []string{
		`"Smith, John" <john@example.com>`,
		"other@example.com",
	})
	if requests != 0 {
		t.Fatalf("expected no Gmail API requests, got %d", requests)
	}
}
