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
