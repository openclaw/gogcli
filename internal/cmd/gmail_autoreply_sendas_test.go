package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	googleapi "google.golang.org/api/googleapi"
)

func TestExecuteGmailAutoReplySendAsAlias(t *testing.T) {
	t.Run("alias", func(t *testing.T) {
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, "alias@example.com", "", false, 0)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
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
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, "alice@example.com", "", false, 0)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
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

	t.Run("display name", func(t *testing.T) {
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, `"Alias" <ALIAS@EXAMPLE.COM>`, "", false, 0)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
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

	t.Run("reply-to", func(t *testing.T) {
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, "alias@example.com", "victim@external.com", false, 0)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
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

	t.Run("reply-to allow-self", func(t *testing.T) {
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, "alias@example.com", "victim@external.com", true, 0)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if sendCalls != 1 {
			t.Fatalf("expected one messages.send call, got %d", sendCalls)
		}
		if summary.Replied != 1 || summary.Skipped != 0 || len(summary.Results) != 1 {
			t.Fatalf("unexpected summary: %#v", summary)
		}
		got := summary.Results[0]
		if got.Action != "replied" || got.ReplyTo != "victim@external.com" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

	t.Run("allow-self", func(t *testing.T) {
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, "alias@example.com", "", true, 0)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if sendCalls != 1 {
			t.Fatalf("expected one messages.send call, got %d", sendCalls)
		}
		if summary.Replied != 1 || summary.Skipped != 0 || len(summary.Results) != 1 {
			t.Fatalf("unexpected summary: %#v", summary)
		}
		got := summary.Results[0]
		if got.Action != "replied" || got.ReplyTo != "alias@example.com" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

	t.Run("send-as forbidden", func(t *testing.T) {
		sendCalls, _, err := executeGmailAutoReplyFrom(t, "alias@example.com", "", false, http.StatusForbidden)
		if err == nil {
			t.Fatal("expected error")
		}
		if sendCalls != 0 {
			t.Fatalf("expected no messages.send calls, got %d", sendCalls)
		}
		if !strings.Contains(err.Error(), "list send-as aliases to exclude your own addresses") {
			t.Fatalf("unexpected error: %v", err)
		}
		var apiErr *googleapi.Error
		if !errors.As(err, &apiErr) || apiErr.Code != http.StatusForbidden {
			t.Fatalf("expected wrapped 403, got %v", err)
		}
		if ExitCode(err) != exitCodePermissionDenied {
			t.Fatalf("exit code = %d, want %d", ExitCode(err), exitCodePermissionDenied)
		}
	})

	// --allow-self already permits replies to the user's own addresses, so a
	// failed send-as lookup keeps the previous sender fallback instead of
	// stopping the run.
	t.Run("send-as forbidden with allow-self", func(t *testing.T) {
		sendCalls, summary, err := executeGmailAutoReplyFrom(t, "alice@example.com", "", true, http.StatusForbidden)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if sendCalls != 1 || summary.Replied != 1 {
			t.Fatalf("sendCalls = %d, summary = %#v, want one reply", sendCalls, summary)
		}
	})
}

// executeGmailAutoReplyFrom runs `gmail autoreply` through Execute against an
// httptest Gmail server whose send-as list is the primary account plus
// alias@example.com. from and replyTo are the matching message's From and
// Reply-To headers. allowSelf sets --allow-self. sendAsStatus 0 serves the
// send-as list; any other status is returned by GET settings/sendAs.
// The helper fails the test unless that GET happens exactly once.
func executeGmailAutoReplyFrom(t *testing.T, from, replyTo string, allowSelf bool, sendAsStatus int) (int, gmailAutoReplySummary, error) {
	t.Helper()
	setTestConfigHome(t)

	var sendCalls, sendAsGets, followupCalls int
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		w.Header().Set("Content-Type", "application/json")
		sendAsList := r.Method == http.MethodGet && path == "/users/me/settings/sendAs"
		if !sendAsList {
			followupCalls++
		}
		switch {
		case sendAsList:
			sendAsGets++
			if sendAsStatus != 0 {
				w.WriteHeader(sendAsStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":    sendAsStatus,
						"message": http.StatusText(sendAsStatus),
						"errors": []map[string]any{{
							"message": http.StatusText(sendAsStatus),
							"domain":  "global",
							"reason":  "forbidden",
						}},
					},
				})
				return
			}
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
			headers := []map[string]any{
				{"name": "From", "value": from},
				{"name": "To", "value": "me@example.com"},
				{"name": "Subject", "value": "hello"},
				{"name": "Message-ID", "value": "<orig@example.com>"},
			}
			if replyTo != "" {
				headers = append(headers, map[string]any{"name": "Reply-To", "value": replyTo})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "m1",
				"threadId": "t1",
				"labelIds": []string{"INBOX"},
				"payload": map[string]any{
					"headers": headers,
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
	if allowSelf {
		args = append(args, "--allow-self")
	}
	result := executeWithGmailTestService(t, args, svc)
	if sendAsGets != 1 {
		t.Fatalf("GET settings/sendAs calls = %d, want 1", sendAsGets)
	}
	if sendAsStatus != 0 && !allowSelf && (sendCalls != 0 || followupCalls != 0) {
		t.Fatalf("send-as failure continued: send=%d followup=%d err=%v", sendCalls, followupCalls, result.err)
	}
	if result.err != nil {
		if sendAsStatus == 0 {
			t.Fatalf("Execute(%v): %v\nstderr:\n%s\nstdout:\n%s", args, result.err, result.stderr, result.stdout)
		}
		return sendCalls, gmailAutoReplySummary{}, result.err
	}

	var payload struct {
		AutoReply gmailAutoReplySummary `json:"autoReply"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, result.stdout)
	}
	return sendCalls, payload.AutoReply, nil
}
