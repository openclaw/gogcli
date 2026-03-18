package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/mail"
	"strings"
	"testing"
)

func TestRunGmailAutoReply_RepliesAndArchives(t *testing.T) {
	var sentRaw string
	var modifiedBody map[string]any

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{}})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
					{"id": "Label_1", "name": "SecurityAutoReplied", "type": "user"},
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/messages") && r.URL.Query().Get("q") != "":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{"id": "m1", "threadId": "t1"}},
			})
		case r.Method == http.MethodGet && path == "/users/me/messages/m1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX", "UNREAD"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "Reporter <reporter@example.com>"},
						{"name": "To", "value": "security@openclaw.ai"},
						{"name": "Subject", "value": "security issue"},
						{"name": "Message-ID", "value": "<orig@example.com>"},
					},
				},
			})
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode send: %v", err)
			}
			raw, _ := payload["raw"].(string)
			decoded, err := base64.RawURLEncoding.DecodeString(raw)
			if err != nil {
				t.Fatalf("decode raw: %v", err)
			}
			sentRaw = string(decoded)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "reply1", "threadId": "t1"})
		case r.Method == http.MethodPost && path == "/users/me/threads/t1/modify":
			if err := json.NewDecoder(r.Body).Decode(&modifiedBody); err != nil {
				t.Fatalf("decode modify: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	summary, err := runGmailAutoReply(context.Background(), svc, "clawdbot@gmail.com", gmailAutoReplyInput{
		Query:    "to:security@openclaw.ai in:inbox -label:SecurityAutoReplied",
		Max:      10,
		Body:     "please use GitHub private reporting",
		Label:    "SecurityAutoReplied",
		Archive:  true,
		MarkRead: true,
		SkipBulk: true,
	})
	if err != nil {
		t.Fatalf("runGmailAutoReply: %v", err)
	}
	if summary.Replied != 1 || summary.Skipped != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if !strings.Contains(sentRaw, "To: reporter@example.com") {
		t.Fatalf("expected reply recipient, got raw: %s", sentRaw)
	}
	if !strings.Contains(sentRaw, "Subject: Re: security issue") {
		t.Fatalf("expected reply subject, got raw: %s", sentRaw)
	}
	if !strings.Contains(sentRaw, "Auto-Submitted: auto-replied") {
		t.Fatalf("expected auto-submitted header, got raw: %s", sentRaw)
	}
	add := modifiedBody["addLabelIds"].([]any)
	remove := modifiedBody["removeLabelIds"].([]any)
	if len(add) != 1 || add[0] != "Label_1" {
		t.Fatalf("unexpected add labels: %#v", modifiedBody)
	}
	if len(remove) != 2 {
		t.Fatalf("unexpected remove labels: %#v", modifiedBody)
	}
}

func TestRunGmailAutoReply_SkipsBulkAndLabeled(t *testing.T) {
	var sendCalls int

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{}})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "SecurityAutoReplied", "type": "user"},
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/messages") && r.URL.Query().Get("q") != "":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{
					{"id": "m1", "threadId": "t1"},
					{"id": "m2", "threadId": "t2"},
				},
			})
		case r.Method == http.MethodGet && path == "/users/me/messages/m1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"Label_1"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "done@example.com"},
						{"name": "Subject", "value": "done"},
					},
				},
			})
		case r.Method == http.MethodGet && path == "/users/me/messages/m2":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m2",
				"threadId": "t2",
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": "newsletter@example.com"},
						{"name": "Subject", "value": "bulk"},
						{"name": "List-Id", "value": "list.example.com"},
					},
				},
			})
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			sendCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "reply1", "threadId": "t1"})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	summary, err := runGmailAutoReply(context.Background(), svc, "clawdbot@gmail.com", gmailAutoReplyInput{
		Query:    "in:inbox",
		Max:      10,
		Body:     "hello",
		Label:    "SecurityAutoReplied",
		SkipBulk: true,
	})
	if err != nil {
		t.Fatalf("runGmailAutoReply: %v", err)
	}
	if sendCalls != 0 {
		t.Fatalf("expected no send calls, got %d", sendCalls)
	}
	if summary.Skipped != 2 || summary.Replied != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestAutoReplySubject(t *testing.T) {
	if got := autoReplySubject("", "Hello"); got != "Re: Hello" {
		t.Fatalf("unexpected subject: %q", got)
	}
	if got := autoReplySubject("", "Re: Hello"); got != "Re: Hello" {
		t.Fatalf("unexpected subject: %q", got)
	}
	if got := autoReplySubject("Fixed", "Hello"); got != "Fixed" {
		t.Fatalf("unexpected override: %q", got)
	}
}

func TestAutoReplyRecipients(t *testing.T) {
	info := &replyInfo{
		FromAddr:    "Alice <alice@example.com>",
		ReplyToAddr: "Team <reply@example.com>",
	}
	got := autoReplyRecipients(info, []string{"clawdbot@gmail.com"})
	if len(got) != 1 || got[0] != "reply@example.com" {
		t.Fatalf("unexpected recipients: %#v", got)
	}
}

func TestSendMessageOptionsHeadersReachRFC822(t *testing.T) {
	raw, err := buildRFC822(mailOptions{
		From:              "bot@example.com",
		To:                []string{"user@example.com"},
		Subject:           "Hi",
		Body:              "Hello",
		AdditionalHeaders: map[string]string{"X-Test": "1"},
	}, nil)
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if got := msg.Header.Get("X-Test"); got != "1" {
		t.Fatalf("unexpected header: %q", got)
	}
}
