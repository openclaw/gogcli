package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
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
				"nextPageToken": "cal1-next",
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

	ctx := newCmdJSONContext(t)

	jsonOut := captureStdout(t, func() {
		if runErr := listAllCalendarsEvents(ctx, svc, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "", "", nil); runErr != nil {
			t.Fatalf("listAllCalendarsEvents: %v", runErr)
		}
	})

	var parsed struct {
		Events         []map[string]any         `json:"events"`
		NextPageTokens []calendarEventsNextPage `json:"nextPageTokens"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("unexpected events: %#v", parsed.Events)
	}
	if len(parsed.NextPageTokens) != 1 || parsed.NextPageTokens[0].CalendarID != "cal1" || parsed.NextPageTokens[0].NextPageToken != "cal1-next" {
		t.Fatalf("unexpected nextPageTokens: %#v", parsed.NextPageTokens)
	}
}

// TestListAllCalendarsEvents_SortByStart verifies that --sort=start orders
// events from multiple calendars chronologically (default API order returns
// them grouped per calendar in iteration order).
func TestListAllCalendarsEvents_SortByStart(t *testing.T) {
	svc, closeSvc := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendarList") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "cal1"}, {"id": "cal2"}},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "late", "summary": "Late", "status": "confirmed",
						"start": map[string]any{"dateTime": "2025-01-01T15:00:00Z"},
						"end":   map[string]any{"dateTime": "2025-01-01T16:00:00Z"},
					},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/cal2/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id": "early", "summary": "Early", "status": "confirmed",
						"start": map[string]any{"dateTime": "2025-01-01T08:00:00Z"},
						"end":   map[string]any{"dateTime": "2025-01-01T09:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer closeSvc()

	ctx := newCmdJSONContext(t)
	jsonOut := captureStdout(t, func() {
		if err := listAllCalendarsEvents(ctx, svc, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "start", "asc", nil); err != nil {
			t.Fatalf("listAllCalendarsEvents: %v", err)
		}
	})

	var parsed struct {
		Events         []map[string]any         `json:"events"`
		NextPageTokens []calendarEventsNextPage `json:"nextPageTokens"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("expected 2 events, got %#v", parsed.Events)
	}
	if len(parsed.NextPageTokens) != 0 {
		t.Fatalf("unexpected nextPageTokens: %#v", parsed.NextPageTokens)
	}
	if got, _ := parsed.Events[0]["id"].(string); got != "early" {
		t.Fatalf("expected first event id 'early', got %q (events: %#v)", got, parsed.Events)
	}
	if got, _ := parsed.Events[1]["id"].(string); got != "late" {
		t.Fatalf("expected second event id 'late', got %q (events: %#v)", got, parsed.Events)
	}

	// Descending order flips it.
	jsonOut = captureStdout(t, func() {
		if err := listAllCalendarsEvents(ctx, svc, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "start", "desc", nil); err != nil {
			t.Fatalf("listAllCalendarsEvents desc: %v", err)
		}
	})
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse desc: %v", err)
	}
	if got, _ := parsed.Events[0]["id"].(string); got != "late" {
		t.Fatalf("desc: expected first id 'late', got %q", got)
	}
}

// TestSortEventsBy_Summary verifies case-insensitive summary sort works on
// the wrapper slice independent of API call wiring.
func TestSortEventsBy_Summary(t *testing.T) {
	events := []*eventWithCalendar{
		{Event: &calendar.Event{Summary: "banana"}},
		{Event: &calendar.Event{Summary: "Apple"}},
		{Event: &calendar.Event{Summary: "cherry"}},
	}
	sortEventsBy(events, "summary", "asc")
	got := []string{events[0].Summary, events[1].Summary, events[2].Summary}
	want := []string{"Apple", "banana", "cherry"}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("summary asc: got %v want %v", got, want)
		}
	}

	sortEventsBy(events, "summary", "desc")
	got = []string{events[0].Summary, events[1].Summary, events[2].Summary}
	want = []string{"cherry", "banana", "Apple"}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("summary desc: got %v want %v", got, want)
		}
	}
}
