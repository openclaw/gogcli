package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestCalendarMoveCmd_RunJSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var (
		gotDestination string
		gotSendUpdates string
	)
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodPost && path == "/calendars/agent@example.com/events/ev/move":
			gotDestination = r.URL.Query().Get("destination")
			gotSendUpdates = r.URL.Query().Get("sendUpdates")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev",
				"summary": "Moved",
				"organizer": map[string]any{
					"email": "owner@example.com",
				},
			})
			return
		case r.Method == http.MethodGet && path == "/calendars/owner@example.com":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "owner@example.com",
				"timeZone": "UTC",
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)

	cmd := CalendarMoveCmd{
		CalendarID:            "agent@example.com",
		EventID:               "ev",
		DestinationCalendarID: "owner@example.com",
		SendUpdates:           "all",
	}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("CalendarMoveCmd: %v", err)
		}
	})
	var payload struct {
		Event struct {
			ID        string `json:"id"`
			Summary   string `json:"summary"`
			Organizer struct {
				Email string `json:"email"`
			} `json:"organizer"`
		} `json:"event"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if gotDestination != "owner@example.com" || gotSendUpdates != "all" {
		t.Fatalf("unexpected query destination=%q sendUpdates=%q", gotDestination, gotSendUpdates)
	}
	if payload.Event.ID != "ev" || payload.Event.Organizer.Email != "owner@example.com" {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestCalendarMoveCmd_DryRunSkipsService(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	called := false
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		called = true
		return nil, errors.New("unexpected service creation")
	}

	ctx := newCalendarJSONContext(t)

	cmd := CalendarMoveCmd{
		CalendarID:            "agent@example.com",
		EventID:               "ev",
		DestinationCalendarID: "owner@example.com",
	}
	out := captureStdout(t, func() {
		err := cmd.Run(ctx, &RootFlags{Account: "a@b.com", DryRun: true, NoInput: true})
		if ExitCode(err) != 0 {
			t.Fatalf("expected dry-run exit, got %v", err)
		}
	})
	if called {
		t.Fatalf("expected no service creation during dry-run")
	}
	var payload struct {
		Op      string         `json:"op"`
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if payload.Op != "calendar.move" || payload.Request["destination_calendar_id"] != "owner@example.com" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}

func TestCalendarMoveCmd_RejectsSameCalendar(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	called := false
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		called = true
		return nil, errors.New("unexpected service creation")
	}

	err := (&CalendarMoveCmd{
		CalendarID:            "owner@example.com",
		EventID:               "ev",
		DestinationCalendarID: "owner@example.com",
	}).Run(newCalendarJSONContext(t), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "destination calendar must differ") {
		t.Fatalf("expected same-calendar error, got %v", err)
	}
	if called {
		t.Fatalf("expected no service creation for same-calendar validation")
	}
}
