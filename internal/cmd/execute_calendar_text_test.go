package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestExecute_CalendarEvent_Text(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/calendars/c1@example.com/events/e1") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "e1",
			"summary":     "Hello",
			"location":    "Room 1",
			"description": "Desc",
			"status":      "confirmed",
			"htmlLink":    "https://example.com/e1",
			"start":       map[string]any{"dateTime": "2025-12-17T10:00:00Z"},
			"end":         map[string]any{"dateTime": "2025-12-17T11:00:00Z"},
			"attendees": []map[string]any{
				{"email": "a@b.com"},
				{"email": "b@b.com"},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "calendar", "event", "c1@example.com", "e1"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "id\te1") || !strings.Contains(out, "location\tRoom 1") || !strings.Contains(out, "attendee\ta@b.com\t") || !strings.Contains(out, "attendee\tb@b.com\t") || !strings.Contains(out, "link\thttps://example.com/e1") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_CalendarAcl_Text(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/calendars/c1@example.com/acl") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"role": "reader", "scope": map[string]any{"type": "user", "value": "a@b.com"}},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "calendar", "acl", "c1@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "SCOPE_TYPE") || !strings.Contains(out, "user") || !strings.Contains(out, "a@b.com") || !strings.Contains(out, "reader") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_CalendarCalendars_Text(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "c1", "summary": "One", "accessRole": "owner"},
				{"id": "c2", "summary": "Two", "accessRole": "reader"},
			},
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "a@b.com", "calendar", "calendars"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "ID") || !strings.Contains(out, "ROLE") || !strings.Contains(out, "c1") || !strings.Contains(out, "c2") {
		t.Fatalf("unexpected out=%q", out)
	}
}
