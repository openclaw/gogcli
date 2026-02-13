package cmd

import (
	"testing"
	"time"
)

func TestParseTimeExpr(t *testing.T) {
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

	parsed, err := parseTimeExpr("today", now, time.UTC)
	if err != nil {
		t.Fatalf("parseTimeExpr today: %v", err)
	}

	if !parsed.Equal(startOfDay(now)) {
		t.Fatalf("unexpected today: %v", parsed)
	}

	parsed, err = parseTimeExpr("2025-01-05", now, time.UTC)
	if err != nil {
		t.Fatalf("parseTimeExpr date: %v", err)
	}

	if parsed.Year() != 2025 || parsed.Day() != 5 {
		t.Fatalf("unexpected date: %v", parsed)
	}

	if _, err = parseTimeExpr("nope", now, time.UTC); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestParseTimeExprMore(t *testing.T) {
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	loc := time.FixedZone("Offset", -5*3600)

	parsed, err := parseTimeExpr("2025-01-05T14:00:00Z", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr rfc3339: %v", err)
	}

	if parsed.Location() != time.UTC {
		t.Fatalf("expected UTC location, got %v", parsed.Location())
	}

	// Test ISO 8601 with numeric timezone without colon (macOS date +%z format)
	parsed, err = parseTimeExpr("2025-01-09T16:38:41-0800", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr iso8601 numeric tz: %v", err)
	}
	if parsed.Hour() != 16 || parsed.Minute() != 38 || parsed.Second() != 41 {
		t.Fatalf("unexpected iso8601 numeric tz time: %v", parsed)
	}
	_, offset := parsed.Zone()
	if offset != -8*3600 {
		t.Fatalf("unexpected iso8601 numeric tz offset: %d", offset)
	}

	parsed, err = parseTimeExpr("yesterday", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr yesterday: %v", err)
	}

	if !parsed.Equal(startOfDay(now.AddDate(0, 0, -1))) {
		t.Fatalf("unexpected yesterday: %v", parsed)
	}

	parsed, err = parseTimeExpr("next monday", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr next monday: %v", err)
	}

	if parsed.Weekday() != time.Monday {
		t.Fatalf("unexpected weekday: %v", parsed.Weekday())
	}

	parsed, err = parseTimeExpr("2025-01-05T10:00:00", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr local datetime: %v", err)
	}

	if parsed.Location() != loc {
		t.Fatalf("expected loc, got %v", parsed.Location())
	}

	parsed, err = parseTimeExpr("2025-01-05 10:00", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr local short: %v", err)
	}

	if parsed.Location() != loc {
		t.Fatalf("expected loc, got %v", parsed.Location())
	}
}

func TestParseWeekday(t *testing.T) {
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	parsed, ok := parseWeekday("monday", now)
	if !ok || parsed.Weekday() != time.Monday {
		t.Fatalf("unexpected weekday: %v ok=%v", parsed, ok)
	}

	next, ok := parseWeekday("next monday", now)
	if !ok || next.Weekday() != time.Monday || !next.After(startOfDay(now)) {
		t.Fatalf("unexpected next weekday: %v ok=%v", next, ok)
	}
}

func TestResolveWeekStart(t *testing.T) {
	day, err := resolveWeekStart("sun")
	if err != nil || day != time.Sunday {
		t.Fatalf("unexpected week start: %v %v", day, err)
	}

	if _, err = resolveWeekStart("nope"); err == nil {
		t.Fatalf("expected error for invalid week start")
	}
}

func TestTimeRangeFormatting(t *testing.T) {
	tr := &TimeRange{
		From: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	from, to := tr.FormatRFC3339()
	if from == "" || to == "" {
		t.Fatalf("expected formatted range")
	}

	if tr.FormatHuman() == "" {
		t.Fatalf("expected human format")
	}
}

func TestWeekBounds(t *testing.T) {
	now := time.Date(2025, 1, 8, 12, 0, 0, 0, time.UTC) // Wednesday
	start := startOfWeek(now, time.Monday)
	end := endOfWeek(now, time.Monday)
	if start.Weekday() != time.Monday || end.Weekday() != time.Sunday {
		t.Fatalf("unexpected week bounds: %v to %v", start.Weekday(), end.Weekday())
	}

	startSun := startOfWeek(now, time.Sunday)
	endSun := endOfWeek(now, time.Sunday)
	if startSun.Weekday() != time.Sunday || endSun.Weekday() != time.Saturday {
		t.Fatalf("unexpected week bounds (sun): %v to %v", startSun.Weekday(), endSun.Weekday())
	}
}

func TestDayBounds(t *testing.T) {
	now := time.Date(2025, 1, 8, 12, 34, 56, 0, time.UTC)
	start := startOfDay(now)
	end := endOfDay(now)
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Fatalf("unexpected startOfDay: %v", start)
	}

	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Fatalf("unexpected endOfDay: %v", end)
	}
}

func TestParseWeekStartVariants(t *testing.T) {
	if wd, ok := parseWeekStart("tues"); !ok || wd != time.Tuesday {
		t.Fatalf("unexpected week start: %v ok=%v", wd, ok)
	}

	if _, ok := parseWeekStart("nope"); ok {
		t.Fatalf("expected invalid week start")
	}
}

func TestParseTimeExpr_LocalMinutesWithT(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	loc := time.FixedZone("Offset", -5*3600)

	parsed, err := parseTimeExpr("2025-01-05T10:30", now, loc)
	if err != nil {
		t.Fatalf("parseTimeExpr local minutes: %v", err)
	}
	if parsed.Location() != loc {
		t.Fatalf("expected loc, got %v", parsed.Location())
	}
	if parsed.Hour() != 10 || parsed.Minute() != 30 {
		t.Fatalf("unexpected time: %v", parsed)
	}
}
