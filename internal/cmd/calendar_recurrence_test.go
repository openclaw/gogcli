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

func TestOriginalStartRange(t *testing.T) {
	minRange, maxRange, err := originalStartRange("2025-01-02T10:00:00Z")
	if err != nil {
		t.Fatalf("originalStartRange: %v", err)
	}
	if !strings.Contains(minRange, "2025-01-02") || !strings.Contains(maxRange, "2025-01-02") {
		t.Fatalf("unexpected range: %s %s", minRange, maxRange)
	}

	minRange, maxRange, err = originalStartRange("2025-01-02")
	if err != nil {
		t.Fatalf("originalStartRange date: %v", err)
	}
	if !strings.Contains(minRange, "2025-01-02") || !strings.Contains(maxRange, "2025-01-03") {
		t.Fatalf("unexpected date range: %s %s", minRange, maxRange)
	}
}

func TestMatchesOriginalStart(t *testing.T) {
	event := &calendar.Event{
		OriginalStartTime: &calendar.EventDateTime{DateTime: "2025-01-02T10:00:00Z"},
		Start:             &calendar.EventDateTime{Date: "2025-01-02"},
	}
	if !matchesOriginalStart(event, "2025-01-02T10:00:00Z") {
		t.Fatalf("expected match for datetime")
	}
	if !matchesOriginalStart(event, "2025-01-02") {
		t.Fatalf("expected match for date")
	}
}

func TestTruncateRecurrence_Extra(t *testing.T) {
	rules := []string{"RRULE:FREQ=DAILY;COUNT=10", "EXDATE:20250103T100000Z"}
	updated, err := truncateRecurrence(rules, "2025-01-05T10:00:00Z")
	if err != nil {
		t.Fatalf("truncateRecurrence: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("unexpected updated rules: %v", updated)
	}
	if !strings.Contains(updated[0], "UNTIL=") {
		t.Fatalf("expected UNTIL in rule: %v", updated[0])
	}
	if updated[1] != "EXDATE:20250103T100000Z" {
		t.Fatalf("expected exdate preserved")
	}
}

func TestRecurrenceUntil_Extra(t *testing.T) {
	until, err := recurrenceUntil("2025-01-02T10:00:00Z")
	if err != nil {
		t.Fatalf("recurrenceUntil: %v", err)
	}
	if !strings.HasPrefix(until, "20250102") {
		t.Fatalf("unexpected until: %s", until)
	}

	until, err = recurrenceUntil("2025-01-02")
	if err != nil {
		t.Fatalf("recurrenceUntil date: %v", err)
	}
	if until != time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format("20060102") {
		t.Fatalf("unexpected date until: %s", until)
	}
}

func TestResolveRecurringSeriesID_Instance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodGet && path == "/calendars/cal/events/ev_instance" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "ev_instance",
				"recurringEventId": "ev_master",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	got, err := resolveRecurringSeriesID(context.Background(), svc, "cal", "ev_instance")
	if err != nil {
		t.Fatalf("resolveRecurringSeriesID: %v", err)
	}
	if got != "ev_master" {
		t.Fatalf("unexpected recurring series id: %q", got)
	}
}

func TestResolveRecurringParentEvent_Instance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ev_instance":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "ev_instance",
				"recurringEventId": "ev_master",
			})
		case r.Method == http.MethodGet && path == "/calendars/cal/events/ev_master":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev_master",
				"recurrence": []string{"RRULE:FREQ=WEEKLY"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	parentID, recurrence, err := resolveRecurringParentEvent(context.Background(), svc, "cal", "ev_instance")
	if err != nil {
		t.Fatalf("resolveRecurringParentEvent: %v", err)
	}
	if parentID != "ev_master" {
		t.Fatalf("unexpected parent id: %q", parentID)
	}
	if len(recurrence) != 1 || recurrence[0] != "RRULE:FREQ=WEEKLY" {
		t.Fatalf("unexpected recurrence: %#v", recurrence)
	}
}
