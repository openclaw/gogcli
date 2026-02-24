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

func TestCalendarMoreCommands_JSON(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
		switch {
		case path == "/users/me/calendarList" && r.Method == http.MethodGet:
			// Simulate paging: first response returns a page token, second ends.
			if r.URL.Query().Get("pageToken") != "" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"items": []map[string]any{},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "cal1", "summary": "Primary", "accessRole": "owner"},
				},
				"nextPageToken": "npt",
			})
			return
		case strings.Contains(path, "/calendars/cal1/acl") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"role": "reader", "scope": map[string]any{"type": "user", "value": "a@b.com"}},
				},
			})
			return
		case strings.Contains(path, "/calendars/cal1/events/evt1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "evt1",
				"summary":  "Meeting",
				"start":    map[string]any{"dateTime": "2025-01-01T10:00:00Z"},
				"end":      map[string]any{"dateTime": "2025-01-01T11:00:00Z"},
				"htmlLink": "http://example.com/event",
			})
			return
		case strings.Contains(path, "/calendars/cal1/events") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "evt2",
				"summary":  "Created",
				"htmlLink": "http://example.com/created",
			})
			return
		case strings.Contains(strings.ToLower(path), "freebusy") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"calendars": map[string]any{
					"cal1": map[string]any{
						"busy": []map[string]any{{"start": "2025-01-01T10:00:00Z", "end": "2025-01-01T11:00:00Z"}},
					},
				},
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

	flags := &RootFlags{Account: "a@b.com", Force: true}
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarCalendarsCmd{}, []string{}, ctx, flags); err != nil {
			t.Fatalf("calendars: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarAclCmd{}, []string{"cal1"}, ctx, flags); err != nil {
			t.Fatalf("acl: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarEventCmd{}, []string{"cal1", "evt1"}, ctx, flags); err != nil {
			t.Fatalf("event: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarCreateCmd{}, []string{"cal1", "--summary", "Created", "--from", "2025-01-01T12:00:00Z", "--to", "2025-01-01T13:00:00Z"}, ctx, flags); err != nil {
			t.Fatalf("create: %v", err)
		}
	})

	_ = captureStdout(t, func() {
		if err := runKong(t, &CalendarFreeBusyCmd{}, []string{"cal1", "--from", "2025-01-01T00:00:00Z", "--to", "2025-01-02T00:00:00Z"}, ctx, flags); err != nil {
			t.Fatalf("freebusy: %v", err)
		}
	})
}
