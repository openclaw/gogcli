package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestListAllCalendarsEvents_JSON(t *testing.T) {
	svc, closeSvc := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendarList") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "cal1"},
					{"id": "cal2"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":          "e1",
						"summary":     "Event 1",
						"description": "Desc",
						"location":    "Room",
						"status":      "confirmed",
						"start":       map[string]any{"dateTime": "2025-01-01T10:00:00Z"},
						"end":         map[string]any{"dateTime": "2025-01-01T11:00:00Z"},
						"attendees":   []map[string]any{{"email": "a@example.com"}},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/cal2/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "e2",
						"summary": "Event 2",
						"status":  "confirmed",
						"start":   map[string]any{"dateTime": "2025-01-01T09:00:00Z"},
						"end":     map[string]any{"dateTime": "2025-01-01T09:30:00Z"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})))
	defer closeSvc()

	ctx := newCalendarJSONContext(t)

	jsonOut := captureStdout(t, func() {
		if runErr := listAllCalendarsEvents(ctx, svc, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", false); runErr != nil {
			t.Fatalf("listAllCalendarsEvents: %v", runErr)
		}
	})

	var parsed struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("unexpected events: %#v", parsed.Events)
	}
}
