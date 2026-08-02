package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestBuildReplyRecipientsPreservesNamesAndMovesRecipients(t *testing.T) {
	info := &replyInfo{
		FromAddr:    `"Alice Sender" <alice@example.com>`,
		ToHeader:    `"Me Person" <me@example.com>, "Other Person" <other@example.com>`,
		CcHeader:    `"Intro Person" <intro@example.com>, "CC Person" <cc@example.com>`,
		ReplyToAddr: "",
	}
	recipients, err := buildReplyRecipients(
		info,
		[]string{"me@example.com", "alias@example.com"},
		true,
		nil,
		nil,
		[]string{`"Intro Person" <intro@example.com>`},
		[]string{"cc@example.com"},
	)
	if err != nil {
		t.Fatalf("buildReplyRecipients: %v", err)
	}
	if got := formatMailboxes(recipients.To); strings.Join(got, "|") != `"Alice Sender" <alice@example.com>|"Other Person" <other@example.com>` {
		t.Fatalf("To = %#v", got)
	}
	if len(recipients.Cc) != 0 {
		t.Fatalf("Cc = %#v", formatMailboxes(recipients.Cc))
	}
	if got := formatMailboxes(recipients.Bcc); len(got) != 1 || got[0] != `"Intro Person" <intro@example.com>` {
		t.Fatalf("Bcc = %#v", got)
	}
}

func TestBuildReplyRecipientsOnlyFallsBackForSelfSentMessages(t *testing.T) {
	selfEmails := []string{"me@example.com", "alias@example.com"}

	selfSent, err := buildReplyRecipients(
		&replyInfo{
			FromAddr: `"Me Person" <me@example.com>`,
			ToHeader: `"Recipient" <recipient@example.com>`,
		},
		selfEmails,
		false,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("self-sent buildReplyRecipients: %v", err)
	}
	if got := formatMailboxes(selfSent.To); len(got) != 1 || got[0] != `"Recipient" <recipient@example.com>` {
		t.Fatalf("self-sent To = %#v", got)
	}

	_, err = buildReplyRecipients(
		&replyInfo{
			FromAddr:    `"External Sender" <sender@example.com>`,
			ReplyToAddr: `"Me Person" <me@example.com>`,
			ToHeader:    `"Unrelated Recipient" <recipient@example.com>`,
		},
		selfEmails,
		false,
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "reply has no recipients") {
		t.Fatalf("external sender with self Reply-To error = %v", err)
	}
}

func TestParseMailboxValuesPreservesCommaInDisplayName(t *testing.T) {
	addrs, err := parseMailboxValues("--to", []string{`"Bourgon, Malo" <malo@example.com>`})
	if err != nil {
		t.Fatalf("parseMailboxValues: %v", err)
	}
	if got := formatMailboxes(addrs); len(got) != 1 || got[0] != `"Bourgon, Malo" <malo@example.com>` {
		t.Fatalf("addresses = %#v", got)
	}
}

func TestGmailReplyDryRunPreservesCommaInDisplayNameFlag(t *testing.T) {
	result := executeWithGmailTestService(t, []string{
		"--dry-run",
		"--account", "me@example.com",
		"gmail", "reply", "msg-1",
		"--body", "Hello",
		"--to", `"Bourgon, Malo" <malo@example.com>`,
	}, nil)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stdout, "Bourgon, Malo") {
		t.Fatalf("dry-run output lost display name: %q", result.stdout)
	}
}

func TestExecuteGmailReplyAllDefaultsAndInlineImages(t *testing.T) {
	var sentRaw string
	var sentThreadID string
	htmlBody := `<p>Original HTML<img src="cid:image-1@example.com"></p>`

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sendAs": []map[string]any{
					{"sendAsEmail": "me@example.com", "displayName": "Me Person", "isPrimary": true, "verificationStatus": "accepted"},
					{"sendAsEmail": "alias@example.com", "displayName": "Alias", "verificationStatus": "accepted"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			if got := r.URL.Query().Get("format"); got != "full" {
				t.Fatalf("format = %q, want full", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg-1",
				"threadId": "thread-1",
				"payload": map[string]any{
					"mimeType": "multipart/related",
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<original@example.com>"},
						{"name": "References", "value": "<root@example.com>"},
						{"name": "From", "value": `"Alice Sender" <alice@example.com>`},
						{"name": "To", "value": `"Me Person" <me@example.com>, "Other Person" <other@example.com>`},
						{"name": "Cc", "value": `"Intro Person" <intro@example.com>, "CC Person" <cc@example.com>`},
						{"name": "Date", "value": "Fri, 12 Jun 2026 10:00:00 +0000"},
						{"name": "Subject", "value": "Project update"},
					},
					"parts": []map[string]any{
						{
							"mimeType": "multipart/alternative",
							"parts": []map[string]any{
								{
									"mimeType": "text/plain",
									"body": map[string]any{
										"data": base64.RawURLEncoding.EncodeToString([]byte("Original plain")),
										"size": 14,
									},
								},
								{
									"mimeType": "text/html",
									"body": map[string]any{
										"data": base64.RawURLEncoding.EncodeToString([]byte(htmlBody)),
										"size": len(htmlBody),
									},
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
							"body": map[string]any{
								"data": base64.RawURLEncoding.EncodeToString([]byte("png-data")),
								"size": 8,
							},
						},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read send body: %v", err)
			}
			var msg gmail.Message
			if unmarshalErr := json.Unmarshal(body, &msg); unmarshalErr != nil {
				t.Fatalf("decode send body: %v", unmarshalErr)
			}
			raw, err := base64.RawURLEncoding.DecodeString(msg.Raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			sentRaw = string(raw)
			sentThreadID = msg.ThreadId
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-1", "threadId": "thread-1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--json",
		"--account", "me@example.com",
		"gmail", "reply-all", "msg-1",
		"--body", "Thanks",
		"--bcc", `"Intro Person" <intro@example.com>`,
		"--remove", "cc@example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	for _, want := range []string{
		`Subject: Re: Project update`,
		`In-Reply-To: <original@example.com>`,
		`References: <root@example.com> <original@example.com>`,
		`Alice Sender`,
		`Other Person`,
		`Bcc: "Intro Person" <intro@example.com>`,
		`Content-Type: multipart/related`,
		`Content-ID: <image-1@example.com>`,
		`Content-Disposition: inline; filename="inline.png"`,
		`Original HTML`,
		`cid:image-1@example.com`,
	} {
		if !strings.Contains(sentRaw, want) {
			t.Fatalf("sent message missing %q:\n%s", want, sentRaw)
		}
	}
	if strings.Contains(sentRaw, "CC Person") || strings.Contains(sentRaw, "\r\nCc:") {
		t.Fatalf("removed Cc recipient still present:\n%s", sentRaw)
	}
	if sentThreadID != "thread-1" {
		t.Fatalf("threadId = %q, want thread-1", sentThreadID)
	}
}

func TestExecuteGmailReplyAutoSelectsFromAddressedAlias(t *testing.T) {
	sendReply := func(t *testing.T, originalTo string, autoSelect bool, extraArgs ...string) string {
		t.Helper()
		var sentRaw string
		svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
				_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{
					{"sendAsEmail": "me@example.com", "displayName": "Me Person", "isPrimary": true, "verificationStatus": "accepted"},
					{"sendAsEmail": "alias@example.com", "displayName": "Alias", "verificationStatus": "accepted"},
				}})
			case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "msg-1", "threadId": "thread-1",
					"payload": map[string]any{"headers": []map[string]any{
						{"name": "Message-ID", "value": "<original@example.com>"},
						{"name": "From", "value": "alice@example.com"},
						{"name": "To", "value": originalTo},
						{"name": "Subject", "value": "Hi"},
					}},
				})
			case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
				body, _ := io.ReadAll(r.Body)
				var msg gmail.Message
				_ = json.Unmarshal(body, &msg)
				raw, _ := base64.RawURLEncoding.DecodeString(msg.Raw)
				sentRaw = string(raw)
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-1", "threadId": "thread-1"})
			default:
				http.NotFound(w, r)
			}
		})
		defer cleanup()

		args := []string{"--json", "--account", "me@example.com", "gmail", "reply", "msg-1", "--body", "ok", "--no-quote"}
		if autoSelect {
			args = append(args, "--auto-from-addressed-alias")
		}
		args = append(args, extraArgs...)
		result := executeWithGmailTestService(t, args, svc)
		if result.err != nil {
			t.Fatalf("Execute: %v", result.err)
		}
		return sentRaw
	}

	fromAddr := func(raw string) string {
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(line, "From:") {
				return line
			}
		}
		return ""
	}

	t.Run("addressed to alias uses it as From", func(t *testing.T) {
		if from := fromAddr(sendReply(t, "alias@example.com", true)); !strings.Contains(from, "alias@example.com") {
			t.Fatalf("From is not the addressed alias: %q", from)
		}
	})
	t.Run("no alias match falls back to account default", func(t *testing.T) {
		if from := fromAddr(sendReply(t, "someone@else.com", true)); !strings.Contains(from, "me@example.com") {
			t.Fatalf("From is not the account default: %q", from)
		}
	})
	t.Run("explicit --from overrides auto-select", func(t *testing.T) {
		if from := fromAddr(sendReply(t, "alias@example.com", true, "--from", "me@example.com")); !strings.Contains(from, "me@example.com") {
			t.Fatalf("explicit --from not honored: %q", from)
		}
	})
	t.Run("omitted flag keeps account default", func(t *testing.T) {
		if from := fromAddr(sendReply(t, "alias@example.com", false)); !strings.Contains(from, "me@example.com") {
			t.Fatalf("From is not the account default: %q", from)
		}
	})
}

// TestExecuteGmailReplySignatureFollowsAutoSelectedAlias proves the signature is
// resolved for the auto-selected alias, not the account default: the alias must be
// chosen before the signature is fetched.
func TestExecuteGmailReplySignatureFollowsAutoSelectedAlias(t *testing.T) {
	var sigEmail, sentRaw string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{
				{"sendAsEmail": "me@example.com", "displayName": "Me Person", "isPrimary": true, "verificationStatus": "accepted"},
				{"sendAsEmail": "alias@example.com", "displayName": "Alias", "verificationStatus": "accepted"},
			}})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/gmail/v1/users/me/settings/sendAs/"):
			sigEmail = strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me/settings/sendAs/")
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAsEmail": sigEmail, "signature": "<div>sig for " + sigEmail + "</div>"})
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "msg-1", "threadId": "thread-1",
				"payload": map[string]any{"headers": []map[string]any{
					{"name": "Message-ID", "value": "<original@example.com>"},
					{"name": "From", "value": "alice@example.com"},
					{"name": "To", "value": "alias@example.com"},
					{"name": "Subject", "value": "Hi"},
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
			body, _ := io.ReadAll(r.Body)
			var msg gmail.Message
			_ = json.Unmarshal(body, &msg)
			raw, _ := base64.RawURLEncoding.DecodeString(msg.Raw)
			sentRaw = string(raw)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-1", "threadId": "thread-1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--json", "--account", "me@example.com", "gmail", "reply", "msg-1", "--body", "ok", "--no-quote", "--signature", "--auto-from-addressed-alias",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if sigEmail != "alias@example.com" {
		t.Fatalf("signature fetched for %q, want alias@example.com", sigEmail)
	}
	if !strings.Contains(sentRaw, "sig for alias@example.com") {
		t.Fatalf("sent body missing alias signature:\n%s", sentRaw)
	}
}

func TestExecuteGmailReplyEditedSubjectStartsNewThread(t *testing.T) {
	var sentThreadID string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{
				{"sendAsEmail": "me@example.com", "isPrimary": true, "verificationStatus": "accepted"},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "msg-2",
				"threadId": "thread-2",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "Message-ID", "value": "<original-2@example.com>"},
						{"name": "From", "value": "alice@example.com"},
						{"name": "To", "value": "me@example.com"},
						{"name": "Subject", "value": "Original"},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/gmail/v1/users/me/messages/send":
			var msg gmail.Message
			_ = json.NewDecoder(r.Body).Decode(&msg)
			sentThreadID = msg.ThreadId
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent-2", "threadId": "new-thread"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	result := executeWithGmailTestService(t, []string{
		"--account", "me@example.com",
		"gmail", "reply", "msg-2",
		"--body", "New topic",
		"--subject", "Different subject",
		"--no-quote",
	}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if sentThreadID != "" {
		t.Fatalf("edited subject unexpectedly retained threadId %q", sentThreadID)
	}
}

func TestPreserveReferencedInlineResourcesFetchesAttachmentBody(t *testing.T) {
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gmail/v1/users/me/messages/msg-3/attachments/inline-att" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": base64.RawURLEncoding.EncodeToString([]byte("inline-bytes")),
			"size": 12,
		})
	})
	defer cleanup()

	payload := &gmail.MessagePart{
		MimeType: "multipart/related",
		Parts: []*gmail.MessagePart{
			{
				MimeType: "image/png",
				Filename: "image.png",
				Headers: []*gmail.MessagePartHeader{
					{Name: "Content-ID", Value: "<image-3@example.com>"},
				},
				Body: &gmail.MessagePartBody{AttachmentId: "inline-att", Size: 12},
			},
		},
	}
	resources, err := preserveReferencedInlineResources(
		t.Context(),
		svc,
		"msg-3",
		payload,
		`<img src="cid:image-3%40example.com">`,
	)
	if err != nil {
		t.Fatalf("preserveReferencedInlineResources: %v", err)
	}
	if len(resources) != 1 || !resources[0].Inline || string(resources[0].Data) != "inline-bytes" {
		t.Fatalf("resources = %#v", resources)
	}
}

func TestPreserveReferencedInlineResourcesRejectsBrokenCID(t *testing.T) {
	_, err := preserveReferencedInlineResources(
		t.Context(),
		nil,
		"msg-4",
		&gmail.MessagePart{},
		`<img src="cid:missing@example.com">`,
	)
	if err == nil || !strings.Contains(err.Error(), "no matching MIME part") {
		t.Fatalf("expected missing CID error, got %v", err)
	}
}

func TestReferencedContentIDsOnlyScansResourceReferences(t *testing.T) {
	htmlBody := `<p>Literal <code>cid:not-a-resource</code></p>
<a href="https://example.test/?q=cid:not-a-resource">Link</a>
<img src="cid:image-1%40example.com" srcset="https://example.test/?q=cid:not-a-resource 1x, cid:image-2@example.com 2x">
<div style="background-image: url('cid:image-3@example.com')"></div>
<style>.hero { background: url(cid:image-4@example.com) }</style>`

	got := referencedContentIDs(htmlBody)
	want := []string{
		"image-1@example.com",
		"image-2@example.com",
		"image-3@example.com",
		"image-4@example.com",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("referencedContentIDs = %#v, want %#v", got, want)
	}

	resources, err := preserveReferencedInlineResources(
		t.Context(),
		nil,
		"msg-literal-cid",
		&gmail.MessagePart{},
		`<p>Document <code>cid:example</code> syntax.</p>`,
	)
	if err != nil {
		t.Fatalf("literal CID text returned error: %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("literal CID text returned resources: %#v", resources)
	}
}

func TestPreserveForwardMessagePartsIncludesNamelessAttachmentID(t *testing.T) {
	attachmentFetched := false
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/attachments/att-1") {
			http.NotFound(w, r)
			return
		}
		attachmentFetched = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": base64.RawURLEncoding.EncodeToString([]byte("attachment-data")),
			"size": 15,
		})
	})
	defer cleanup()

	attachments, err := preserveForwardMessageParts(
		t.Context(),
		svc,
		"msg-1",
		&gmail.MessagePart{
			MimeType: "multipart/mixed",
			Parts: []*gmail.MessagePart{{
				MimeType: "application/octet-stream",
				Body: &gmail.MessagePartBody{
					AttachmentId: "att-1",
					Size:         15,
				},
			}},
		},
		"",
		true,
	)
	if err != nil {
		t.Fatalf("preserveForwardMessageParts: %v", err)
	}
	if !attachmentFetched {
		t.Fatal("expected nameless attachment body to be fetched")
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want one", attachments)
	}
	if attachments[0].Filename != defaultAttachmentFilename {
		t.Fatalf("filename = %q, want %q", attachments[0].Filename, defaultAttachmentFilename)
	}
}
