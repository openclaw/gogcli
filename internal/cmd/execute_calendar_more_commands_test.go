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

func TestExecute_CalendarMoreCommands_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	const calendarID = "c1@example.com"
	const eventID = "e1"

	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/calendars/"+calendarID+"/acl") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"role": "owner", "scope": map[string]any{"type": "user", "value": "a@b.com"}},
				},
			})
			return
		case strings.Contains(path, "/calendars/"+calendarID+"/events/"+eventID) && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      eventID,
				"summary": "Hello",
				"start":   map[string]any{"dateTime": "2025-12-17T10:00:00Z"},
				"end":     map[string]any{"dateTime": "2025-12-17T11:00:00Z"},
			})
			return
		case strings.Contains(path, "/calendars/"+calendarID+"/events/"+eventID) && (r.Method == http.MethodPatch || r.Method == http.MethodPut):
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			payload["id"] = eventID
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		case strings.Contains(path, "/calendars/"+calendarID+"/events/"+eventID) && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case strings.Contains(path, "/calendars/"+calendarID+"/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e0", "summary": "S", "start": map[string]any{"dateTime": "2025-12-17T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-12-17T11:00:00Z"}},
				},
				"nextPageToken": "npt",
			})
			return
		case strings.Contains(path, "/calendars/"+calendarID+"/events") && r.Method == http.MethodPost:
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			payload["id"] = eventID
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
			return
		case strings.Contains(path, "/freeBusy") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"c1": map[string]any{
						"busy": []map[string]any{{"start": "2025-12-17T10:00:00Z", "end": "2025-12-17T11:00:00Z"}},
					},
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
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "acl", calendarID}); err != nil {
				t.Fatalf("acl: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "events", calendarID, "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z", "--query", "hello"}); err != nil {
				t.Fatalf("events: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "event", calendarID, eventID}); err != nil {
				t.Fatalf("event: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "create", calendarID, "--summary", "S", "--from", "2025-12-17T10:00:00Z", "--to", "2025-12-17T11:00:00Z"}); err != nil {
				t.Fatalf("create: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "update", calendarID, eventID, "--summary", "S2"}); err != nil {
				t.Fatalf("update: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--force", "--account", "a@b.com", "calendar", "delete", calendarID, eventID}); err != nil {
				t.Fatalf("delete: %v", err)
			}
		})
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--json", "--account", "a@b.com", "calendar", "freebusy", "c1", "--from", "2025-12-17T00:00:00Z", "--to", "2025-12-18T00:00:00Z"}); err != nil {
				t.Fatalf("freebusy: %v", err)
			}
		})
	})
}
