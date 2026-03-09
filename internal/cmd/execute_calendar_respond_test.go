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

func TestExecute_CalendarRespond_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	const calendarID = "c1@example.com"
	const eventID = "e1"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/calendars/"+calendarID+"/events/"+eventID) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": eventID,
				"attendees": []map[string]any{
					{"email": "a@b.com", "self": true, "responseStatus": "needsAction"},
				},
			})
			return
		case http.MethodPatch:
			if got := r.URL.Query().Get("sendUpdates"); got != "" {
				t.Fatalf("expected no sendUpdates by default, got %q", got)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode: %v", err)
			}
			attendees, _ := payload["attendees"].([]any)
			if len(attendees) != 1 {
				t.Fatalf("unexpected attendees: %#v", attendees)
			}
			first, _ := attendees[0].(map[string]any)
			if first["responseStatus"] != "accepted" {
				t.Fatalf("expected accepted, got %#v", first["responseStatus"])
			}
			// Calendar API returns the updated event; don't echo the patch payload (which may be partial).
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":        eventID,
				"attendees": payload["attendees"],
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

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "respond",
				calendarID, eventID,
				"--status", "accepted",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var parsed struct {
		Event struct {
			ID        string `json:"id"`
			Attendees []struct {
				ResponseStatus string `json:"responseStatus"`
			} `json:"attendees"`
		} `json:"event"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v\nout=%q", err, out)
	}
	if parsed.Event.ID != eventID || len(parsed.Event.Attendees) != 1 || parsed.Event.Attendees[0].ResponseStatus != "accepted" {
		t.Fatalf("unexpected event: %#v", parsed.Event)
	}
}
