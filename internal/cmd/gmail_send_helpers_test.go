package cmd

import (
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestBuildSendBatches_NoTrack(t *testing.T) {
	batches := buildSendBatches(
		[]string{"to1@example.com", "to2@example.com"},
		[]string{"cc@example.com"},
		[]string{"bcc@example.com"},
		false,
		false,
	)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	batch := batches[0]
	if batch.TrackingRecipient != "to1@example.com" {
		t.Fatalf("unexpected tracking recipient: %q", batch.TrackingRecipient)
	}
	if len(batch.To) != 2 || len(batch.Cc) != 1 || len(batch.Bcc) != 1 {
		t.Fatalf("unexpected recipients: %#v", batch)
	}
}

func TestBuildSendBatches_TrackSplit(t *testing.T) {
	batches := buildSendBatches(
		[]string{"A@example.com", "a@example.com", "b@example.com"},
		nil,
		[]string{"c@example.com"},
		true,
		true,
	)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if batches[0].TrackingRecipient == "" || batches[1].TrackingRecipient == "" || batches[2].TrackingRecipient == "" {
		t.Fatalf("expected tracking recipients in batches: %#v", batches)
	}
}

func TestInjectTrackingPixelHTML(t *testing.T) {
	pixel := "<img src=\"/pixel\"/>"
	withBody := "<html><body>Hello</body></html>"
	out := injectTrackingPixelHTML(withBody, pixel)
	if !strings.Contains(out, pixel) || !strings.Contains(out, "</body>") {
		t.Fatalf("pixel not injected before body end: %q", out)
	}

	withHtml := "<html>Hello</html>"
	out = injectTrackingPixelHTML(withHtml, pixel)
	if !strings.Contains(out, pixel) || !strings.Contains(out, "</html>") {
		t.Fatalf("pixel not injected before html end: %q", out)
	}

	plain := "Hello"
	out = injectTrackingPixelHTML(plain, pixel)
	if out != plain+pixel {
		t.Fatalf("pixel not appended: %q", out)
	}
}

func TestBuildReplyAllRecipients_More(t *testing.T) {
	info := &replyInfo{
		FromAddr:    "from@example.com",
		ReplyToAddr: "reply@example.com",
		ToAddrs:     []string{"to@example.com", "me@example.com"},
		CcAddrs:     []string{"cc@example.com", "reply@example.com"},
	}
	to, cc := buildReplyAllRecipients(info, "me@example.com")
	if len(to) != 2 {
		t.Fatalf("expected 2 to recipients, got %v", to)
	}
	if to[0] != "reply@example.com" || to[1] != "to@example.com" {
		t.Fatalf("unexpected to recipients: %v", to)
	}
	if len(cc) != 1 || cc[0] != "cc@example.com" {
		t.Fatalf("unexpected cc recipients: %v", cc)
	}
}

func TestParseEmailAddresses_More(t *testing.T) {
	addrs := parseEmailAddresses("Alice <a@example.com>, b@example.com")
	if len(addrs) != 2 || addrs[0] != "a@example.com" || addrs[1] != "b@example.com" {
		t.Fatalf("unexpected addresses: %v", addrs)
	}

	fallback := parseEmailAddressesFallback("Name <X@Example.com>, y@example.com")
	if len(fallback) != 2 || fallback[0] != "x@example.com" || fallback[1] != "y@example.com" {
		t.Fatalf("unexpected fallback addresses: %v", fallback)
	}
}

func TestSelectLatestThreadMessage_More(t *testing.T) {
	msg1 := &gmail.Message{Id: "1", InternalDate: 0}
	msg2 := &gmail.Message{Id: "2", InternalDate: 10}
	msg3 := &gmail.Message{Id: "3", InternalDate: 5}
	selected := selectLatestThreadMessage([]*gmail.Message{msg1, msg2, msg3})
	if selected == nil || selected.Id != "2" {
		t.Fatalf("unexpected selected message: %#v", selected)
	}
}

func TestReplyInfoFromMessage(t *testing.T) {
	msg := &gmail.Message{
		ThreadId: "t1",
		Payload: &gmail.MessagePart{
			Headers: []*gmail.MessagePartHeader{
				{Name: "Message-ID", Value: "<id@example.com>"},
				{Name: "References", Value: "<ref@example.com>"},
				{Name: "From", Value: "From <from@example.com>"},
				{Name: "Reply-To", Value: "Reply <reply@example.com>"},
				{Name: "To", Value: "To <to@example.com>"},
				{Name: "Cc", Value: "cc@example.com"},
			},
		},
	}
	info := replyInfoFromMessage(msg, false)
	if info.ThreadID != "t1" {
		t.Fatalf("unexpected thread id: %q", info.ThreadID)
	}
	if info.InReplyTo != "<id@example.com>" {
		t.Fatalf("unexpected in-reply-to: %q", info.InReplyTo)
	}
	if !strings.Contains(info.References, "<id@example.com>") {
		t.Fatalf("expected references to include message id, got %q", info.References)
	}
	if len(info.ToAddrs) != 1 || info.ToAddrs[0] != "to@example.com" {
		t.Fatalf("unexpected to addrs: %v", info.ToAddrs)
	}
	if len(info.CcAddrs) != 1 || info.CcAddrs[0] != "cc@example.com" {
		t.Fatalf("unexpected cc addrs: %v", info.CcAddrs)
	}
}

func TestFilterOutSelfAndDeduplicate(t *testing.T) {
	filtered := filterOutSelf([]string{"a@example.com", "ME@EXAMPLE.COM"}, "me@example.com")
	if len(filtered) != 1 || filtered[0] != "a@example.com" {
		t.Fatalf("unexpected filtered list: %v", filtered)
	}

	deduped := deduplicateAddresses([]string{"A@example.com", "a@example.com", "b@example.com"})
	if len(deduped) != 2 {
		t.Fatalf("unexpected deduped list: %v", deduped)
	}
}
