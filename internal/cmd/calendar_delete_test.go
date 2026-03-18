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

func TestCalendarDeleteCmd_ScopeSingle(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev",
				"recurrence": []string{"RRULE:FREQ=DAILY"},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/cal@example.com/events/ev/instances"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "ev_1",
						"originalStartTime": map[string]any{
							"dateTime": "2025-01-02T10:00:00Z",
						},
					},
				},
			})
			return
		case r.Method == http.MethodDelete && path == "/calendars/cal@example.com/events/ev_1":
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)

	cmd := CalendarDeleteCmd{
		CalendarID:        "cal@example.com",
		EventID:           "ev",
		Scope:             scopeSingle,
		OriginalStartTime: "2025-01-02T10:00:00Z",
	}
	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("CalendarDeleteCmd: %v", err)
		}
	})
	var payload struct {
		Deleted bool   `json:"deleted"`
		EventID string `json:"eventId"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !payload.Deleted || payload.EventID != "ev_1" {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestCalendarDeleteCmd_SendUpdates(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotSendUpdates string
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			// resolveCalendarID() lists calendars and matches by Summary.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":       "cal",
						"summary":  "cal",
						"timeZone": "UTC",
					},
				},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/") && !strings.Contains(path, "/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "cal",
				"timeZone": "UTC",
			})
			return
		case r.Method == http.MethodDelete && path == "/calendars/cal/events/ev":
			gotSendUpdates = r.URL.Query().Get("sendUpdates")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)

	cmd := CalendarDeleteCmd{
		CalendarID:  "cal",
		EventID:     "ev",
		SendUpdates: "all",
	}
	flags := &RootFlags{Account: "a@b.com", Force: true}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("CalendarDeleteCmd: %v", err)
	}
	if gotSendUpdates != "all" {
		t.Fatalf("expected sendUpdates=all, got %q", gotSendUpdates)
	}
}

func TestCalendarDeleteCmd_ScopeFuture(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var patchedRecurrence []string
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev",
				"recurrence": []string{"RRULE:FREQ=DAILY"},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/cal@example.com/events/ev/instances"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "ev_2",
						"originalStartTime": map[string]any{
							"dateTime": "2025-01-02T10:00:00Z",
						},
					},
				},
			})
			return
		case r.Method == http.MethodDelete && path == "/calendars/cal@example.com/events/ev_2":
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			patchedRecurrence = append([]string{}, body.Recurrence...)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)

	cmd := CalendarDeleteCmd{
		CalendarID:        "cal@example.com",
		EventID:           "ev",
		Scope:             scopeFuture,
		OriginalStartTime: "2025-01-02T10:00:00Z",
	}
	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("CalendarDeleteCmd: %v", err)
		}
	})
	var payload struct {
		Deleted bool   `json:"deleted"`
		EventID string `json:"eventId"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !payload.Deleted || payload.EventID != "ev_2" || len(patchedRecurrence) == 0 {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestCalendarDeleteCmd_DryRunSkipsService(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	called := false
	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		called = true
		return nil, errors.New("unexpected service creation")
	}

	ctx := newCalendarJSONContext(t)

	cmd := CalendarDeleteCmd{CalendarID: "cal@example.com", EventID: "ev"}
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
	if payload.Op != "calendar.delete" || payload.Request["event_id"] != "ev" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
}

func TestCalendarDeleteCmd_ScopeFuture_InstanceEventID(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var patchedRecurrence []string
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev_instance":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":               "ev_instance",
				"recurringEventId": "ev_master",
			})
			return
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev_master":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         "ev_master",
				"recurrence": []string{"RRULE:FREQ=DAILY"},
			})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/calendars/cal@example.com/events/ev_master/instances"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "ev_2",
						"originalStartTime": map[string]any{
							"dateTime": "2025-01-02T10:00:00Z",
						},
					},
				},
			})
			return
		case r.Method == http.MethodDelete && path == "/calendars/cal@example.com/events/ev_2":
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev_master":
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			patchedRecurrence = append([]string{}, body.Recurrence...)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)

	cmd := CalendarDeleteCmd{
		CalendarID:        "cal@example.com",
		EventID:           "ev_instance",
		Scope:             scopeFuture,
		OriginalStartTime: "2025-01-02T10:00:00Z",
	}
	flags := &RootFlags{Account: "a@b.com", Force: true}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("CalendarDeleteCmd: %v", err)
		}
	})
	var payload struct {
		Deleted bool   `json:"deleted"`
		EventID string `json:"eventId"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !payload.Deleted || payload.EventID != "ev_2" || len(patchedRecurrence) == 0 {
		t.Fatalf("unexpected output: %#v", payload)
	}
}
