package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestListCalendarEvents_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event", "start": map[string]any{"dateTime": "2025-01-01T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T11:00:00Z"}},
				},
				"nextPageToken": "next",
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

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		if err := listCalendarEvents(ctx, svc, "cal1", "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", false); err != nil {
			t.Fatalf("listCalendarEvents: %v", err)
		}
	})

	var parsed struct {
		Events []map[string]any `json:"events"`
		Next   string           `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 1 || parsed.Next != "next" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func TestCalendarEventsCmd_DefaultsToPrimary(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/primary/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event"},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	})))
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
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &CalendarEventsCmd{
		From: "2025-01-01T00:00:00Z",
		To:   "2025-01-02T00:00:00Z",
	}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"events\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCalendarEventsCmd_CalendarsFlag(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var mu sync.Mutex
	calls := make(map[string]int)

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Family"},
					{"id": "c3", "summary": "Other"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c1/events") && r.Method == http.MethodGet:
			mu.Lock()
			calls["c1"]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event 1"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c2/events") && r.Method == http.MethodGet:
			mu.Lock()
			calls["c2"]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e2", "summary": "Event 2"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c3/events") && r.Method == http.MethodGet:
			mu.Lock()
			calls["c3"]++
			mu.Unlock()
			http.Error(w, "unexpected calendar", http.StatusBadRequest)
			return
		default:
			http.NotFound(w, r)
			return
		}
	})))
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
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &CalendarEventsCmd{
		Calendars: "1,Family",
		From:      "2025-01-01T00:00:00Z",
		To:        "2025-01-02T00:00:00Z",
	}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, flags); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var parsed struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("unexpected events: %#v", parsed.Events)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls["c1"] == 0 || calls["c2"] == 0 || calls["c3"] != 0 {
		t.Fatalf("unexpected calendar calls: %#v", calls)
	}
}

func TestResolveCalendarIDs_IndexOutOfRange(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = resolveCalendarIDs(context.Background(), svc, []string{"2"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestResolveCalendarIDs_AmbiguousName(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Work"},
					{"id": "c3", "summary": "Family"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = resolveCalendarIDs(context.Background(), svc, []string{"Work"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestResolveCalendarIDs_UnrecognizedName(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Family"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer srv.Close()

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Test single unrecognized name
	_, err = resolveCalendarIDs(context.Background(), svc, []string{"NonExistent"})
	if err == nil {
		t.Fatalf("expected error for unrecognized calendar name")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "unrecognized calendar name(s)") {
		t.Fatalf("expected error message to mention unrecognized calendar, got: %v", err)
	}
	if !strings.Contains(err.Error(), "NonExistent") {
		t.Fatalf("expected error message to include the unrecognized name, got: %v", err)
	}

	// Test multiple unrecognized names
	_, err = resolveCalendarIDs(context.Background(), svc, []string{"Work", "Unknown1", "Unknown2"})
	if err == nil {
		t.Fatalf("expected error for unrecognized calendar names")
	}
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Unknown1") || !strings.Contains(err.Error(), "Unknown2") {
		t.Fatalf("expected error message to include all unrecognized names, got: %v", err)
	}

	// Test valid names still work
	ids, err := resolveCalendarIDs(context.Background(), svc, []string{"Work", "Family"})
	if err != nil {
		t.Fatalf("unexpected error for valid calendar names: %v", err)
	}
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}
