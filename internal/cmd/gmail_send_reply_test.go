package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestReplyInfoFromMessage_More(t *testing.T) {
	msg := &gmail.Message{
		ThreadId: "thread1",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Message-ID", Value: "<m1>"},
				{Name: "References", Value: "<ref1>"},
				{Name: "From", Value: "Alice <a@example.com>"},
				{Name: "Reply-To", Value: "Reply <r@example.com>"},
				{Name: "To", Value: "b@example.com"},
				{Name: "Cc", Value: "c@example.com"},
			},
		},
	}
	info := replyInfoFromMessage(msg, false)
	if info.InReplyTo != "<m1>" {
		t.Fatalf("unexpected InReplyTo: %q", info.InReplyTo)
	}
	if info.References != "<ref1> <m1>" {
		t.Fatalf("unexpected References: %q", info.References)
	}
	if info.ReplyToAddr == "" || info.FromAddr == "" {
		t.Fatalf("missing reply info: %#v", info)
	}
	if len(info.ToAddrs) != 1 || info.ToAddrs[0] != "b@example.com" {
		t.Fatalf("unexpected ToAddrs: %#v", info.ToAddrs)
	}
	if len(info.CcAddrs) != 1 || info.CcAddrs[0] != "c@example.com" {
		t.Fatalf("unexpected CcAddrs: %#v", info.CcAddrs)
	}
}

func TestSelectLatestThreadMessage_Extra(t *testing.T) {
	msg1 := &gmail.Message{Id: "m1", InternalDate: 100}
	msg2 := &gmail.Message{Id: "m2", InternalDate: 200}
	msg3 := &gmail.Message{Id: "m3"}
	selected := selectLatestThreadMessage([]*gmail.Message{msg3, msg1, msg2})
	if selected == nil || selected.Id != "m2" {
		t.Fatalf("unexpected selected message: %#v", selected)
	}
}

func TestFetchReplyInfoFromThread(t *testing.T) {
	origNew := newGmailService
	t.Cleanup(func() { newGmailService = origNew })

	svc, cleanup := newGmailServiceForTest(t, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/gmail/v1")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/threads/t1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":           "m1",
						"internalDate": "200",
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "Message-ID", "value": "<m1>"},
								{"name": "From", "value": "a@example.com"},
							},
						},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})
	defer cleanup()
	newGmailService = func(context.Context, string) (*gmail.Service, error) { return svc, nil }

	info, err := fetchReplyInfo(context.Background(), svc, "", "t1", false)
	if err != nil {
		t.Fatalf("fetchReplyInfo: %v", err)
	}
	if info.ThreadID != "t1" {
		t.Fatalf("expected thread id t1, got %q", info.ThreadID)
	}
	if info.InReplyTo != "<m1>" {
		t.Fatalf("unexpected InReplyTo: %q", info.InReplyTo)
	}
}

func TestWriteSendResults_JSON(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := writeSendResults(ctx, u, "from@example.com", []sendResult{
			{MessageID: "m1", ThreadID: "t1", TrackingID: "trk"},
		}); err != nil {
			t.Fatalf("writeSendResults: %v", err)
		}
	})
	var payload map[string]string
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if payload["messageId"] != "m1" || payload["threadId"] != "t1" || payload["tracking_id"] != "trk" {
		t.Fatalf("unexpected json payload: %#v", payload)
	}
}
