package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/config"
)

func setupCalendarAliasHome(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
}

func TestCalendarEventCmd_UsesResolvedAliasID(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	setupCalendarAliasHome(t)
	if err := config.SetCalendarAlias("family", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("SetCalendarAlias: %v", err)
	}

	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/calendars/family-cal%40group.calendar.google.com/events/evt1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "evt1",
				"summary": "Family event",
				"start":   map[string]any{"dateTime": "2025-01-01T10:00:00Z"},
				"end":     map[string]any{"dateTime": "2025-01-01T11:00:00Z"},
			})
			return
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/calendars/family-cal%40group.calendar.google.com":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "family-cal@group.calendar.google.com",
				"timeZone": "UTC",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	ctx := newCalendarJSONContext(t)
	out := captureStdout(t, func() {
		if err := runKong(t, &CalendarEventCmd{}, []string{"family", "evt1"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("runKong: %v", err)
		}
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	event, ok := got["event"].(map[string]any)
	if !ok || event["id"] != "evt1" {
		t.Fatalf("unexpected output: %#v", got)
	}
}

func TestCalendarEventsCmd_CalInput_UsesResolvedAliasID(t *testing.T) {
	origNew := newCalendarService
	t.Cleanup(func() { newCalendarService = origNew })

	setupCalendarAliasHome(t)
	if err := config.SetCalendarAlias("family", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("SetCalendarAlias: %v", err)
	}

	svc, closeSvc := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.EscapedPath(), "/users/me/calendarList") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "family-cal@group.calendar.google.com",
						"summary": "Family",
					},
				},
			})
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.EscapedPath(), "/calendars/family-cal%40group.calendar.google.com/events") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "evt1",
						"summary": "Family event",
						"start":   map[string]any{"dateTime": "2025-01-01T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2025-01-01T11:00:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer closeSvc()
	newCalendarService = func(context.Context, string) (*calendar.Service, error) { return svc, nil }

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--account", "a@b.com",
				"calendar", "events",
				"--cal", "family",
				"--from", "2025-01-01T00:00:00Z",
				"--to", "2025-01-02T00:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	events, ok := got["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("unexpected output: %#v", got)
	}
}

func TestCalendarCreateCmd_DryRun_UsesResolvedAliasID(t *testing.T) {
	setupCalendarAliasHome(t)
	if err := config.SetCalendarAlias("family", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("SetCalendarAlias: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--dry-run",
				"calendar", "create",
				"family",
				"--summary", "Meeting",
				"--from", "2025-01-01T10:00:00Z",
				"--to", "2025-01-01T11:00:00Z",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	request, ok := got["request"].(map[string]any)
	if !ok {
		t.Fatalf("request = %#v", got["request"])
	}
	if request["calendar_id"] != "family-cal@group.calendar.google.com" {
		t.Fatalf("request.calendar_id = %v", request["calendar_id"])
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %v", got["dry_run"])
	}
}
