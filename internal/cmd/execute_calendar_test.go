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

func stubCalendarExecuteService(t *testing.T, handler http.Handler) {
	t.Helper()

	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	svc, err := calendar.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }
}

func executeWithCapturedOutput(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var execErr error

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			execErr = Execute(args)
		})
	})

	return out, execErr
}

func TestExecute_CalendarCalendars_JSON(t *testing.T) {
	stubCalendarExecuteService(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))

	out, err := executeWithCapturedOutput(t, "--json", "--account", "a@b.com", "calendar", "calendars")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var parsed struct {
		Calendars []struct {
			ID         string `json:"id"`
			Summary    string `json:"summary"`
			AccessRole string `json:"accessRole"`
		} `json:"calendars"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if len(parsed.Calendars) != 2 || parsed.Calendars[0].ID != "c1" || parsed.Calendars[1].ID != "c2" {
		t.Fatalf("unexpected calendars: %#v", parsed.Calendars)
	}
}

func TestExecute_CalendarSubscribe_JSON(t *testing.T) {
	stubCalendarExecuteService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         req["id"],
			"summary":    "Test Calendar",
			"accessRole": "reader",
		})
	}))

	out, err := executeWithCapturedOutput(t, "--json", "--account", "a@b.com", "calendar", "subscribe", "test@example.com")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var parsed struct {
		Calendar struct {
			ID         string `json:"id"`
			Summary    string `json:"summary"`
			AccessRole string `json:"accessRole"`
		} `json:"calendar"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Calendar.ID != "test@example.com" {
		t.Fatalf("unexpected calendar id: %s", parsed.Calendar.ID)
	}
	if parsed.Calendar.AccessRole != "reader" {
		t.Fatalf("unexpected access role: %s", parsed.Calendar.AccessRole)
	}
}

func TestExecute_CalendarSubscribe_Flags(t *testing.T) {
	stubCalendarExecuteService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}

		var req struct {
			ID       string `json:"id"`
			ColorID  string `json:"colorId"`
			Hidden   bool   `json:"hidden"`
			Selected bool   `json:"selected"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID != "team@example.com" {
			t.Fatalf("unexpected calendar id: %q", req.ID)
		}
		if req.ColorID != "24" {
			t.Fatalf("unexpected color id: %q", req.ColorID)
		}
		if !req.Hidden {
			t.Fatalf("expected hidden=true")
		}
		if req.Selected {
			t.Fatalf("expected selected=false")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         req.ID,
			"summary":    "Team Calendar",
			"accessRole": "reader",
		})
	}))

	if _, err := executeWithCapturedOutput(t, "--account", "a@b.com", "calendar", "subscribe", "--color-id", "24", "--hidden", "--no-selected", "team@example.com"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecute_CalendarSubscribe_MissingCalendarID(t *testing.T) {
	_, err := executeWithCapturedOutput(t, "--account", "a@b.com", "calendar", "subscribe")
	if err == nil || !strings.Contains(err.Error(), "<calendarId>") {
		t.Fatalf("expected missing calendarId error, got %v", err)
	}
}

func TestExecute_CalendarSubscribe_InvalidColor(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	newCalendarService = func(context.Context, string) (*calendar.Service, error) {
		t.Fatalf("newCalendarService should not be called for invalid color")
		return &calendar.Service{}, nil
	}

	_, err := executeWithCapturedOutput(t, "--account", "a@b.com", "calendar", "subscribe", "--color-id", "25", "team@example.com")
	if err == nil || !strings.Contains(err.Error(), "calendar color ID must be 1-24") {
		t.Fatalf("expected invalid color error, got %v", err)
	}
}

func TestExecute_CalendarSubscribe_APIFailure(t *testing.T) {
	stubCalendarExecuteService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}

		http.Error(w, "denied", http.StatusForbidden)
	}))

	_, err := executeWithCapturedOutput(t, "--account", "a@b.com", "calendar", "subscribe", "team@example.com")
	if err == nil || !strings.Contains(err.Error(), "HTTP response code 403") {
		t.Fatalf("expected API error, got %v", err)
	}
}

func TestExecute_CalendarSubscribe_Text(t *testing.T) {
	stubCalendarExecuteService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !(strings.Contains(r.URL.Path, "calendarList") && r.Method == http.MethodPost) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":         "user@example.com",
			"summary":    "User Calendar",
			"accessRole": "writer",
		})
	}))

	out, err := executeWithCapturedOutput(t, "--account", "a@b.com", "calendar", "subscribe", "user@example.com")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out, "user@example.com") {
		t.Fatalf("expected calendar id in output: %s", out)
	}
	if !strings.Contains(out, "writer") {
		t.Fatalf("expected access role in output: %s", out)
	}
}
