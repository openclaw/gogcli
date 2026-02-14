package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestCalendarDeleteCmd_ScopeSingle(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
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
