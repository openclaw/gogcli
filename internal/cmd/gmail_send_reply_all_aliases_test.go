package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

// headerLine returns the first raw header line that starts with name.
func headerLine(raw, name string) string {
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, name+":") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// The original is addressed to the user's send-as alias and to bob. A
// reply-all must leave every address of the user out of the recipients,
// whichever command sends it.
func sendReplyAllViaCommand(t *testing.T, args ...string) string {
	t.Helper()
	return sendReplyAllWithHeaders(t, "alice@example.com", "me@example.com, alias@example.com, bob@example.com", "", args...)
}

func sendReplyAllWithHeaders(t *testing.T, from, to, cc string, args ...string) string {
	t.Helper()
	var sentRaw string
	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/settings/sendAs":
			_ = json.NewEncoder(w).Encode(map[string]any{"sendAs": []map[string]any{
				{"sendAsEmail": "me@example.com", "isPrimary": true, "verificationStatus": "accepted"},
				{"sendAsEmail": "alias@example.com", "verificationStatus": "accepted"},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/gmail/v1/users/me/messages/msg-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "msg-1", "threadId": "thread-1",
				"payload": map[string]any{"headers": []map[string]any{
					{"name": "Message-ID", "value": "<original@example.com>"},
					{"name": "From", "value": from},
					{"name": "To", "value": to},
					{"name": "Cc", "value": cc},
					{"name": "Subject", "value": "Plans"},
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

	result := executeWithGmailTestService(t, append([]string{"--json", "--account", "me@example.com"}, args...), svc)
	if result.err != nil {
		t.Fatalf("Execute %v: %v", args, result.err)
	}
	return sentRaw
}

func TestGmailSendReplyAllExcludesSendAsAliases(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "gmail reply-all (control)", args: []string{"gmail", "reply-all", "msg-1", "--body", "ok", "--no-quote"}},
		{name: "gmail send --reply-all", args: []string{"gmail", "send", "--reply-to-message-id", "msg-1", "--reply-all", "--body", "ok"}},
		{name: "gmail send --reply-all --from alias", args: []string{"gmail", "send", "--reply-to-message-id", "msg-1", "--reply-all", "--from", "alias@example.com", "--body", "ok"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := sendReplyAllViaCommand(t, tc.args...)
			recipients := headerLine(raw, "To") + " " + headerLine(raw, "Cc")
			if !strings.Contains(recipients, "alice@example.com") || !strings.Contains(recipients, "bob@example.com") {
				t.Fatalf("recipients %q, want alice and bob", recipients)
			}
			if strings.Contains(recipients, "alias@example.com") || strings.Contains(recipients, "me@example.com") {
				t.Fatalf("recipients %q include the sender's own address", recipients)
			}
		})
	}
}

// A message the user sent from an alias to their own primary address, copying
// bob: once both self addresses are left out, bob remains only in Cc. That is
// still a valid reply, as gmail reply-all already treats it.
func TestGmailSendReplyAllKeepsCcOnlyRecipients(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "gmail reply-all (control)", args: []string{"gmail", "reply-all", "msg-1", "--body", "ok", "--no-quote"}},
		{name: "gmail send --reply-all", args: []string{"gmail", "send", "--reply-to-message-id", "msg-1", "--reply-all", "--body", "ok"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := sendReplyAllWithHeaders(t, "alias@example.com", "me@example.com", "bob@example.com", tc.args...)
			if got := headerLine(raw, "Cc"); !strings.Contains(got, "bob@example.com") {
				t.Fatalf("Cc %q, want bob", got)
			}
			recipients := headerLine(raw, "To") + " " + headerLine(raw, "Cc")
			if strings.Contains(recipients, "alias@example.com") || strings.Contains(recipients, "me@example.com") {
				t.Fatalf("recipients %q include the sender's own address", recipients)
			}
		})
	}
}
