package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestListAllCalendarsEvents_JSON(t *testing.T) {
	srv := httptest.NewServer(withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if err := listAllCalendarsEvents(ctx, svc, "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", "", "", "", "", false); err != nil {
			t.Fatalf("listAllCalendarsEvents: %v", err)
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
