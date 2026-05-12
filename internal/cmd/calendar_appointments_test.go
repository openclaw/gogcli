package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestCalendarAppointmentSchedulesListUsesAppointmentScheduleEventType(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendars/primary/events" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query()["eventTypes"]; len(got) != 1 || got[0] != calendarAppointmentScheduleEventType {
			t.Fatalf("eventTypes query = %v, want [%s]", got, calendarAppointmentScheduleEventType)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":        "as1",
					"summary":   "Office hours",
					"eventType": calendarAppointmentScheduleEventType,
					"start":     map[string]any{"dateTime": "2026-01-01T10:00:00Z"},
					"end":       map[string]any{"dateTime": "2026-01-01T11:00:00Z"},
				},
			},
		})
	})))
	defer closeServer()

	ctx := newCalendarJSONContext(t)
	out := captureStdout(t, func() {
		if err := listCalendarEventsWithEventTypes(ctx, svc, "primary", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", false, []string{calendarAppointmentScheduleEventType}); err != nil {
			t.Fatalf("listCalendarEventsWithEventTypes: %v", err)
		}
	})

	var parsed struct {
		Events []struct {
			ID        string `json:"id"`
			EventType string `json:"eventType"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].ID != "as1" || parsed.Events[0].EventType != calendarAppointmentScheduleEventType {
		t.Fatalf("unexpected output: %#v", parsed.Events)
	}
}

func TestCalendarAppointmentSchedulesListCommandShape(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	svc, closeServer := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendars/primary/events" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("eventTypes"); got != calendarAppointmentScheduleEventType {
			t.Fatalf("eventTypes query = %q, want %q", got, calendarAppointmentScheduleEventType)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	})))
	defer closeServer()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)
	out := captureStdout(t, func() {
		if err := runKong(t, &CalendarAppointmentsCmd{}, []string{"list", "--from", "2026-01-01T00:00:00Z", "--to", "2026-01-02T00:00:00Z"}, ctx, &RootFlags{Account: "a@example.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})
	if out == "" {
		t.Fatal("expected JSON output")
	}
}
