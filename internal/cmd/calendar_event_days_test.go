package cmd

import (
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

func TestEventDaysOfWeek_DateTime(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z"},
	}
	start, end := eventDaysOfWeek(ev)
	if start != "Wednesday" || end != "Wednesday" {
		t.Fatalf("expected Wednesday/Wednesday, got %q/%q", start, end)
	}
}

func TestEventDaysOfWeek_DateOnly(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{Date: "2025-01-02"},
		End:   &calendar.EventDateTime{Date: "2025-01-03"},
	}
	start, end := eventDaysOfWeek(ev)
	if start != "Thursday" || end != "Friday" {
		t.Fatalf("expected Thursday/Friday, got %q/%q", start, end)
	}
}

func TestWrapEventsWithDays(t *testing.T) {
	events := []*calendar.Event{
		{Start: &calendar.EventDateTime{Date: "2025-01-02"}, End: &calendar.EventDateTime{Date: "2025-01-02"}},
	}
	wrapped := wrapEventsWithDays(events)
	if len(wrapped) != 1 {
		t.Fatalf("expected 1 wrapped event, got %d", len(wrapped))
	}
	if wrapped[0].StartDayOfWeek != "Thursday" {
		t.Fatalf("unexpected start day: %q", wrapped[0].StartDayOfWeek)
	}
	if wrapped[0].StartLocal != "2025-01-02" {
		t.Fatalf("unexpected start local: %q", wrapped[0].StartLocal)
	}
}

// Tests for parseEventTime function
func TestParseEventTime_RFC3339(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		tz       string
		wantOk   bool
		wantYear int
		wantDay  time.Weekday
	}{
		{
			name:     "RFC3339 UTC",
			value:    "2025-01-01T10:00:00Z",
			tz:       "",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Wednesday,
		},
		{
			name:     "RFC3339 with offset",
			value:    "2025-01-01T10:00:00-05:00",
			tz:       "",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Wednesday,
		},
		{
			name:     "RFC3339Nano",
			value:    "2025-01-01T10:00:00.123456789Z",
			tz:       "",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Wednesday,
		},
		{
			name:     "RFC3339 with timezone override",
			value:    "2025-01-01T10:00:00Z",
			tz:       "America/New_York",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Wednesday,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseEventTime(tc.value, tc.tz)
			if ok != tc.wantOk {
				t.Fatalf("parseEventTime(%q, %q) ok = %v, want %v", tc.value, tc.tz, ok, tc.wantOk)
			}
			if !ok {
				return
			}
			if got.Year() != tc.wantYear {
				t.Fatalf("parseEventTime(%q, %q).Year() = %d, want %d", tc.value, tc.tz, got.Year(), tc.wantYear)
			}
			if got.Weekday() != tc.wantDay {
				t.Fatalf("parseEventTime(%q, %q).Weekday() = %v, want %v", tc.value, tc.tz, got.Weekday(), tc.wantDay)
			}
		})
	}
}

func TestParseEventTime_LocalFormat(t *testing.T) {
	// Test the "2006-01-02T15:04:05" format (without timezone suffix)
	tests := []struct {
		name     string
		value    string
		tz       string
		wantOk   bool
		wantYear int
	}{
		{
			name:     "local format with valid timezone",
			value:    "2025-06-15T14:30:00",
			tz:       "America/New_York",
			wantOk:   true,
			wantYear: 2025,
		},
		{
			name:     "local format with UTC timezone",
			value:    "2025-06-15T14:30:00",
			tz:       "UTC",
			wantOk:   true,
			wantYear: 2025,
		},
		{
			name:   "local format without timezone fails",
			value:  "2025-06-15T14:30:00",
			tz:     "",
			wantOk: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseEventTime(tc.value, tc.tz)
			if ok != tc.wantOk {
				t.Fatalf("parseEventTime(%q, %q) ok = %v, want %v", tc.value, tc.tz, ok, tc.wantOk)
			}
			if ok && got.Year() != tc.wantYear {
				t.Fatalf("parseEventTime(%q, %q).Year() = %d, want %d", tc.value, tc.tz, got.Year(), tc.wantYear)
			}
		})
	}
}

func TestParseEventTime_Empty(t *testing.T) {
	tests := []struct {
		name  string
		value string
		tz    string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"empty with timezone", "", "America/New_York"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseEventTime(tc.value, tc.tz)
			if ok {
				t.Fatalf("parseEventTime(%q, %q) should return false for empty value", tc.value, tc.tz)
			}
		})
	}
}

func TestParseEventTime_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
		tz    string
	}{
		{"invalid format", "not-a-date", ""},
		{"invalid RFC3339", "2025-01-01T10:00:00", ""}, // missing timezone designator
		{"partial date", "2025-01-01", ""},
		{"garbage", "xyz123", "America/New_York"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseEventTime(tc.value, tc.tz)
			if ok {
				t.Fatalf("parseEventTime(%q, %q) should return false for invalid value", tc.value, tc.tz)
			}
		})
	}
}

// Tests for parseEventDate function
func TestParseEventDate_Valid(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		tz       string
		wantOk   bool
		wantYear int
		wantDay  time.Weekday
	}{
		{
			name:     "date without timezone",
			value:    "2025-01-02",
			tz:       "",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Thursday,
		},
		{
			name:     "date with UTC timezone",
			value:    "2025-01-02",
			tz:       "UTC",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Thursday,
		},
		{
			name:     "date with America/New_York timezone",
			value:    "2025-01-02",
			tz:       "America/New_York",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Thursday,
		},
		{
			name:     "date with Asia/Tokyo timezone",
			value:    "2025-07-15",
			tz:       "Asia/Tokyo",
			wantOk:   true,
			wantYear: 2025,
			wantDay:  time.Tuesday,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseEventDate(tc.value, tc.tz)
			if ok != tc.wantOk {
				t.Fatalf("parseEventDate(%q, %q) ok = %v, want %v", tc.value, tc.tz, ok, tc.wantOk)
			}
			if !ok {
				return
			}
			if got.Year() != tc.wantYear {
				t.Fatalf("parseEventDate(%q, %q).Year() = %d, want %d", tc.value, tc.tz, got.Year(), tc.wantYear)
			}
			if got.Weekday() != tc.wantDay {
				t.Fatalf("parseEventDate(%q, %q).Weekday() = %v, want %v", tc.value, tc.tz, got.Weekday(), tc.wantDay)
			}
		})
	}
}

func TestParseEventDate_Empty(t *testing.T) {
	tests := []struct {
		name  string
		value string
		tz    string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"empty with timezone", "", "America/New_York"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseEventDate(tc.value, tc.tz)
			if ok {
				t.Fatalf("parseEventDate(%q, %q) should return false for empty value", tc.value, tc.tz)
			}
		})
	}
}

func TestParseEventDate_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
		tz    string
	}{
		{"datetime format", "2025-01-01T10:00:00Z", ""},
		{"invalid format", "01-02-2025", ""},
		{"garbage", "not-a-date", ""},
		{"partial", "2025-01", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := parseEventDate(tc.value, tc.tz)
			if ok {
				t.Fatalf("parseEventDate(%q, %q) should return false for invalid value", tc.value, tc.tz)
			}
		})
	}
}

// Tests for loadEventLocation function
func TestLoadEventLocation(t *testing.T) {
	tests := []struct {
		name   string
		tz     string
		wantOk bool
	}{
		{"valid UTC", "UTC", true},
		{"valid America/New_York", "America/New_York", true},
		{"valid Europe/London", "Europe/London", true},
		{"valid Asia/Tokyo", "Asia/Tokyo", true},
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"invalid timezone", "Invalid/Timezone", false},
		{"garbage", "not-a-timezone", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, ok := loadEventLocation(tc.tz)
			if ok != tc.wantOk {
				t.Fatalf("loadEventLocation(%q) ok = %v, want %v", tc.tz, ok, tc.wantOk)
			}
			if ok && loc == nil {
				t.Fatalf("loadEventLocation(%q) returned nil location with ok=true", tc.tz)
			}
		})
	}
}

// Tests for dayOfWeekFromEventDateTime function
func TestDayOfWeekFromEventDateTime(t *testing.T) {
	tests := []struct {
		name string
		dt   *calendar.EventDateTime
		want string
	}{
		{
			name: "nil input",
			dt:   nil,
			want: "",
		},
		{
			name: "datetime only",
			dt:   &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
			want: "Wednesday",
		},
		{
			name: "date only",
			dt:   &calendar.EventDateTime{Date: "2025-01-02"},
			want: "Thursday",
		},
		{
			name: "datetime with timezone",
			dt:   &calendar.EventDateTime{DateTime: "2025-01-03T10:00:00Z", TimeZone: "America/New_York"},
			want: "Friday",
		},
		{
			name: "date with timezone",
			dt:   &calendar.EventDateTime{Date: "2025-01-04", TimeZone: "UTC"},
			want: "Saturday",
		},
		{
			name: "both datetime and date (datetime takes precedence)",
			dt:   &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z", Date: "2025-01-02"},
			want: "Wednesday", // DateTime should take precedence
		},
		{
			name: "empty datetime and date",
			dt:   &calendar.EventDateTime{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dayOfWeekFromEventDateTime(tc.dt)
			if got != tc.want {
				t.Fatalf("dayOfWeekFromEventDateTime(%v) = %q, want %q", tc.dt, got, tc.want)
			}
		})
	}
}

// Tests for eventDaysOfWeek function
func TestEventDaysOfWeek_NilEvent(t *testing.T) {
	start, end := eventDaysOfWeek(nil)
	if start != "" || end != "" {
		t.Fatalf("eventDaysOfWeek(nil) = (%q, %q), want (\"\", \"\")", start, end)
	}
}

func TestEventDaysOfWeek_NilStartEnd(t *testing.T) {
	ev := &calendar.Event{}
	start, end := eventDaysOfWeek(ev)
	if start != "" || end != "" {
		t.Fatalf("eventDaysOfWeek with nil start/end = (%q, %q), want (\"\", \"\")", start, end)
	}
}

func TestEventDaysOfWeek_MixedDateTimeAndDate(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-06T10:00:00Z"},
		End:   &calendar.EventDateTime{Date: "2025-01-07"},
	}
	start, end := eventDaysOfWeek(ev)
	if start != "Monday" {
		t.Fatalf("expected start Monday, got %q", start)
	}
	if end != "Tuesday" {
		t.Fatalf("expected end Tuesday, got %q", end)
	}
}

// Tests for wrapEventsWithDays function
func TestWrapEventsWithDays_Empty(t *testing.T) {
	wrapped := wrapEventsWithDays([]*calendar.Event{})
	if len(wrapped) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(wrapped))
	}
}

func TestWrapEventsWithDays_Nil(t *testing.T) {
	wrapped := wrapEventsWithDays(nil)
	if len(wrapped) != 0 {
		t.Fatalf("expected empty slice for nil input, got %d items", len(wrapped))
	}
}

func TestWrapEventsWithDays_Multiple(t *testing.T) {
	events := []*calendar.Event{
		{Start: &calendar.EventDateTime{Date: "2025-01-06"}, End: &calendar.EventDateTime{Date: "2025-01-06"}},                             // Monday
		{Start: &calendar.EventDateTime{Date: "2025-01-07"}, End: &calendar.EventDateTime{Date: "2025-01-07"}},                             // Tuesday
		{Start: &calendar.EventDateTime{DateTime: "2025-01-08T10:00:00Z"}, End: &calendar.EventDateTime{DateTime: "2025-01-08T11:00:00Z"}}, // Wednesday
	}
	wrapped := wrapEventsWithDays(events)
	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped events, got %d", len(wrapped))
	}
	expectedDays := []string{"Monday", "Tuesday", "Wednesday"}
	for i, w := range wrapped {
		if w.StartDayOfWeek != expectedDays[i] {
			t.Fatalf("event %d: expected StartDayOfWeek %q, got %q", i, expectedDays[i], w.StartDayOfWeek)
		}
	}
}

// Tests for wrapEventWithDaysWithTimezone function
func TestWrapEventWithDaysWithTimezone_NilEvent(t *testing.T) {
	wrapped := wrapEventWithDaysWithTimezone(nil, "", nil)
	if wrapped != nil {
		t.Fatal("expected nil for nil event input")
	}
}

func TestWrapEventWithDaysWithTimezone_WithCalendarTimezone(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z"},
	}
	wrapped := wrapEventWithDaysWithTimezone(ev, "America/New_York", nil)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped event")
	}
	if wrapped.Timezone != "America/New_York" {
		t.Fatalf("expected timezone America/New_York, got %q", wrapped.Timezone)
	}
	if wrapped.StartDayOfWeek != "Wednesday" {
		t.Fatalf("expected Wednesday, got %q", wrapped.StartDayOfWeek)
	}
}

func TestWrapEventWithDaysWithTimezone_WithLocation(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z"},
	}
	loc, _ := time.LoadLocation("Europe/London")
	wrapped := wrapEventWithDaysWithTimezone(ev, "Europe/London", loc)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped event")
	}
	if wrapped.Timezone != "Europe/London" {
		t.Fatalf("expected timezone Europe/London, got %q", wrapped.Timezone)
	}
}

func TestWrapEventWithDaysWithTimezone_EventTimezoneOverride(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z", TimeZone: "Asia/Tokyo"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z", TimeZone: "Asia/Tokyo"},
	}
	wrapped := wrapEventWithDaysWithTimezone(ev, "America/New_York", nil)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped event")
	}
	// Calendar timezone should be used, but EventTimezone should capture the event's timezone
	if wrapped.Timezone != "America/New_York" {
		t.Fatalf("expected calendar timezone America/New_York, got %q", wrapped.Timezone)
	}
	if wrapped.EventTimezone != "Asia/Tokyo" {
		t.Fatalf("expected event timezone Asia/Tokyo, got %q", wrapped.EventTimezone)
	}
}

func TestWrapEventWithDaysWithTimezone_FallbackToEventTimezone(t *testing.T) {
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z", TimeZone: "Europe/Paris"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z", TimeZone: "Europe/Paris"},
	}
	// No calendar timezone provided - should fallback to event timezone
	wrapped := wrapEventWithDaysWithTimezone(ev, "", nil)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped event")
	}
	if wrapped.Timezone != "Europe/Paris" {
		t.Fatalf("expected fallback to event timezone Europe/Paris, got %q", wrapped.Timezone)
	}
	// EventTimezone should be empty when it matches the resolved timezone
	if wrapped.EventTimezone != "" {
		t.Fatalf("expected empty EventTimezone when it matches Timezone, got %q", wrapped.EventTimezone)
	}
}

// Tests for resolveEventTimezone function
func TestResolveEventTimezone(t *testing.T) {
	tests := []struct {
		name             string
		event            *calendar.Event
		calendarTimezone string
		loc              *time.Location
		wantTimezone     string
		wantLocNil       bool
	}{
		{
			name:             "calendar timezone with nil loc",
			event:            &calendar.Event{},
			calendarTimezone: "America/New_York",
			loc:              nil,
			wantTimezone:     "America/New_York",
			wantLocNil:       false,
		},
		{
			name:             "fallback to event timezone",
			event:            &calendar.Event{Start: &calendar.EventDateTime{TimeZone: "Europe/London"}},
			calendarTimezone: "",
			loc:              nil,
			wantTimezone:     "Europe/London",
			wantLocNil:       false,
		},
		{
			name:             "invalid calendar timezone fallback to event",
			event:            &calendar.Event{Start: &calendar.EventDateTime{TimeZone: "Asia/Tokyo"}},
			calendarTimezone: "Invalid/Timezone",
			loc:              nil,
			wantTimezone:     "Asia/Tokyo",
			wantLocNil:       false,
		},
		{
			name:             "both invalid timezones",
			event:            &calendar.Event{Start: &calendar.EventDateTime{TimeZone: "Invalid/EventTz"}},
			calendarTimezone: "Invalid/CalendarTz",
			loc:              nil,
			wantTimezone:     "",
			wantLocNil:       true,
		},
		{
			name:             "empty everything",
			event:            &calendar.Event{},
			calendarTimezone: "",
			loc:              nil,
			wantTimezone:     "",
			wantLocNil:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTz, gotLoc := resolveEventTimezone(tc.event, tc.calendarTimezone, tc.loc)
			if gotTz != tc.wantTimezone {
				t.Fatalf("resolveEventTimezone() timezone = %q, want %q", gotTz, tc.wantTimezone)
			}
			if (gotLoc == nil) != tc.wantLocNil {
				t.Fatalf("resolveEventTimezone() loc nil = %v, want %v", gotLoc == nil, tc.wantLocNil)
			}
		})
	}
}

func TestResolveEventTimezone_PreserveProvidedLoc(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	event := &calendar.Event{}

	// When loc is provided, it should be preserved
	gotTz, gotLoc := resolveEventTimezone(event, "America/New_York", loc)
	if gotTz != "America/New_York" {
		t.Fatalf("expected timezone America/New_York, got %q", gotTz)
	}
	if gotLoc != loc {
		t.Fatal("expected provided location to be preserved")
	}
}

func TestWrapEventWithDaysWithTimezone_StartLocalEndLocal(t *testing.T) {
	// Test that StartLocal and EndLocal are correctly populated
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2025-01-01T10:00:00Z"},
		End:   &calendar.EventDateTime{DateTime: "2025-01-01T11:00:00Z"},
	}
	wrapped := wrapEventWithDaysWithTimezone(ev, "UTC", nil)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped event")
	}
	// StartLocal should be formatted in the calendar timezone
	if wrapped.StartLocal == "" {
		t.Fatal("expected StartLocal to be populated")
	}
	if wrapped.EndLocal == "" {
		t.Fatal("expected EndLocal to be populated")
	}
}

func TestWrapEventWithDaysWithTimezone_DateOnlyEvent(t *testing.T) {
	// Test that date-only events have correct local times
	ev := &calendar.Event{
		Start: &calendar.EventDateTime{Date: "2025-01-02"},
		End:   &calendar.EventDateTime{Date: "2025-01-03"},
	}
	wrapped := wrapEventWithDaysWithTimezone(ev, "UTC", nil)
	if wrapped == nil {
		t.Fatal("expected non-nil wrapped event")
	}
	// For date-only events, StartLocal should be the date string
	if wrapped.StartLocal != "2025-01-02" {
		t.Fatalf("expected StartLocal '2025-01-02', got %q", wrapped.StartLocal)
	}
	if wrapped.EndLocal != "2025-01-03" {
		t.Fatalf("expected EndLocal '2025-01-03', got %q", wrapped.EndLocal)
	}
}
