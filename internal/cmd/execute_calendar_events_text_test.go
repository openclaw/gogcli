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

func TestExecute_CalendarEvents_Text_WithPaging(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendars/c1@example.com/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "S", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
				"nextPageToken": "npt",
			})
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
	result := executeWithCalendarTestService(t, []string{"--account", "a@b.com", "calendar", "events", "c1@example.com", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stderr, "# More results: use --all-pages to fetch every page, or --page npt for the next page") {
		t.Fatalf("unexpected stderr=%q", result.stderr)
	}
	out := result.stdout
	if !strings.Contains(out, "ID") || !strings.Contains(out, "START") || !strings.Contains(out, "e1") || !strings.Contains(out, "S") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_CalendarEvents_Text_All(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/users/me/calendarList"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1"},
					{"id": "c2"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c1/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "S1", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c2/events"):
			// Simulate calendar fetch failure path (ignored).
			http.Error(w, "nope", http.StatusForbidden)
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
	result := executeWithCalendarTestService(t, []string{"--account", "a@b.com", "calendar", "events", "--all", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	out := result.stdout
	if !strings.Contains(out, "CALENDAR") || !strings.Contains(out, "c1") || !strings.Contains(out, "e1") || !strings.Contains(out, "S1") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_CalendarEvents_Text_AllWithPagingHint(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/users/me/calendarList"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1"},
					{"id": "c2"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c1/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "S1", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
				"nextPageToken": "c1-next",
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c2/events"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e2", "summary": "S2", "start": map[string]any{"dateTime": "2025-12-17T12:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T13:00:00Z"}},
				},
			})
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
	result := executeWithCalendarTestService(t, []string{"--account", "a@b.com", "calendar", "events", "--all", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"}, svc)
	if result.err != nil {
		t.Fatalf("Execute: %v", result.err)
	}
	if !strings.Contains(result.stderr, "# More results: use --all-pages to fetch every page (c1 has more results)") {
		t.Fatalf("unexpected stderr=%q", result.stderr)
	}
	out := result.stdout
	if !strings.Contains(out, "CALENDAR") || !strings.Contains(out, "c1") || !strings.Contains(out, "e1") || !strings.Contains(out, "S1") || !strings.Contains(out, "c2") || !strings.Contains(out, "e2") || !strings.Contains(out, "S2") {
		t.Fatalf("unexpected out=%q", out)
	}
}

func TestExecute_CalendarEvents_ValidatesPageScopeWithMultipleCalendars(t *testing.T) {
	for _, test := range []struct {
		name       string
		selector   []string
		pageFlag   string
		pageValue  string
		wantError  bool
		wantEvents int
	}{
		{name: "cal", selector: []string{"--cal", "c1", "--cal", "c2"}, pageFlag: "--page", pageValue: "calendar-specific-token", wantError: true},
		{name: "calendars cursor alias", selector: []string{"--calendars", "c1,c2"}, pageFlag: "--cursor", pageValue: "calendar-specific-token", wantError: true},
		{name: "all", selector: []string{"--all"}, pageFlag: "--page", pageValue: "calendar-specific-token", wantError: true},
		{name: "blank token", selector: []string{"--calendars", "c1,c2"}, pageFlag: "--page", pageValue: "   ", wantEvents: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			eventRequests := 0
			srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/users/me/calendarList"):
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"items": []map[string]any{{"id": "c1"}, {"id": "c2"}},
					})
					return
				case strings.Contains(r.URL.Path, "/events"):
					eventRequests++
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
					return
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

			args := []string{"--account", "a@b.com", "calendar", "events"}
			args = append(args, test.selector...)
			args = append(args,
				test.pageFlag, test.pageValue,
				"--from", "2025-12-17T00:00:00Z",
				"--to", "2025-12-18T00:00:00Z",
			)
			result := executeWithCalendarTestService(t, args, svc)
			if test.wantError {
				if result.err == nil || ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "--page/--cursor can only be used with one calendar") {
					t.Fatalf("error = %v, exit = %d, want multi-calendar page-token usage error", result.err, ExitCode(result.err))
				}
			} else if result.err != nil {
				t.Fatalf("Execute: %v", result.err)
			}
			if eventRequests != test.wantEvents {
				t.Fatalf("events.list requests = %d, want %d", eventRequests, test.wantEvents)
			}
		})
	}
}
