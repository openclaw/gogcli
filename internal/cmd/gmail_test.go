package cmd

import (
	"os"
	"strings"
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

func TestFormatGmailDateInLocation_Timezones(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		timezone string
		expected string
	}{
		{
			name:     "UTC input to America/New_York output",
			input:    "Mon, 02 Jan 2006 15:04:05 +0000",
			timezone: "America/New_York",
			expected: "2006-01-02 10:04", // 15:04 UTC - 5 hours = 10:04 EST
		},
		{
			name:     "UTC input to Europe/London output",
			input:    "Mon, 02 Jan 2006 15:04:05 +0000",
			timezone: "Europe/London",
			expected: "2006-01-02 15:04", // UTC+0 in January (no DST)
		},
		{
			name:     "America/Los_Angeles input to UTC output",
			input:    "Mon, 02 Jan 2006 08:00:00 -0800",
			timezone: "UTC",
			expected: "2006-01-02 16:00", // 08:00 PST + 8 hours = 16:00 UTC
		},
		{
			name:     "Europe/Berlin input to Asia/Tokyo output",
			input:    "Mon, 02 Jan 2006 12:00:00 +0100",
			timezone: "Asia/Tokyo",
			expected: "2006-01-02 20:00", // 12:00 CET - 1 + 9 = 20:00 JST
		},
		{
			name:     "negative offset input to positive offset output crossing midnight",
			input:    "Mon, 02 Jan 2006 20:00:00 -0500",
			timezone: "Europe/Paris",
			expected: "2006-01-03 02:00", // 20:00 EST + 5 + 1 = 02:00 next day CET
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.timezone)
			if err != nil {
				t.Fatalf("failed to load timezone %s: %v", tt.timezone, err)
			}
			got := formatGmailDateInLocation(tt.input, loc)
			if got != tt.expected {
				t.Errorf("formatGmailDateInLocation(%q, %s) = %q, want %q",
					tt.input, tt.timezone, got, tt.expected)
			}
		})
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

func TestResolveOutputLocation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	tests := []struct {
		name        string
		timezone    string
		local       bool
		wantLocal   bool
		wantName    string
		wantErr     bool
		errContains string
	}{
		{
			name:      "local=true returns time.Local",
			timezone:  "America/New_York",
			local:     true,
			wantLocal: true,
		},
		{
			name:      "empty timezone returns time.Local",
			timezone:  "",
			local:     false,
			wantLocal: true,
		},
		{
			name:      "whitespace only returns time.Local",
			timezone:  "   ",
			local:     false,
			wantLocal: true,
		},
		{
			name:      "timezone=local (lowercase) returns time.Local",
			timezone:  "local",
			local:     false,
			wantLocal: true,
		},
		{
			name:      "timezone=LOCAL (uppercase) returns time.Local",
			timezone:  "LOCAL",
			local:     false,
			wantLocal: true,
		},
		{
			name:     "timezone=America/New_York returns that location",
			timezone: "America/New_York",
			local:    false,
			wantName: "America/New_York",
		},
		{
			name:     "timezone=UTC returns UTC",
			timezone: "UTC",
			local:    false,
			wantName: "UTC",
		},
		{
			name:        "invalid timezone returns error",
			timezone:    "Invalid/Zone",
			local:       false,
			wantErr:     true,
			errContains: "invalid timezone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveOutputLocation(tt.timezone, tt.local)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantLocal {
				if got != time.Local {
					t.Fatalf("expected time.Local, got %v", got)
				}
				return
			}

			if got.String() != tt.wantName {
				t.Fatalf("expected location %q, got %q", tt.wantName, got.String())
			}
		})
	}
}

func TestResolveOutputLocation_EnvVar(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Save and restore GOG_TIMEZONE
	orig := os.Getenv("GOG_TIMEZONE")
	defer os.Setenv("GOG_TIMEZONE", orig)

	envTZ := pickNonLocalTimezone(t)
	flagTZ := pickTimezoneExcluding(t, envTZ)

	// Test GOG_TIMEZONE takes effect when no flag provided
	os.Setenv("GOG_TIMEZONE", envTZ)
	loc, err := resolveOutputLocation("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc.String() != envTZ {
		t.Errorf("expected %s, got %s", envTZ, loc.String())
	}

	// Test flag takes precedence over env var
	loc, err = resolveOutputLocation(flagTZ, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc.String() != flagTZ {
		t.Errorf("expected %s (from flag), got %s", flagTZ, loc.String())
	}

	// Test --timezone local overrides env var
	loc, err = resolveOutputLocation("local", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc != time.Local {
		t.Errorf("expected time.Local, got %s", loc.String())
	}

	// Test --local overrides env var
	loc, err = resolveOutputLocation("", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loc != time.Local {
		t.Errorf("expected time.Local, got %s", loc.String())
	}

	// Test invalid env var returns error
	os.Setenv("GOG_TIMEZONE", "Invalid/Zone")
	_, err = resolveOutputLocation("", false)
	if err == nil {
		t.Fatal("expected error for invalid GOG_TIMEZONE")
	}
	if !strings.Contains(err.Error(), "GOG_TIMEZONE") {
		t.Errorf("error should mention GOG_TIMEZONE: %v", err)
	}
}

func TestGetConfiguredTimezone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Save and restore GOG_TIMEZONE
	orig := os.Getenv("GOG_TIMEZONE")
	defer os.Setenv("GOG_TIMEZONE", orig)
	os.Setenv("GOG_TIMEZONE", "")

	tests := []struct {
		name      string
		flag      string
		env       string
		wantLocal bool
		wantNil   bool
		wantZone  string
		wantErr   bool
	}{
		{
			name:     "flag takes precedence",
			flag:     "UTC",
			env:      "America/New_York",
			wantZone: "UTC",
		},
		{
			name:     "env var used when no flag",
			flag:     "",
			env:      "Europe/London",
			wantZone: "Europe/London",
		},
		{
			name:    "returns nil when nothing configured",
			flag:    "",
			env:     "",
			wantNil: true,
		},
		{
			name:    "invalid flag returns error",
			flag:    "Invalid/Zone",
			wantErr: true,
		},
		{
			name:    "invalid env returns error",
			flag:    "",
			env:     "Bad/Zone",
			wantErr: true,
		},
		{
			name:      "local flag returns time.Local",
			flag:      "local",
			env:       "UTC",
			wantLocal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GOG_TIMEZONE", tt.env)
			loc, err := getConfiguredTimezone(tt.flag)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantLocal {
				if loc != time.Local {
					t.Errorf("expected time.Local, got %s", loc.String())
				}
				return
			}

			if tt.wantNil {
				if loc != nil {
					t.Errorf("expected nil, got %s", loc.String())
				}
				return
			}

			if loc == nil {
				t.Fatal("expected non-nil location")
			}
			if loc.String() != tt.wantZone {
				t.Errorf("expected %s, got %s", tt.wantZone, loc.String())
			}
		})
	}
}
