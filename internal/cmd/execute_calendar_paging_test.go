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

func TestExecute_CalendarCalendars_MaxAndPage_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("maxResults"); got != "1" {
			t.Fatalf("maxResults=%q", got)
		}
		if got := r.URL.Query().Get("pageToken"); got != "p1" {
			t.Fatalf("pageToken=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "c1", "summary": "One", "accessRole": "owner"},
			},
			"nextPageToken": "npt",
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "calendars",
				"--max", "1",
				"--page", "p1",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Calendars []struct {
			ID string `json:"id"`
		} `json:"calendars"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Calendars) != 1 || parsed.Calendars[0].ID != "c1" || parsed.NextPageToken != "npt" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_CalendarCalendars_AllPages_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	page1Calls := 0
	page2Calls := 0
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("maxResults"); got != "1" {
			t.Fatalf("maxResults=%q", got)
		}
		switch strings.TrimSpace(r.URL.Query().Get("pageToken")) {
		case "":
			page1Calls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "One", "accessRole": "owner"},
				},
				"nextPageToken": "p2",
			})
		case "p2":
			page2Calls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c2", "summary": "Two", "accessRole": "reader"},
				},
				"nextPageToken": "",
			})
		default:
			t.Fatalf("unexpected pageToken=%q", r.URL.Query().Get("pageToken"))
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "calendars",
				"--max", "1",
				"--all",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Calendars []struct {
			ID string `json:"id"`
		} `json:"calendars"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Calendars) != 2 || parsed.Calendars[0].ID != "c1" || parsed.Calendars[1].ID != "c2" || parsed.NextPageToken != "" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
	if page1Calls != 1 || page2Calls != 1 {
		t.Fatalf("unexpected page call counts: page1=%d page2=%d", page1Calls, page2Calls)
	}
}

func TestExecute_CalendarCalendars_FailEmpty_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":         []map[string]any{},
			"nextPageToken": "",
		})
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

	var execErr error
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "calendars",
				"--fail-empty",
			})
		})
	})
	if execErr == nil {
		t.Fatalf("expected error")
	}
	if got := ExitCode(execErr); got != emptyResultsExitCode {
		t.Fatalf("expected exit code %d, got %d", emptyResultsExitCode, got)
	}

	var parsed struct {
		Calendars []struct {
			ID string `json:"id"`
		} `json:"calendars"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Calendars) != 0 || parsed.NextPageToken != "" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}

func TestExecute_CalendarAcl_MaxAndPage_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "/calendars/c1@example.com/acl") && r.Method == http.MethodGet) {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("maxResults"); got != "2" {
			t.Fatalf("maxResults=%q", got)
		}
		if got := r.URL.Query().Get("pageToken"); got != "p2" {
			t.Fatalf("pageToken=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"role": "reader", "scope": map[string]any{"type": "user", "value": "a@b.com"}},
			},
			"nextPageToken": "npt2",
		})
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "acl", "c1@example.com",
				"--max", "2",
				"--page", "p2",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		NextPageToken string `json:"nextPageToken"`
		Rules         []struct {
			Role string `json:"role"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Rules) != 1 || parsed.Rules[0].Role != "reader" || parsed.NextPageToken != "npt2" {
		t.Fatalf("unexpected payload: %#v", parsed)
	}
}
