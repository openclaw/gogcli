package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestExecuteGmailAutoReplySendAsAlias(t *testing.T) {
	t.Run("alias", func(t *testing.T) {
		sendCalls, summary := executeGmailAutoReplyFrom(t, "alias@example.com")
		if sendCalls != 0 {
			t.Fatalf("expected no messages.send calls, got %d", sendCalls)
		}
		if summary.Replied != 0 || summary.Skipped != 1 || len(summary.Results) != 1 {
			t.Fatalf("unexpected summary: %#v", summary)
		}
		got := summary.Results[0]
		if got.Action != autoReplyActionSkipped || got.Reason != "no_reply_recipient" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

	t.Run("external", func(t *testing.T) {
		sendCalls, summary := executeGmailAutoReplyFrom(t, "alice@example.com")
		if sendCalls != 1 {
			t.Fatalf("expected one messages.send call, got %d", sendCalls)
		}
		if summary.Replied != 1 || summary.Skipped != 0 || len(summary.Results) != 1 {
			t.Fatalf("unexpected summary: %#v", summary)
		}
		got := summary.Results[0]
		if got.Action != "replied" || got.ReplyTo != "alice@example.com" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})
}

// executeGmailAutoReplyFrom runs `gmail autoreply` through Execute against an
// httptest Gmail server whose send-as list is the primary account plus
// alias@example.com. from is the matching message's From header.
func executeGmailAutoReplyFrom(t *testing.T, from string) (int, gmailAutoReplySummary) {
	t.Helper()
	setTestConfigHome(t)

	var sendCalls int
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/settings/sendAs":
			sendAsListHandler(w)
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/labels"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "Label_1", "name": "AutoReplied", "type": "user"},
				},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/messages") && r.URL.Query().Get("q") != "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"messages": []map[string]any{{"id": "m1", "threadId": "t1"}},
			})
		case r.Method == http.MethodGet && path == "/users/me/messages/m1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"headers": []map[string]any{
						{"name": "From", "value": from},
						{"name": "To", "value": "me@example.com"},
						{"name": "Subject", "value": "hello"},
						{"name": "Message-ID", "value": "<orig@example.com>"},
					},
				},
			})
		case r.Method == http.MethodPost && path == "/users/me/messages/send":
			sendCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "reply1", "threadId": "t1"})
		case r.Method == http.MethodPost && path == "/users/me/threads/t1/modify":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "t1"})
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.RequestURI(), http.StatusNotFound)
		}
	})
	defer cleanup()

	args := []string{
		"--json", "--no-input", "--account", "me@example.com",
		"gmail", "autoreply", "in:inbox", "--body", "thanks",
	}
	result := executeWithGmailTestService(t, args, svc)
	if result.err != nil {
		t.Fatalf("Execute(%v): %v\nstderr:\n%s\nstdout:\n%s", args, result.err, result.stderr, result.stdout)
	}

	var payload struct {
		AutoReply gmailAutoReplySummary `json:"autoReply"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, result.stdout)
	}
	return sendCalls, payload.AutoReply
}
