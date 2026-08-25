package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

func TestSplitCSV(t *testing.T) {
	if got := splitCSV(""); got != nil {
		t.Fatalf("unexpected: %#v", got)
	}
	got := splitCSV(" a@b.com, c@d.com ,,")
	if len(got) != 2 || got[0] != "a@b.com" || got[1] != "c@d.com" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestBuildEventDateTime(t *testing.T) {
	allDay := buildEventDateTime("2025-01-01", true)
	if allDay.Date != "2025-01-01" || allDay.DateTime != "" {
		t.Fatalf("unexpected: %#v", allDay)
	}
	timed := buildEventDateTime("2025-01-01T10:00:00Z", false)
	if timed.DateTime != "2025-01-01T10:00:00Z" || timed.Date != "" {
		t.Fatalf("unexpected: %#v", timed)
	}
}

func TestIsAllDayEvent(t *testing.T) {
	if isAllDayEvent(nil) {
		t.Fatalf("expected false")
	}
	if !isAllDayEvent(&calendar.Event{Start: &calendar.EventDateTime{Date: "2025-01-01"}}) {
		t.Fatalf("expected true")
	}
}

func TestBuildColorId(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"1", "1", false},
		{"11", "11", false},
		{"0", "", true},
		{"12", "", true},
		{"", "", false},
		{"abc", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateColorId(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateColorId(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateColorId(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateVisibility(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"default", "default", false},
		{"public", "public", false},
		{"private", "private", false},
		{"confidential", "confidential", false},
		{"DEFAULT", "default", false},
		{"", "", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateVisibility(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVisibility(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateVisibility(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateTransparency(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"opaque", "opaque", false},
		{"transparent", "transparent", false},
		{"busy", "opaque", false},
		{"free", "transparent", false},
		{"OPAQUE", "opaque", false},
		{"", "", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateTransparency(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTransparency(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateTransparency(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateSendUpdates(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"all", "all", false},
		{"externalOnly", "externalOnly", false},
		{"none", "none", false},
		{"ALL", "all", false},
		{"", "", false},
		{"invalid", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := validateSendUpdates(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSendUpdates(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateSendUpdates(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseAttendee(t *testing.T) {
	tests := []struct {
		input    string
		email    string
		optional bool
		resource bool
		comment  string
		isNil    bool
	}{
		{"alice@example.com", "alice@example.com", false, false, "", false},
		{"bob@example.com;optional", "bob@example.com", true, false, "", false},
		{"carol@example.com;comment=FYI only", "carol@example.com", false, false, "FYI only", false},
		{"dave@example.com;OPTIONAL;comment=Hi", "dave@example.com", true, false, "Hi", false},
		{"room@example.com;RESOURCE", "room@example.com", false, true, "", false},
		{"room@example.com;resource;optional;comment=Project room", "room@example.com", true, true, "Project room", false},
		{";optional", "", false, false, "", true},
		{"", "", false, false, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseAttendee(tt.input)
			if tt.isNil {
				if got != nil {
					t.Fatalf("expected nil attendee, got %#v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected attendee, got nil")
				return
			}
			if got.Email != tt.email || got.Optional != tt.optional || got.Resource != tt.resource || got.Comment != tt.comment {
				t.Fatalf("unexpected attendee: %#v", got)
			}
		})
	}
}

func TestRecurrenceUntil(t *testing.T) {
	got, err := recurrenceUntil("2025-01-10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "20250109" {
		t.Fatalf("unexpected until date: %s", got)
	}

	got, err = recurrenceUntil("2025-01-10T12:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "20250110T115959Z" {
		t.Fatalf("unexpected until datetime: %s", got)
	}
}

func TestTruncateRecurrence(t *testing.T) {
	rules := []string{
		"RRULE:FREQ=WEEKLY;COUNT=10",
		"EXDATE:20250101T100000Z",
	}
	truncated, err := truncateRecurrence(rules, "2025-01-10T12:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(truncated) != 2 {
		t.Fatalf("unexpected rule count: %#v", truncated)
	}
	if truncated[0] != "RRULE:FREQ=WEEKLY;UNTIL=20250110T115959Z" {
		t.Fatalf("unexpected RRULE: %s", truncated[0])
	}
	if truncated[1] != "EXDATE:20250101T100000Z" {
		t.Fatalf("unexpected EXDATE: %s", truncated[1])
	}
}

func TestBuildRecurrence(t *testing.T) {
	if got := buildRecurrence(nil); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
	if got := buildRecurrence([]string{}); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
	if got := buildRecurrence([]string{"", "  "}); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
	got := buildRecurrence([]string{"RRULE:FREQ=DAILY", "", "EXDATE:20250101"})
	if len(got) != 2 || got[0] != "RRULE:FREQ=DAILY" || got[1] != "EXDATE:20250101" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"30", 30, false},
		{"0", 0, false},
		{"40320", 40320, false},
		{"30m", 30, false},
		{"1h", 60, false},
		{"2h", 120, false},
		{"1d", 1440, false},
		{"3d", 4320, false},
		{"1w", 10080, false},
		{"4w", 40320, false},
		{"1H", 60, false},
		{"1D", 1440, false},
		{"1M", 1, false},
		{"30M", 30, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-1", 0, true},
		{"40321", 0, true},
		{"5w", 0, true},
		{"1x", 0, true},
	}

	for _, tc := range tests {
		got, err := parseDuration(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseDuration(%q): expected error, got %d", tc.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDuration(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseReminder(t *testing.T) {
	tests := []struct {
		input       string
		wantMethod  string
		wantMinutes int64
		wantErr     bool
	}{
		{"popup:30m", "popup", 30, false},
		{"email:1h", "email", 60, false},
		{"POPUP:1d", "popup", 1440, false},
		{"EMAIL:3d", "email", 4320, false},
		{"popup:60", "popup", 60, false},
		{"", "", 0, true},
		{"popup", "", 0, true},
		{"sms:30m", "", 0, true},
		{"popup:abc", "", 0, true},
		{"popup:-1", "", 0, true},
		{"popup:50000", "", 0, true},
	}

	for _, tc := range tests {
		method, minutes, err := parseReminder(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseReminder(%q): expected error, got method=%q minutes=%d", tc.input, method, minutes)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseReminder(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if method != tc.wantMethod || minutes != tc.wantMinutes {
			t.Errorf("parseReminder(%q) = (%q, %d), want (%q, %d)", tc.input, method, minutes, tc.wantMethod, tc.wantMinutes)
		}
	}
}

func TestBuildReminders(t *testing.T) {
	got, err := buildReminders(nil, false)
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil), got (%#v, %v)", got, err)
	}

	got, err = buildReminders([]string{}, false)
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil), got (%#v, %v)", got, err)
	}

	got, err = buildReminders([]string{"", "  "}, false)
	if err != nil || got != nil {
		t.Fatalf("expected (nil, nil), got (%#v, %v)", got, err)
	}

	got, err = buildReminders([]string{"popup:30m", "email:1d"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.UseDefault || len(got.Overrides) != 2 {
		t.Fatalf("unexpected: %#v", got)
	}
	if !hasStringValue(got.ForceSendFields, "UseDefault") {
		t.Fatalf("expected UseDefault to be force-sent, got %#v", got.ForceSendFields)
	}
	if got.Overrides[0].Method != "popup" || got.Overrides[0].Minutes != 30 {
		t.Fatalf("unexpected override[0]: %#v", got.Overrides[0])
	}
	if got.Overrides[1].Method != "email" || got.Overrides[1].Minutes != 1440 {
		t.Fatalf("unexpected override[1]: %#v", got.Overrides[1])
	}

	got, err = buildReminders([]string{"popup:0m"}, false)
	if err != nil {
		t.Fatalf("unexpected error for 0-minute reminder: %v", err)
	}
	if got == nil || len(got.Overrides) != 1 {
		t.Fatalf("unexpected 0-minute reminders payload: %#v", got)
	}
	if got.Overrides[0].Method != "popup" || got.Overrides[0].Minutes != 0 {
		t.Fatalf("unexpected 0-minute override: %#v", got.Overrides[0])
	}
	if !hasStringValue(got.Overrides[0].ForceSendFields, "Minutes") {
		t.Fatalf("expected Minutes to be force-sent for 0-minute reminder, got %#v", got.Overrides[0].ForceSendFields)
	}

	_, err = buildReminders([]string{"popup:1m", "popup:2m", "popup:3m", "popup:4m", "popup:5m", "popup:6m"}, false)
	if err == nil {
		t.Fatalf("expected error for >5 reminders")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}

	_, err = buildReminders([]string{"popup:30m", "invalid"}, false)
	if err == nil {
		t.Fatalf("expected error for invalid reminder")
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", got, err)
	}
}

func TestBuildRemindersDisabled(t *testing.T) {
	got, err := buildReminders(nil, true)
	if err != nil {
		t.Fatalf("build disabled reminders: %v", err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal disabled reminders: %v", err)
	}
	if string(encoded) != `{"overrides":[],"useDefault":false}` {
		t.Fatalf("disabled reminders payload = %s", encoded)
	}

	_, err = buildReminders([]string{"popup:30m"}, true)
	if err == nil || !strings.Contains(err.Error(), "--reminder and --no-reminders") {
		t.Fatalf("expected mutually exclusive reminder flags, got %v", err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("expected usage exit code 2, got %d", got)
	}
}

func hasStringValue(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func TestWrapEventWithDaysPrefersEventTimezone(t *testing.T) {
	t.Parallel()

	event := &calendar.Event{
		Start: &calendar.EventDateTime{
			DateTime: "2026-07-07T07:30:00+08:00",
			TimeZone: "Asia/Seoul",
		},
		End: &calendar.EventDateTime{
			DateTime: "2026-07-07T08:30:00+08:00",
			TimeZone: "Asia/Seoul",
		},
	}

	wrapped := wrapEventWithDaysWithTimezone(event, "Asia/Hong_Kong", nil)
	if wrapped == nil {
		t.Fatal("expected wrapped event")
	}
	if wrapped.Timezone != "Asia/Seoul" {
		t.Fatalf("Timezone = %q, want Asia/Seoul", wrapped.Timezone)
	}
	if wrapped.EventTimezone != "" {
		t.Fatalf("EventTimezone = %q, want empty when it matches display timezone", wrapped.EventTimezone)
	}
	if wrapped.StartLocal != "2026-07-07T08:30:00+09:00" {
		t.Fatalf("StartLocal = %q, want Seoul local time", wrapped.StartLocal)
	}
	if wrapped.EndLocal != "2026-07-07T09:30:00+09:00" {
		t.Fatalf("EndLocal = %q, want Seoul local time", wrapped.EndLocal)
	}
}

func TestWrapEventWithDaysFallsBackFromInvalidEventTimezone(t *testing.T) {
	t.Parallel()

	event := &calendar.Event{
		Start: &calendar.EventDateTime{
			DateTime: "2026-07-07T07:30:00+08:00",
			TimeZone: "Not/AZone",
		},
		End: &calendar.EventDateTime{
			DateTime: "2026-07-07T08:30:00+08:00",
			TimeZone: "Not/AZone",
		},
	}

	calendarLoc, err := time.LoadLocation("Asia/Hong_Kong")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	wrapped := wrapEventWithDaysWithTimezone(event, "Asia/Hong_Kong", calendarLoc)
	if wrapped.Timezone != "Asia/Hong_Kong" || wrapped.EventTimezone != "Not/AZone" {
		t.Fatalf("unexpected timezone fields: timezone=%q eventTimezone=%q", wrapped.Timezone, wrapped.EventTimezone)
	}
	if wrapped.StartLocal != "2026-07-07T07:30:00+08:00" || wrapped.EndLocal != "2026-07-07T08:30:00+08:00" {
		t.Fatalf("unexpected calendar-local fields: start=%q end=%q", wrapped.StartLocal, wrapped.EndLocal)
	}
}

func TestWrapEventWithDaysResolvesEndpointTimezonesIndependently(t *testing.T) {
	t.Parallel()

	event := &calendar.Event{
		Start: &calendar.EventDateTime{
			DateTime: "2026-07-07T10:00:00+02:00",
			TimeZone: "Europe/Rome",
		},
		End: &calendar.EventDateTime{
			DateTime: "2026-07-07T12:00:00-04:00",
			TimeZone: "America/New_York",
		},
	}

	wrapped := wrapEventWithDaysWithTimezone(event, "UTC", time.UTC)
	if wrapped.StartLocal != "2026-07-07T10:00:00+02:00" || wrapped.EndLocal != "2026-07-07T12:00:00-04:00" {
		t.Fatalf("unexpected endpoint-local fields: start=%q end=%q", wrapped.StartLocal, wrapped.EndLocal)
	}
	if wrapped.StartDayOfWeek != "Tuesday" || wrapped.EndDayOfWeek != "Tuesday" {
		t.Fatalf("unexpected endpoint-local weekdays: start=%q end=%q", wrapped.StartDayOfWeek, wrapped.EndDayOfWeek)
	}
}

func TestWrapEventWithDaysExplicitTimezoneOverrideWins(t *testing.T) {
	t.Parallel()

	event := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2026-07-08T02:00:00Z", TimeZone: "Asia/Seoul"},
		End:   &calendar.EventDateTime{DateTime: "2026-07-08T03:00:00Z", TimeZone: "Asia/Seoul"},
	}
	override, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	wrapped := wrapEventWithDaysWithTimezoneOverride(event, "America/New_York", override, true)
	if wrapped.Timezone != "America/New_York" || wrapped.EventTimezone != "Asia/Seoul" {
		t.Fatalf("unexpected timezone fields: timezone=%q eventTimezone=%q", wrapped.Timezone, wrapped.EventTimezone)
	}
	if wrapped.StartLocal != "2026-07-07T22:00:00-04:00" || wrapped.EndLocal != "2026-07-07T23:00:00-04:00" {
		t.Fatalf("unexpected override-local fields: start=%q end=%q", wrapped.StartLocal, wrapped.EndLocal)
	}
	if wrapped.StartDayOfWeek != "Tuesday" || wrapped.EndDayOfWeek != "Tuesday" {
		t.Fatalf("unexpected override-local weekdays: start=%q end=%q", wrapped.StartDayOfWeek, wrapped.EndDayOfWeek)
	}
}
