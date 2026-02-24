package cmd

import (
	"encoding/base64"
	"testing"
)

func TestNormalizeGmailThreadID(t *testing.T) {
	t.Parallel()

	threadID := "18c0abc123def456"
	tests := []struct {
		in   string
		want string
	}{
		{in: "  " + threadID + "  ", want: threadID},
		{in: "https://mail.google.com/mail/u/0/#inbox/" + threadID, want: threadID},
		{in: "mail.google.com/mail/u/0/#all/" + threadID, want: threadID},
		{in: "https://mail.google.com/mail/u/0/?ui=2&th=" + threadID + "&view=pt", want: threadID},
		{in: "https://example.com/not-gmail/" + threadID, want: "https://example.com/not-gmail/" + threadID},
	}

	for _, tt := range tests {
		if got := normalizeGmailThreadID(tt.in); got != tt.want {
			t.Fatalf("normalizeGmailThreadID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeGmailMessageID(t *testing.T) {
	t.Parallel()

	msgID := "18c0abc123def457"
	tests := []struct {
		in   string
		want string
	}{
		{in: msgID, want: msgID},
		{in: "https://mail.google.com/mail/u/0/?ui=2&message_id=" + msgID + "&view=pt", want: msgID},
		{in: "https://mail.google.com/mail/u/0/?ui=2&permmsgid=msg-f:" + msgID + "&view=pt", want: msgID},
	}

	for _, tt := range tests {
		if got := normalizeGmailMessageID(tt.in); got != tt.want {
			t.Fatalf("normalizeGmailMessageID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeCalendarEventID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: " ev123 ", want: "ev123"},
		{in: "https://calendar.google.com/calendar/u/0/r/eventedit/ev456", want: "ev456"},
	}

	for _, tt := range tests {
		if got := normalizeCalendarEventID(tt.in); got != tt.want {
			t.Fatalf("normalizeCalendarEventID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}

	decoded := "ev789 primary"
	eid := base64.RawURLEncoding.EncodeToString([]byte(decoded))
	in := "https://calendar.google.com/calendar/event?eid=" + eid
	if got := normalizeCalendarEventID(in); got != "ev789" {
		t.Fatalf("normalizeCalendarEventID(eid) = %q, want %q (decoded=%q)", got, "ev789", decoded)
	}
}
