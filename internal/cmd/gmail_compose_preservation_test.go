package cmd

import (
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestGmailDraftsUpdate_PreservesUnspecifiedRecipients(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		to, cc, bcc string
	}{
		{name: "omitted", to: "to@example.com", cc: "copy@example.com", bcc: "blind@example.com"},
		{name: "replace cc", args: []string{"--cc", "new@example.com"}, to: "to@example.com", cc: "new@example.com", bcc: "blind@example.com"},
		{name: "clear cc", args: []string{"--cc", ""}, to: "to@example.com", bcc: "blind@example.com"},
		{name: "clear bcc", args: []string{"--bcc", ""}, to: "to@example.com", cc: "copy@example.com"},
		{name: "replace to", args: []string{"--to", "new@example.com"}, to: "new@example.com", cc: "copy@example.com", bcc: "blind@example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &draftReplyContextServer{existingHeaders: []map[string]any{
				{"name": "To", "value": "to@example.com"},
				{"name": "Cc", "value": "copy@example.com"},
				{"name": "Bcc", "value": "blind@example.com"},
			}}
			srv := newDraftReplyContextServer(t, cfg)
			args := append([]string{"d1", "--subject", "Updated", "--body", "New body"}, tc.args...)
			if err := runReplyCtxUpdate(t, srv, io.Discard, args...); err != nil {
				t.Fatal(err)
			}
			msg, err := mail.ReadMessage(strings.NewReader(cfg.rawPosted(t)))
			if err != nil {
				t.Fatal(err)
			}
			for header, want := range map[string]string{"To": tc.to, "Cc": tc.cc, "Bcc": tc.bcc} {
				if got := msg.Header.Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}
		})
	}
}

func TestGmailSend_ExplicitEmptyBodyWithAttachment(t *testing.T) {
	attachment := writeTempFile(t, "document.pdf", "synthetic attachment bytes")
	emptyBody := writeTempFile(t, "empty.txt", "")
	for _, bodyFlags := range [][]string{{"--body", ""}, {"--body-file", emptyBody}} {
		t.Run(bodyFlags[0], func(t *testing.T) {
			args := append([]string{"--json", "--account", "me@example.com", "gmail", "send", "--to", "printer@example.com", "--subject", "Print", "--attach", attachment}, bodyFlags...)
			raw, _ := captureComposeRaw(t, args, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)
			msg, err := mail.ReadMessage(strings.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/mixed" {
				t.Fatalf("Content-Type: %q, err=%v", mediaType, err)
			}
			reader := multipart.NewReader(msg.Body, params["boundary"])
			body, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(body)
			if err != nil || strings.TrimSpace(string(content)) != "" {
				t.Fatalf("body = %q, err=%v", content, err)
			}
			part, err := reader.NextPart()
			if err != nil || part.FileName() != "document.pdf" {
				t.Fatalf("attachment = %v, err=%v", part, err)
			}
		})
	}
	for _, flags := range [][]string{{"--attach", attachment}, {"--body", ""}, {"--body-file", emptyBody}} {
		args := append([]string{"--json", "--dry-run", "gmail", "send", "--to", "printer@example.com", "--subject", "Print"}, flags...)
		assertGmailComposeFailsFast(t, args, "--body", "required")
	}
}

func TestGmailSend_EmptyBodyAttachmentKeepsExplicitSignature(t *testing.T) {
	attachment := writeTempFile(t, "document.pdf", "synthetic attachment bytes")
	signature := writeTempFile(t, "signature.txt", "Requested signature")
	raw, _ := captureComposeRaw(t, []string{
		"--account", "me@example.com", "gmail", "send", "--to", "printer@example.com",
		"--subject", "Print", "--body", "", "--attach", attachment, "--signature-file", signature,
	}, "/gmail/v1/users/me/messages/send", mockReplySourceMessage)
	if !strings.Contains(raw, "Requested signature") {
		t.Fatal("attachment-only send discarded its explicit signature")
	}
}
