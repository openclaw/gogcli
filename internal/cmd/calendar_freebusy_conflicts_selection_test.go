package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func TestCalendarFreeBusyCmd_ResolvesCalendarName(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "primary", "summary": "Primary"},
					{"id": "work@example.com", "summary": "Work"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(path, "/freeBusy"):
			var payload struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, item := range payload.Items {
				gotIDs = append(gotIDs, item.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "freebusy",
				"--cal", "Work",
				"--from", "2026-01-10T00:00:00Z",
				"--to", "2026-01-11T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if len(gotIDs) != 1 || gotIDs[0] != "work@example.com" {
		t.Fatalf("expected resolved calendar id work@example.com, got %#v", gotIDs)
	}
}

func TestCalendarConflictsCmd_AllCalendarsSelection(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	var gotIDs []string
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case r.Method == http.MethodGet && path == "/users/me/calendarList":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "primary", "summary": "Primary"},
					{"id": "work@example.com", "summary": "Work"},
				},
			})
		case r.Method == http.MethodPost && strings.Contains(path, "/freeBusy"):
			var payload struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			for _, item := range payload.Items {
				gotIDs = append(gotIDs, item.ID)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"primary":          map[string]any{"busy": []map[string]any{}},
					"work@example.com": map[string]any{"busy": []map[string]any{}},
				},
			})
		default:
			http.NotFound(w, r)
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

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "conflicts",
				"--all",
				"--from", "2026-01-10T00:00:00Z",
				"--to", "2026-01-11T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	sort.Strings(gotIDs)
	if len(gotIDs) != 2 || gotIDs[0] != "primary" || gotIDs[1] != "work@example.com" {
		t.Fatalf("expected all calendar ids [primary work@example.com], got %#v", gotIDs)
	}
}
