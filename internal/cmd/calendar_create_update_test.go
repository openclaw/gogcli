package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestCalendarCreateCmd_RunJSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev1",
				"summary": "Meeting",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarCreateCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{
			"cal@example.com",
			"--summary", "Meeting",
			"--from", "2025-01-02T10:00:00Z",
			"--to", "2025-01-02T11:00:00Z",
		}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})
	if !strings.Contains(out, "\"event\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCalendarCreateCmd_WithMeetAndAttachments(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var sawConference, sawAttachments bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			sawConference = body.ConferenceData != nil
			sawAttachments = len(body.Attachments) > 0
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev2",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--summary", "Meet",
		"--from", "2025-01-02T10:00:00Z",
		"--to", "2025-01-02T11:00:00Z",
		"--with-meet",
		"--attachment", "https://example.com/file",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !sawConference || !sawAttachments {
		t.Fatalf("expected conference+attachments, sawConference=%v sawAttachments=%v", sawConference, sawAttachments)
	}
}

func TestCalendarCreateCmd_RecurringOffsetTimezoneFallback(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/calendars/") && strings.HasSuffix(r.URL.Path, "/events"):
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev3",
			})
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users/me/calendarList/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"timeZone": "UTC",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"primary",
		"--summary", "Recurring Test",
		"--from", "2026-02-13T08:00:00+02:00",
		"--to", "2026-02-13T09:00:00+02:00",
		"--rrule", "FREQ=WEEKLY;BYDAY=TU,TH",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.Start == nil || gotEvent.Start.TimeZone != "Etc/GMT-2" {
		t.Fatalf("expected start timezone fallback Etc/GMT-2, got %#v", gotEvent.Start)
	}
	if gotEvent.End == nil || gotEvent.End.TimeZone != "Etc/GMT-2" {
		t.Fatalf("expected end timezone fallback Etc/GMT-2, got %#v", gotEvent.End)
	}
	if len(gotEvent.Recurrence) == 0 {
		t.Fatalf("expected recurrence to be set")
	}
}

func TestCalendarUpdateCmd_RunJSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev",
				"summary": "Updated",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarUpdateCmd{}
	out := captureStdout(t, func() {
		if err := runKong(t, cmd, []string{
			"cal@example.com",
			"ev",
			"--summary", "Updated",
			"--scope", "all",
		}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})
	if !strings.Contains(out, "\"event\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCalendarUpdateCmd_AddAttendee(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var patchedAttendees int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/calendars/cal@example.com/events/ev":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
				"attendees": []map[string]any{
					{"email": "a@example.com"},
				},
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			var body calendar.Event
			_ = json.NewDecoder(r.Body).Decode(&body)
			patchedAttendees = len(body.Attendees)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
			})
			return
		default:
			http.NotFound(w, r)
			return
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--add-attendee", "b@example.com",
		"--scope", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if patchedAttendees < 2 {
		t.Fatalf("expected merged attendees, got %d", patchedAttendees)
	}
}

func TestCalendarCreateCmd_EventTypeFocusTimeDefaults(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev1",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--event-type", "focus-time",
		"--from", "2025-01-02T10:00:00Z",
		"--to", "2025-01-02T11:00:00Z",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeFocusTime {
		t.Fatalf("expected focusTime event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Summary != defaultFocusSummary {
		t.Fatalf("expected default summary, got %q", gotEvent.Summary)
	}
	if gotEvent.Transparency != transparencyOpaque {
		t.Fatalf("expected opaque transparency, got %q", gotEvent.Transparency)
	}
	if gotEvent.FocusTimeProperties == nil {
		t.Fatalf("expected focus time properties")
	}
	if gotEvent.FocusTimeProperties.AutoDeclineMode != "declineAllConflictingInvitations" {
		t.Fatalf("unexpected autoDeclineMode: %q", gotEvent.FocusTimeProperties.AutoDeclineMode)
	}
	if gotEvent.FocusTimeProperties.ChatStatus != defaultFocusChatStatus {
		t.Fatalf("unexpected chat status: %q", gotEvent.FocusTimeProperties.ChatStatus)
	}
}

func TestCalendarCreateCmd_EventTypeWorkingLocation(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPost && path == "/calendars/cal@example.com/events" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev1",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarCreateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"--event-type", "working-location",
		"--working-location-type", "office",
		"--working-office-label", "HQ",
		"--from", "2025-01-01",
		"--to", "2025-01-02",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeWorkingLocation {
		t.Fatalf("expected workingLocation event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Summary != "Working from HQ" {
		t.Fatalf("expected working location summary, got %q", gotEvent.Summary)
	}
	if gotEvent.Start == nil || gotEvent.Start.Date != "2025-01-01" {
		t.Fatalf("unexpected start date: %#v", gotEvent.Start)
	}
	if gotEvent.End == nil || gotEvent.End.Date != "2025-01-02" {
		t.Fatalf("unexpected end date: %#v", gotEvent.End)
	}
	if gotEvent.WorkingLocationProperties == nil || gotEvent.WorkingLocationProperties.Type != "officeLocation" {
		t.Fatalf("unexpected working location props: %#v", gotEvent.WorkingLocationProperties)
	}
}

func TestCalendarUpdateCmd_EventTypeOOO(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotEvent calendar.Event
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		if r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev" {
			_ = json.NewDecoder(r.Body).Decode(&gotEvent)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ev",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--event-type", "out-of-office",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if gotEvent.EventType != eventTypeOutOfOffice {
		t.Fatalf("expected outOfOffice event type, got %q", gotEvent.EventType)
	}
	if gotEvent.Transparency != transparencyOpaque {
		t.Fatalf("expected opaque transparency, got %q", gotEvent.Transparency)
	}
	if gotEvent.OutOfOfficeProperties == nil {
		t.Fatalf("expected out-of-office properties")
	}
	if gotEvent.OutOfOfficeProperties.AutoDeclineMode != "declineAllConflictingInvitations" {
		t.Fatalf("unexpected autoDeclineMode: %q", gotEvent.OutOfOfficeProperties.AutoDeclineMode)
	}
	if gotEvent.OutOfOfficeProperties.DeclineMessage != defaultOOODeclineMsg {
		t.Fatalf("unexpected decline message: %q", gotEvent.OutOfOfficeProperties.DeclineMessage)
	}
}

func TestCalendarUpdateCmd_SendUpdates(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotSendUpdates string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/users/me/calendarList/"):
			// getCalendarLocation() fetches the calendar timezone via CalendarList.Get.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "cal",
				"timeZone": "UTC",
			})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal/events/ev":
			gotSendUpdates = r.URL.Query().Get("sendUpdates")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "ev",
				"summary": "Updated",
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal",
		"ev",
		"--summary", "Updated",
		"--send-updates", "all",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if gotSendUpdates != "all" {
		t.Fatalf("expected sendUpdates=all, got %q", gotSendUpdates)
	}
}

func TestCalendarUpdateCmd_ScopeFuture(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var truncated bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev_1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev_1"})
			return
		case r.Method == http.MethodPatch && path == "/calendars/cal@example.com/events/ev":
			truncated = true
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev"})
			return
		default:
			http.NotFound(w, r)
			return
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := &CalendarUpdateCmd{}
	if err := runKong(t, cmd, []string{
		"cal@example.com",
		"ev",
		"--summary", "Updated",
		"--scope", "future",
		"--original-start", "2025-01-02T10:00:00Z",
	}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}
	if !truncated {
		t.Fatalf("expected recurrence truncation")
	}
}
