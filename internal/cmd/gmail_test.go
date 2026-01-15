package cmd

import (
	"testing"
	"time"

	"google.golang.org/api/gmail/v1"
)

func TestHeaderValue(t *testing.T) {
	p := &gmail.MessagePart{
		Headers: []*gmail.MessagePartHeader{
			{Name: "From", Value: "a@example.com"},
			{Name: "Subject", Value: "Hello"},
		},
	}
	if got := headerValue(p, "from"); got != "a@example.com" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := headerValue(p, "subject"); got != "Hello" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := headerValue(p, "date"); got != "" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestBestUnsubscribeLink(t *testing.T) {
	p := &gmail.MessagePart{
		Headers: []*gmail.MessagePartHeader{
			{Name: "List-Unsubscribe", Value: "<mailto:unsubscribe@example.com>, <https://example.com/unsub?id=1>"},
		},
	}
	if got := bestUnsubscribeLink(p); got != "https://example.com/unsub?id=1" {
		t.Fatalf("unexpected: %q", got)
	}
	p.Headers[0].Value = "<mailto:unsubscribe@example.com>, https://example.com/unsub"
	if got := bestUnsubscribeLink(p); got != "https://example.com/unsub" {
		t.Fatalf("unexpected: %q", got)
	}
	p.Headers[0].Value = "http://example.com/unsub, https://example.com/unsub-secure"
	if got := bestUnsubscribeLink(p); got != "https://example.com/unsub-secure" {
		t.Fatalf("unexpected: %q", got)
	}
	p.Headers[0].Value = "<mailto:unsubscribe@example.com>"
	if got := bestUnsubscribeLink(p); got != "mailto:unsubscribe@example.com" {
		t.Fatalf("unexpected: %q", got)
	}
	p.Headers[0].Value = "not a link"
	if got := bestUnsubscribeLink(p); got != "" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestSanitizeTab(t *testing.T) {
	if got := sanitizeTab("a\tb"); got != "a b" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormatGmailDate(t *testing.T) {
	loc := time.UTC
	got := formatGmailDateInLocation("Mon, 02 Jan 2006 15:04:05 -0700", loc)
	if got != "2006-01-02 22:04" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatGmailDateInLocation("not a date", loc); got != "not a date" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFirstMessage(t *testing.T) {
	if firstMessage(nil) != nil {
		t.Fatalf("expected nil")
	}
	if firstMessage(&gmail.Thread{}) != nil {
		t.Fatalf("expected nil")
	}
	m := &gmail.Message{Id: "m1"}
	if got := firstMessage(&gmail.Thread{Messages: []*gmail.Message{m}}); got == nil || got.Id != "m1" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestLastMessage(t *testing.T) {
	if lastMessage(nil) != nil {
		t.Fatalf("expected nil")
	}
	if lastMessage(&gmail.Thread{}) != nil {
		t.Fatalf("expected nil")
	}
	m1 := &gmail.Message{Id: "m1"}
	m2 := &gmail.Message{Id: "m2"}
	if got := lastMessage(&gmail.Thread{Messages: []*gmail.Message{m1, m2}}); got == nil || got.Id != "m2" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestMessageDateMillis(t *testing.T) {
	msg := &gmail.Message{InternalDate: 1234}
	if got := messageDateMillis(msg); got != 1234 {
		t.Fatalf("unexpected internal date: %d", got)
	}

	msg = &gmail.Message{Payload: &gmail.MessagePart{
		Headers: []*gmail.MessagePartHeader{
			{Name: "Date", Value: "Mon, 02 Jan 2006 15:04:05 -0700"},
		},
	}}
	if got := messageDateMillis(msg); got == 0 {
		t.Fatalf("expected parsed date")
	}

	msg = &gmail.Message{Payload: &gmail.MessagePart{
		Headers: []*gmail.MessagePartHeader{
			{Name: "Date", Value: "not a date"},
		},
	}}
	if got := messageDateMillis(msg); got != 0 {
		t.Fatalf("expected zero for invalid date, got %d", got)
	}

	if got := messageDateMillis(&gmail.Message{}); got != 0 {
		t.Fatalf("expected zero for missing payload, got %d", got)
	}
}

func TestMessageByDate(t *testing.T) {
	m1 := &gmail.Message{Id: "m1", InternalDate: 100}
	m2 := &gmail.Message{Id: "m2", InternalDate: 200}
	m3 := &gmail.Message{Id: "m3", InternalDate: 150}
	thread := &gmail.Thread{Messages: []*gmail.Message{m1, m2, m3}}

	if got := messageByDate(thread, false); got == nil || got.Id != "m2" {
		t.Fatalf("unexpected newest: %#v", got)
	}
	if got := messageByDate(thread, true); got == nil || got.Id != "m1" {
		t.Fatalf("unexpected oldest: %#v", got)
	}
	if got := newestMessageByDate(thread); got == nil || got.Id != "m2" {
		t.Fatalf("unexpected newest wrapper: %#v", got)
	}
	if got := oldestMessageByDate(thread); got == nil || got.Id != "m1" {
		t.Fatalf("unexpected oldest wrapper: %#v", got)
	}

	noDates := &gmail.Thread{Messages: []*gmail.Message{{Id: "a"}, {Id: "b"}}}
	if got := messageByDate(noDates, false); got == nil || got.Id != "b" {
		t.Fatalf("unexpected fallback newest: %#v", got)
	}
	if got := messageByDate(noDates, true); got == nil || got.Id != "a" {
		t.Fatalf("unexpected fallback oldest: %#v", got)
	}
}
