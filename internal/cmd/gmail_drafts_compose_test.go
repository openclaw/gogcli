package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/config"
)

// mockReplySourceMessage returns a gmail.Message JSON payload suitable as the
// target of a reply: it has From/To/Cc/Subject/Message-ID headers and a plain
// text body.
func mockReplySourceMessage() map[string]any {
	plain := base64.RawURLEncoding.EncodeToString([]byte("Original plain body."))
	return map[string]any{
		"id":       "msg-1",
		"threadId": "thread-1",
		"payload": map[string]any{
			"mimeType": "text/plain",
			"headers": []map[string]any{
				{"name": "Message-ID", "value": "<original@example.com>"},
				{"name": "References", "value": "<root@example.com>"},
				{"name": "From", "value": `"Alice Sender" <alice@example.com>`},
				{"name": "To", "value": `"Me Person" <me@example.com>, "Other Person" <other@example.com>`},
				{"name": "Cc", "value": `"CC Person" <cc@example.com>`},
				{"name": "Date", "value": "Fri, 12 Jun 2026 10:00:00 +0000"},
				{"name": "Subject", "value": "Project update"},
			},
			"body": map[string]any{"data": plain, "size": len(plain)},
		},
	}
}

// mockReplySourceMessageWithInlineImage returns a reply target whose HTML body
// references a CID inline image carried as a multipart/related part. The image
// bytes are embedded inline, so no separate attachment fetch is needed.
func mockReplySourceMessageWithInlineImage() map[string]any {
	htmlBody := `<p>Original HTML<img src="cid:image-1@example.com"></p>`
	return map[string]any{
		"id":       "msg-1",
		"threadId": "thread-1",
		"payload": map[string]any{
			"mimeType": "multipart/related",
			"headers": []map[string]any{
				{"name": "Message-ID", "value": "<original@example.com>"},
				{"name": "References", "value": "<root@example.com>"},
				{"name": "From", "value": `"Alice Sender" <alice@example.com>`},
				{"name": "To", "value": `"Me Person" <me@example.com>`},
				{"name": "Date", "value": "Fri, 12 Jun 2026 10:00:00 +0000"},
				{"name": "Subject", "value": "Project update"},
			},
			"parts": []map[string]any{
				{
					"mimeType": "multipart/alternative",
					"parts": []map[string]any{
						{
							"mimeType": "text/plain",
							"body":     map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("Original plain")), "size": 14},
						},
						{
							"mimeType": "text/html",
							"body":     map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte(htmlBody)), "size": len(htmlBody)},
						},
					},
				},
				{
					"mimeType": "image/png",
					"filename": "inline.png",
					"headers": []map[string]any{
						{"name": "Content-ID", "value": "<image-1@example.com>"},
						{"name": "Content-Disposition", "value": `inline; filename="inline.png"`},
					},
					"body": map[string]any{"data": base64.RawURLEncoding.EncodeToString([]byte("png-data")), "size": 8},
				},
			},
		},
	}
}

func sendAsListHandler(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sendAs": []map[string]any{
			{"sendAsEmail": "me@example.com", "displayName": "Me Person", "isPrimary": true, "verificationStatus": "accepted"},
			{"sendAsEmail": "alias@example.com", "displayName": "Alias", "verificationStatus": "accepted"},
		},
	})
}

// normalizeRawForParity strips the nondeterministic parts of a built RFC822
// message so two independent builds can be compared byte-for-byte: the Date and
// Message-ID header lines, and the randomly generated MIME boundary tokens
// (gogcli_...). Everything else must match exactly.
func normalizeRawForParity(raw string) string {
	dateRE := regexp.MustCompile(`(?m)^Date: .*\r?$`)
	msgIDRE := regexp.MustCompile(`(?m)^Message-ID: .*\r?$`)
	// Boundaries are "gogcli_" + base64url, so the charset is [A-Za-z0-9_-].
	boundaryRE := regexp.MustCompile(`gogcli_[A-Za-z0-9_-]+`)
	raw = dateRE.ReplaceAllString(raw, "Date: NORMALIZED")
	raw = msgIDRE.ReplaceAllString(raw, "Message-ID: NORMALIZED")
	raw = boundaryRE.ReplaceAllString(raw, "gogcli_BOUNDARY")
	return raw
}

// handleFinalizeRaw services the finalize POST shared by the send and draft
// paths: it decodes the outgoing message (from a Draft body when finalizePath is
// the drafts endpoint, otherwise from a bare Message), writes the canned finalize
// response, and returns the decoded RFC822 raw plus the stamped ThreadId.
func handleFinalizeRaw(t *testing.T, w http.ResponseWriter, r *http.Request, finalizePath string) (raw, threadID string) {
	t.Helper()
	var msg *gmail.Message
	if finalizePath == "/gmail/v1/users/me/drafts" {
		var draft gmail.Draft
		if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
			t.Fatalf("decode draft: %v", err)
		}
		msg = draft.Message
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
	} else {
		var m gmail.Message
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Fatalf("decode send: %v", err)
		}
		msg = &m
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-1", "threadId": "thread-1"})
	}
	if msg == nil {
		t.Fatalf("nil message in finalize body")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(msg.Raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	return string(decoded), msg.ThreadId
}

// captureReplyRaw runs a reply-style command (either send or draft) against a
// mock Gmail server and returns the raw RFC822 of the outgoing message plus the
// stamped ThreadId. finalizePath is the API path the command finalizes through
// ("/gmail/v1/users/me/messages/send" or "/gmail/v1/users/me/drafts"). source
// supplies the reply-target message payload.
func captureReplyRaw(t *testing.T, args []string, finalizePath string, source func() map[string]any) (raw, threadID string) {
	t.Helper()
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(source())
		case r.Method == http.MethodPost && r.URL.Path == finalizePath:
			raw, threadID = handleFinalizeRaw(t, w, r, finalizePath)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, args, svc)
	if result.err != nil {
		t.Fatalf("Execute(%v): %v", args, result.err)
	}
	return raw, threadID
}

// TestGmailDraftsReply_ByteIdenticalToReply proves that gmail reply and gmail
// drafts reply build the exact same outgoing message from identical inputs; the
// only difference is the finalize step (Messages.Send vs Drafts.Create).
func TestGmailDraftsReply_ByteIdenticalToReply(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	base := []string{"--account", "me@example.com"}
	replyArgs := append(append([]string{}, base...), "gmail", "reply", "msg-1", "--body", "Thanks for the update")
	draftArgs := append(append([]string{}, base...), "gmail", "drafts", "reply", "msg-1", "--body", "Thanks for the update")

	sentRaw, sentThread := captureReplyRaw(t, replyArgs, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)
	draftRaw, draftThread := captureReplyRaw(t, draftArgs, "/gmail/v1/users/me/drafts", mockReplySourceMessage)

	if normalizeRawForParity(sentRaw) != normalizeRawForParity(draftRaw) {
		t.Fatalf("reply vs drafts reply raw differ:\n--- send ---\n%s\n--- draft ---\n%s", sentRaw, draftRaw)
	}
	if sentThread != draftThread {
		t.Fatalf("threadId differs: send=%q draft=%q", sentThread, draftThread)
	}
	if draftThread != "thread-1" {
		t.Fatalf("expected draft to stamp thread-1, got %q", draftThread)
	}
}

// TestGmailDraftsReply_ByteIdenticalToReply_WithInlineImage proves the draft
// path preserves CID inline images identically to the send path: the full
// RFC822 raw (multipart/related with the image part) must match byte-for-byte.
func TestGmailDraftsReply_ByteIdenticalToReply_WithInlineImage(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	base := []string{"--account", "me@example.com"}
	replyArgs := append(append([]string{}, base...), "gmail", "reply", "msg-1", "--body", "Thanks for the update")
	draftArgs := append(append([]string{}, base...), "gmail", "drafts", "reply", "msg-1", "--body", "Thanks for the update")

	sentRaw, _ := captureReplyRaw(t, replyArgs, "/gmail/v1/users/me/messages/send", mockReplySourceMessageWithInlineImage)
	draftRaw, _ := captureReplyRaw(t, draftArgs, "/gmail/v1/users/me/drafts", mockReplySourceMessageWithInlineImage)

	if normalizeRawForParity(sentRaw) != normalizeRawForParity(draftRaw) {
		t.Fatalf("reply vs drafts reply raw differ (inline image):\n--- send ---\n%s\n--- draft ---\n%s", sentRaw, draftRaw)
	}
	// Sanity-check the inline image actually rode along on both paths.
	for _, want := range []string{"Content-ID: <image-1@example.com>", "cid:image-1@example.com"} {
		if !strings.Contains(draftRaw, want) {
			t.Fatalf("drafts reply missing inline-image marker %q:\n%s", want, draftRaw)
		}
	}
}

func mockForwardSourceMessage() map[string]any {
	return mockOriginalMessage(false)
}

// captureForwardRaw runs a forward-style command and returns the raw RFC822 and
// stamped ThreadId. source supplies the original-message payload; when it
// references attachmentIds (e.g. mockOriginalMessage(true)), the attachment
// bytes are served from the attachments endpoint.
func captureForwardRaw(t *testing.T, args []string, finalizePath string, source func() map[string]any) (raw, threadID string) {
	t.Helper()
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/attachments/"):
			// Original message attachments (e.g. report.pdf / att-123) re-attached
			// on forward. Deterministic bytes so the parity comparison holds.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": base64.RawURLEncoding.EncodeToString([]byte("pdf-file-contents")),
				"size": 100,
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(source())
		case r.Method == http.MethodPost && r.URL.Path == finalizePath:
			raw, threadID = handleFinalizeRaw(t, w, r, finalizePath)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, args, svc)
	if result.err != nil {
		t.Fatalf("Execute(%v): %v", args, result.err)
	}
	return raw, threadID
}

func TestGmailDraftsForward_ByteIdenticalToForward(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	base := []string{"--account", "me@example.com"}
	fwdArgs := append(append([]string{}, base...), "gmail", "forward", "orig-msg-1", "--to", "recipient@example.com", "--note", "FYI")
	draftArgs := append(append([]string{}, base...), "gmail", "drafts", "forward", "orig-msg-1", "--to", "recipient@example.com", "--note", "FYI")

	sentRaw, sentThread := captureForwardRaw(t, fwdArgs, "/gmail/v1/users/me/messages/send", mockForwardSourceMessage)
	draftRaw, draftThread := captureForwardRaw(t, draftArgs, "/gmail/v1/users/me/drafts", mockForwardSourceMessage)

	if normalizeRawForParity(sentRaw) != normalizeRawForParity(draftRaw) {
		t.Fatalf("forward vs drafts forward raw differ:\n--- send ---\n%s\n--- draft ---\n%s", sentRaw, draftRaw)
	}
	if sentThread != "" || draftThread != "" {
		t.Fatalf("forward must not stamp a thread: send=%q draft=%q", sentThread, draftThread)
	}
	// Forward draft must carry no reply headers and no ThreadId.
	for _, h := range []string{"In-Reply-To:", "References:"} {
		if strings.Contains(draftRaw, h) {
			t.Fatalf("forward draft unexpectedly contains %q:\n%s", h, draftRaw)
		}
	}
}

// TestGmailDraftsForward_ByteIdenticalToForward_WithAttachments proves the draft
// path re-attaches the original message's attachments identically to the send
// path: the full RFC822 raw (including the re-attached file part) must match
// byte-for-byte. This also exercises the forward path's nil-metadata wiring.
func TestGmailDraftsForward_ByteIdenticalToForward_WithAttachments(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	withAttachment := func() map[string]any { return mockOriginalMessage(true) }
	base := []string{"--account", "me@example.com"}
	fwdArgs := append(append([]string{}, base...), "gmail", "forward", "orig-msg-1", "--to", "recipient@example.com", "--note", "FYI")
	draftArgs := append(append([]string{}, base...), "gmail", "drafts", "forward", "orig-msg-1", "--to", "recipient@example.com", "--note", "FYI")

	sentRaw, _ := captureForwardRaw(t, fwdArgs, "/gmail/v1/users/me/messages/send", withAttachment)
	draftRaw, _ := captureForwardRaw(t, draftArgs, "/gmail/v1/users/me/drafts", withAttachment)

	if normalizeRawForParity(sentRaw) != normalizeRawForParity(draftRaw) {
		t.Fatalf("forward vs drafts forward raw differ (with attachment):\n--- send ---\n%s\n--- draft ---\n%s", sentRaw, draftRaw)
	}
	// Sanity-check the attachment actually rode along on the draft path.
	if !strings.Contains(draftRaw, "report.pdf") {
		t.Fatalf("drafts forward missing re-attached file:\n%s", draftRaw)
	}
}

// --- No-send guard: drafts compose works under no-send; reply is blocked. ---

func writeNoSendConfig(t *testing.T) {
	t.Helper()
	setTestConfigHome(t)
	if err := defaultConfigStoreForTest(t).Write(config.File{GmailNoSend: true}); err != nil {
		t.Fatalf("write no-send config: %v", err)
	}
}

func TestGmailDraftsReply_SucceedsUnderNoSendFlag(t *testing.T) {
	created := false
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(mockReplySourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			created = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
			t.Fatalf("drafts reply must not call Messages.Send")
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--gmail-no-send", "--account", "me@example.com",
		"gmail", "drafts", "reply", "msg-1", "--body", "hi",
	}, svc)
	if result.err != nil {
		t.Fatalf("drafts reply under --gmail-no-send: %v", result.err)
	}
	if !created {
		t.Fatal("expected Drafts.Create to be called")
	}
}

func TestGmailDraftsReplyAll_SucceedsUnderNoSendConfig(t *testing.T) {
	writeNoSendConfig(t)
	created := false
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(mockReplySourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			created = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply-all", "msg-1", "--body", "hi",
	}, svc)
	if result.err != nil {
		t.Fatalf("drafts reply-all under config no-send: %v", result.err)
	}
	if !created {
		t.Fatal("expected Drafts.Create to be called")
	}
}

func TestGmailDraftsForward_SucceedsUnderNoSendFlag(t *testing.T) {
	created := false
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(mockForwardSourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			created = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--gmail-no-send", "--account", "me@example.com",
		"gmail", "drafts", "forward", "orig-msg-1", "--to", "recipient@example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("drafts forward under --gmail-no-send: %v", result.err)
	}
	if !created {
		t.Fatal("expected Drafts.Create to be called")
	}
}

func TestGmailReply_BlockedUnderNoSendFlag(t *testing.T) {
	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--gmail-no-send", "--account", "me@example.com",
		"gmail", "reply", "msg-1", "--body", "hi",
	}, svc)
	if result.err == nil {
		t.Fatal("expected gmail reply to be blocked by --gmail-no-send")
	}
	if !strings.Contains(result.err.Error(), "no-send") {
		t.Fatalf("unexpected error: %v", result.err)
	}
	if requests != 0 {
		t.Fatalf("expected no Gmail API requests, got %d", requests)
	}
}

// --- Per-command behavior parity with the send path. ---

func TestGmailDraftsReply_QuoteByDefaultAndAdditiveRecipients(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	raw, threadID := captureReplyRaw(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply", "msg-1",
		"--body", "Thanks",
		"--cc", "extra@example.com",
		"--remove", "cc@example.com",
	}, "/gmail/v1/users/me/drafts", mockReplySourceMessage)

	for _, want := range []string{
		"Subject: Re: Project update",
		"In-Reply-To: <original@example.com>",
		"References: <root@example.com> <original@example.com>",
		"Original plain body.", // quote-by-default includes the original
		"Cc: extra@example.com",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("drafts reply missing %q:\n%s", want, raw)
		}
	}
	if strings.Contains(raw, "cc@example.com") {
		t.Fatalf("removed Cc recipient still present:\n%s", raw)
	}
	if threadID != "thread-1" {
		t.Fatalf("threadId = %q, want thread-1", threadID)
	}
}

func TestGmailDraftsReplyAll_DerivesRecipients(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	raw, _ := captureReplyRaw(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply-all", "msg-1",
		"--body", "Thanks",
	}, "/gmail/v1/users/me/drafts", mockReplySourceMessage)

	// reply-all addresses the original sender (To) and keeps the other original
	// recipients, while dropping the account's own address from To/Cc.
	if !strings.Contains(raw, "alice@example.com") {
		t.Fatalf("reply-all missing original sender:\n%s", raw)
	}
	if !strings.Contains(raw, "other@example.com") || !strings.Contains(raw, "cc@example.com") {
		t.Fatalf("reply-all missing carried recipients:\n%s", raw)
	}
	// Self-exclusion: the account address must not appear in the To/Cc recipient
	// header fields (it legitimately appears in From:).
	headerBlock, _, _ := strings.Cut(raw, "\r\n\r\n")
	for _, field := range []string{"To", "Cc"} {
		fieldRE := regexp.MustCompile(`(?mi)^` + field + `:.*$`)
		for _, line := range fieldRE.FindAllString(headerBlock, -1) {
			if strings.Contains(line, "me@example.com") {
				t.Fatalf("reply-all leaked account address into %s header: %q", field, line)
			}
		}
	}
}

func TestGmailDraftsReply_SubjectOverrideClearsThread(t *testing.T) {
	raw, threadID := captureReplyRaw(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply", "msg-1",
		"--body", "New topic",
		"--subject", "Completely different",
		"--no-quote",
	}, "/gmail/v1/users/me/drafts", mockReplySourceMessage)

	if threadID != "" {
		t.Fatalf("edited subject should clear thread, got %q", threadID)
	}
	if !strings.Contains(raw, "Subject: Completely different") {
		t.Fatalf("expected overridden subject:\n%s", raw)
	}
}

func TestGmailDraftsReply_NoQuoteOmitsOriginal(t *testing.T) {
	raw, _ := captureReplyRaw(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply", "msg-1",
		"--body", "Short reply",
		"--no-quote",
	}, "/gmail/v1/users/me/drafts", mockReplySourceMessage)

	if strings.Contains(raw, "Original plain body.") {
		t.Fatalf("--no-quote should omit original body:\n%s", raw)
	}
}

func TestGmailDraftsForward_BodyFormatAndNoReplyHeaders(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	raw, threadID := captureForwardRaw(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "forward", "orig-msg-1",
		"--to", "recipient@example.com",
		"--note", "FYI see below",
	}, "/gmail/v1/users/me/drafts", mockForwardSourceMessage)

	for _, want := range []string{
		"Subject: Fwd: Original Subject",
		"---------- Forwarded message ---------",
		"From: Alice <alice@example.com>",
		"FYI see below",
		"Hello, this is the body.",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("forward draft missing %q:\n%s", want, raw)
		}
	}
	if threadID != "" {
		t.Fatalf("forward draft must not stamp a thread, got %q", threadID)
	}
	for _, h := range []string{"In-Reply-To:", "References:"} {
		if strings.Contains(raw, h) {
			t.Fatalf("forward draft unexpectedly contains %q:\n%s", h, raw)
		}
	}
}

// --- Addressless forward draft: allowed for draft, rejected for send. ---

// newForwardDraftCaptureService builds a mock Gmail service for the drafts-forward
// path: it serves sendAs, the orig-msg-1 source message, and captures the Raw of
// the created draft. The returned created/raw pointers are populated when the
// Drafts.Create POST fires.
func newForwardDraftCaptureService(t *testing.T) (svc *gmail.Service, cleanup func(), created *bool, raw *string) {
	t.Helper()
	created = new(bool)
	raw = new(string)
	svc, cleanup = newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(mockForwardSourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			*created = true
			var draft gmail.Draft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Fatalf("decode draft: %v", err)
			}
			decoded, _ := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			*raw = string(decoded)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
		default:
			http.NotFound(w, r)
		}
	})
	return svc, cleanup, created, raw
}

func TestGmailDraftsForward_NoRecipientsSucceeds(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	svc, cleanup, created, rawPtr := newForwardDraftCaptureService(t)
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "forward", "orig-msg-1",
	}, svc)
	if result.err != nil {
		t.Fatalf("addressless drafts forward: %v", result.err)
	}
	if !*created {
		t.Fatal("expected Drafts.Create to be called")
	}
	// Inspect only the envelope headers (before the first blank line); the
	// forwarded body legitimately quotes the original "To:" header.
	headerBlock, _, _ := strings.Cut(*rawPtr, "\r\n\r\n")
	if regexp.MustCompile(`(?m)^To:`).MatchString(headerBlock) {
		t.Fatalf("addressless draft unexpectedly has To header:\n%s", headerBlock)
	}
}

func TestGmailForward_NoRecipientsStillErrors(t *testing.T) {
	requests := 0
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "forward", "orig-msg-1",
	}, svc)
	if result.err == nil {
		t.Fatal("expected gmail forward to require --to")
	}
	if !strings.Contains(result.err.Error(), "--to") {
		t.Fatalf("unexpected error: %v", result.err)
	}
}

// --- Drafts compose dry-run action names. ---

func TestGmailDraftsCompose_DryRunActionNames(t *testing.T) {
	cases := []struct {
		name string
		args []string
		op   string
	}{
		{"reply", []string{"gmail", "drafts", "reply", "msg-1", "--body", "hi"}, "gmail.drafts.reply"},
		{"reply-all", []string{"gmail", "drafts", "reply-all", "msg-1", "--body", "hi"}, "gmail.drafts.reply-all"},
		{"forward", []string{"gmail", "drafts", "forward", "orig-msg-1", "--to", "a@example.com"}, "gmail.drafts.forward"},
		// Addressless draft-forward dry-run: the action must fire even with no
		// --to (the behavior distinguishing it from send-forward, which requires
		// --to before the dry-run). nil service proves the gate is never reached.
		{"forward-no-to", []string{"gmail", "drafts", "forward", "orig-msg-1"}, "gmail.drafts.forward"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--dry-run", "--account", "me@example.com"}, tc.args...)
			result := executeWithGmailTestService(t, args, nil)
			if result.err != nil {
				t.Fatalf("dry-run: %v", result.err)
			}
			if !strings.Contains(result.stdout, tc.op) {
				t.Fatalf("dry-run output missing action %q:\n%s", tc.op, result.stdout)
			}
		})
	}
}

// --- Stdin read-once on the resolve step. ---

func TestGmailDraftsReply_BodyFileStdinReadOnce(t *testing.T) {
	created := false
	var raw string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(mockReplySourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			created = true
			var draft gmail.Draft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Fatalf("decode draft: %v", err)
			}
			decoded, _ := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			raw = string(decoded)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	ctx := withGmailTestService(
		newCmdRuntimeIOContext(t, strings.NewReader("Body from stdin\n"), io.Discard, io.Discard),
		svc,
	)
	if err := runKong(t, &GmailDraftsReplyCmd{}, []string{"msg-1", "--body-file", "-"}, ctx, &RootFlags{Account: "me@example.com"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !created {
		t.Fatal("expected Drafts.Create to be called")
	}
	if !strings.Contains(raw, "Body from stdin") {
		t.Fatalf("expected stdin body in draft:\n%s", raw)
	}
}

func TestGmailDraftsForward_NoteFileStdinReadOnce(t *testing.T) {
	t.Setenv("GOG_TIMEZONE", "UTC")
	svc, cleanup, created, rawPtr := newForwardDraftCaptureService(t)
	defer cleanup()

	ctx := withGmailTestService(
		newCmdRuntimeIOContext(t, strings.NewReader("Note from stdin\n"), io.Discard, io.Discard),
		svc,
	)
	if err := runKong(t, &GmailDraftsForwardCmd{}, []string{"orig-msg-1", "--to", "r@example.com", "--note-file", "-"}, ctx, &RootFlags{Account: "me@example.com"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !*created {
		t.Fatal("expected Drafts.Create to be called")
	}
	if !strings.Contains(*rawPtr, "Note from stdin") {
		t.Fatalf("expected stdin note in draft:\n%s", *rawPtr)
	}
}

// --- Signature support on drafts reply (closes a pre-existing gap). ---

func TestGmailDraftsReply_WithSignatureFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "signature.txt")
	if err := os.WriteFile(path, []byte("Local Sig\nhttps://example.com"), 0o600); err != nil {
		t.Fatalf("write signature file: %v", err)
	}

	var raw string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(mockReplySourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			var draft gmail.Draft
			if err := json.NewDecoder(r.Body).Decode(&draft); err != nil {
				t.Fatalf("decode draft: %v", err)
			}
			decoded, _ := base64.RawURLEncoding.DecodeString(draft.Message.Raw)
			raw = string(decoded)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "d1", "message": map[string]any{"id": "m1"}})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply", "msg-1",
		"--body", "Body",
		"--no-quote",
		"--signature-file", path,
	}, svc)
	if result.err != nil {
		t.Fatalf("drafts reply with signature: %v", result.err)
	}
	if !strings.Contains(raw, "Body\r\n\r\n--\r\nLocal Sig\r\nhttps://example.com") {
		t.Fatalf("signature missing from draft:\n%s", raw)
	}
}

// --- Drafts.Create failure path: the new error wrappings are surfaced. ---

func TestGmailDraftsReply_CreateFailureWrapsError(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(mockReplySourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "reply", "msg-1", "--body", "hi", "--no-quote",
	}, svc)
	if result.err == nil {
		t.Fatal("expected Drafts.Create failure to surface as an error")
	}
	if !strings.Contains(result.err.Error(), "create reply draft") {
		t.Fatalf("error not wrapped with %q: %v", "create reply draft", result.err)
	}
}

func TestGmailDraftsForward_CreateFailureWrapsError(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/gmail/v1/users/me/messages/orig-msg-1"):
			_ = json.NewEncoder(w).Encode(mockForwardSourceMessage())
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/drafts":
			http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "drafts", "forward", "orig-msg-1", "--to", "recipient@example.com",
	}, svc)
	if result.err == nil {
		t.Fatal("expected Drafts.Create failure to surface as an error")
	}
	if !strings.Contains(result.err.Error(), "create forward draft") {
		t.Fatalf("error not wrapped with %q: %v", "create forward draft", result.err)
	}
}
