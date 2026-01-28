package cli

import (
	"testing"
	"time"
)

func TestParseRelativeTimeRelativeDurations(t *testing.T) {
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

	parsed, err := ParseRelativeTime("2h ago", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime 2h ago: %v", err)
	}

	if !parsed.Equal(now.Add(-2 * time.Hour)) {
		t.Fatalf("unexpected 2h ago: %v", parsed)
	}

	parsed, err = ParseRelativeTime("30m", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime 30m: %v", err)
	}

	if !parsed.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("unexpected 30m: %v", parsed)
	}

	parsed, err = ParseRelativeTime("1d ago", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime 1d ago: %v", err)
	}

	if !parsed.Equal(now.AddDate(0, 0, -1)) {
		t.Fatalf("unexpected 1d ago: %v", parsed)
	}
}

func TestParseRelativeTimeWeekday(t *testing.T) {
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC) // Friday

	parsed, err := ParseRelativeTime("monday", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime monday: %v", err)
	}

	if parsed.Weekday() != time.Monday || !parsed.After(StartOfDay(now)) {
		t.Fatalf("unexpected monday: %v", parsed)
	}

	parsed, err = ParseRelativeTime("next friday", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime next friday: %v", err)
	}

	if parsed.Weekday() != time.Friday || !parsed.After(StartOfDay(now)) {
		t.Fatalf("unexpected next friday: %v", parsed)
	}
}

func TestParseRelativeTimeDatesAndRFC3339(t *testing.T) {
	loc := time.FixedZone("Offset", -5*3600)
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, loc)

	parsed, err := ParseRelativeTime("2025-01-27", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime date: %v", err)
	}

	if parsed.Location() != loc || parsed.Hour() != 0 {
		t.Fatalf("unexpected date parse: %v", parsed)
	}

	parsed, err = ParseRelativeTime("2025-01-27T10:00:00Z", now)
	if err != nil {
		t.Fatalf("ParseRelativeTime rfc3339: %v", err)
	}

	if parsed.Location() != time.UTC || parsed.Hour() != 10 {
		t.Fatalf("unexpected rfc3339 parse: %v", parsed)
	}
}
