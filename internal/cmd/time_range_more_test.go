package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func newCalendarServiceWithTimezone(t *testing.T, tz string) *calendar.Service {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList/primary") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"summary":  "Test Calendar",
				"timeZone": tz,
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestResolveTimeRangeWithDefaultsToday(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{Today: true}
	defaults := TimeRangeDefaults{
		FromOffset:   time.Hour,
		ToOffset:     2 * time.Hour,
		ToFromOffset: 3 * time.Hour,
	}

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, defaults)
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	if tr.From.Hour() != 0 || tr.From.Minute() != 0 || tr.From.Second() != 0 {
		t.Fatalf("expected start of day, got %v", tr.From)
	}

	if tr.To.Hour() != 23 || tr.To.Minute() != 59 || tr.To.Second() != 59 {
		t.Fatalf("expected end of day, got %v", tr.To)
	}

	if !tr.From.Before(tr.To) {
		t.Fatalf("expected from before to: %v -> %v", tr.From, tr.To)
	}
}

func TestResolveTimeRangeWithDefaultsFromTo(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{
		From: "2025-01-05T10:00:00Z",
		To:   "2025-01-05T12:00:00Z",
	}
	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	expectedFrom := time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC)
	expectedTo := time.Date(2025, 1, 5, 12, 0, 0, 0, time.UTC)
	if !tr.From.Equal(expectedFrom) || !tr.To.Equal(expectedTo) {
		t.Fatalf("unexpected range: %v -> %v", tr.From, tr.To)
	}
}

func TestResolveTimeRangeWithDefaultsToDateOnlyEndOfDay(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{
		From: "2025-01-05T10:00:00Z",
		To:   "2025-01-05",
	}
	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{})
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	expectedFrom := time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC)
	expectedTo := time.Date(2025, 1, 5, 23, 59, 59, 999999999, time.UTC)
	if !tr.From.Equal(expectedFrom) {
		t.Fatalf("unexpected from: %v", tr.From)
	}
	if !tr.To.Equal(expectedTo) {
		t.Fatalf("unexpected to: %v", tr.To)
	}
}

func TestResolveTimeRangeWithDefaultsFromOffset(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{From: "2025-01-05T10:00:00Z"}
	defaults := TimeRangeDefaults{ToFromOffset: 2 * time.Hour}

	tr, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, defaults)
	if err != nil {
		t.Fatalf("ResolveTimeRangeWithDefaults: %v", err)
	}

	expectedFrom := time.Date(2025, 1, 5, 10, 0, 0, 0, time.UTC)
	if !tr.From.Equal(expectedFrom) {
		t.Fatalf("unexpected from: %v", tr.From)
	}

	if tr.To.Sub(tr.From) != 2*time.Hour {
		t.Fatalf("unexpected duration: %v", tr.To.Sub(tr.From))
	}
}

func TestResolveTimeRangeWithDefaultsInvalidFrom(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{From: "nope"}
	if _, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestResolveTimeRangeWithDefaultsWeekStartError(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "UTC")
	flags := TimeRangeFlags{Week: true, WeekStart: "nope"}
	if _, err := ResolveTimeRangeWithDefaults(context.Background(), svc, flags, TimeRangeDefaults{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestGetUserTimezoneFallback(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "")
	loc, err := getUserTimezone(context.Background(), svc)
	if err != nil {
		t.Fatalf("getUserTimezone: %v", err)
	}

	if loc != time.UTC {
		t.Fatalf("expected UTC, got %v", loc)
	}
}

func TestGetUserTimezoneInvalid(t *testing.T) {
	svc := newCalendarServiceWithTimezone(t, "Bad/Zone")
	if _, err := getUserTimezone(context.Background(), svc); err == nil {
		t.Fatalf("expected error")
	}
}

func TestIsDayExpr(t *testing.T) {
	loc := time.UTC
	// Use a fixed reference time: Wednesday, January 15, 2025
	now := time.Date(2025, 1, 15, 10, 30, 0, 0, loc)

	tests := []struct {
		name string
		expr string
		want bool
	}{
		// Relative day keywords -> true
		{"today", "today", true},
		{"tomorrow", "tomorrow", true},
		{"yesterday", "yesterday", true},
		{"today uppercase", "TODAY", true},
		{"today mixed case", "ToDay", true},

		// "now" is a precise moment -> false
		{"now", "now", false},
		{"now uppercase", "NOW", false},

		// Weekday names -> true
		{"monday", "monday", true},
		{"tuesday", "tuesday", true},
		{"wednesday", "wednesday", true},
		{"thursday", "thursday", true},
		{"friday", "friday", true},
		{"saturday", "saturday", true},
		{"sunday", "sunday", true},
		{"mon abbreviation", "mon", true},
		{"tue abbreviation", "tue", true},
		{"wed abbreviation", "wed", true},
		{"thu abbreviation", "thu", true},
		{"fri abbreviation", "fri", true},
		{"sat abbreviation", "sat", true},
		{"sun abbreviation", "sun", true},
		{"Monday uppercase", "MONDAY", true},
		{"next monday", "next monday", true},
		{"next tuesday", "next tuesday", true},

		// ISO date (YYYY-MM-DD) -> true
		{"iso date", "2025-01-05", true},
		{"iso date future", "2026-12-31", true},
		{"iso date past", "2020-01-01", true},

		// RFC3339 timestamps -> false (precise moment, not a day)
		{"rfc3339 utc", "2025-01-05T10:00:00Z", false},
		{"rfc3339 offset", "2025-01-05T10:00:00-08:00", false},
		{"rfc3339 positive offset", "2025-01-05T10:00:00+05:30", false},

		// ISO 8601 with numeric timezone (no colon) -> false
		{"iso8601 no colon", "2025-01-05T10:00:00-0800", false},

		// Date with time but no timezone -> false (has time component)
		{"datetime no tz", "2025-01-05T15:04:05", false},
		{"datetime space separator", "2025-01-05 15:04", false},

		// Empty string -> false
		{"empty string", "", false},
		{"whitespace only", "   ", false},

		// Invalid expressions -> false
		{"invalid word", "notaday", false},
		{"invalid format", "01-05-2025", false},
		{"partial date", "2025-01", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDayExpr(tt.expr, now, loc)
			if got != tt.want {
				t.Errorf("isDayExpr(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
