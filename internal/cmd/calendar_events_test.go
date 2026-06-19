package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestListCalendarEvents_JSON(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event", "start": map[string]any{"dateTime": "2025-01-01T10:00:00Z"}, "end": map[string]any{"dateTime": "2025-01-01T11:00:00Z"}},
				},
				"nextPageToken": "next",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer closeServer()

	var output bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2025-01-01T00:00:00Z", "2025-01-02T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "", ""); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}

	var parsed struct {
		Events []map[string]any `json:"events"`
		Next   string           `json:"nextPageToken"`
	}
	if err := json.Unmarshal(output.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 1 || parsed.Next != "next" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}

func calendarTimezoneEventsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/calendars/cal1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "cal1",
				"timeZone": "Africa/Windhoek",
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":      "e1",
						"summary": "Followup",
						"start":   map[string]any{"dateTime": "2026-04-08T20:00:00+13:00"},
						"end":     map[string]any{"dateTime": "2026-04-08T20:20:00+13:00"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})
}

func TestListCalendarEvents_TableUsesCalendarTimezone(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, calendarTimezoneEventsHandler())
	defer closeServer()

	var output bytes.Buffer
	ctx := newCmdRuntimeOutputContext(t, &output, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-04-08T00:00:00Z", "2026-04-09T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "", ""); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}
	text := output.String()

	if !strings.Contains(text, "2026-04-08T09:00:00+02:00") || !strings.Contains(text, "2026-04-08T09:20:00+02:00") {
		t.Fatalf("expected calendar-local times, got: %q", text)
	}
	if strings.Contains(text, "2026-04-08T20:00:00+13:00") {
		t.Fatalf("expected raw +13:00 time to be localized, got: %q", text)
	}
}

// TestListCalendarEvents_TableIncludesLocation asserts that the events list
// table renders the LOCATION column when requested and that embedded newlines in
// the location string are collapsed so the row stays on one line.
func TestListCalendarEvents_TableIncludesLocation(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/cal1/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{
						"id":       "e1",
						"summary":  "Standup",
						"location": "Bahnhofstrasse 1\n8001 Zürich",
						"start":    map[string]any{"dateTime": "2026-04-08T09:00:00Z"},
						"end":      map[string]any{"dateTime": "2026-04-08T09:15:00Z"},
					},
					{
						"id":      "e2",
						"summary": "No-location event",
						"start":   map[string]any{"dateTime": "2026-04-08T10:00:00Z"},
						"end":     map[string]any{"dateTime": "2026-04-08T10:15:00Z"},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer closeServer()

	var output bytes.Buffer
	ctx := newCmdRuntimeOutputContext(t, &output, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-04-08T00:00:00Z", "2026-04-09T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "", ""); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}
	text := output.String()

	if strings.Contains(text, "LOCATION") {
		t.Fatalf("did not expect LOCATION header without --location, got: %q", text)
	}

	output.Reset()
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-04-08T00:00:00Z", "2026-04-09T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, true, "", ""); err != nil {
		t.Fatalf("listCalendarEvents with location: %v", err)
	}
	text = output.String()

	if !strings.Contains(text, "LOCATION") {
		t.Fatalf("expected LOCATION header with --location, got: %q", text)
	}
	if !strings.Contains(text, "Bahnhofstrasse 1 8001 Zürich") {
		t.Fatalf("expected collapsed multi-line location, got: %q", text)
	}
	// Original newline must not leak into the rendered row.
	if strings.Contains(text, "Bahnhofstrasse 1\n8001 Zürich") {
		t.Fatalf("expected newline in location to be collapsed, got: %q", text)
	}
}

func TestListCalendarEvents_JSONUsesCalendarTimezoneForLocalFields(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, calendarTimezoneEventsHandler())
	defer closeServer()

	var output bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)
	if err := listCalendarEvents(ctx, svc, "cal1", "2026-04-08T00:00:00Z", "2026-04-09T00:00:00Z", 10, "", false, false, "", "", "", "", nil, false, false, "", ""); err != nil {
		t.Fatalf("listCalendarEvents: %v", err)
	}

	var parsed struct {
		Events []struct {
			Timezone   string `json:"timezone"`
			StartLocal string `json:"startLocal"`
			EndLocal   string `json:"endLocal"`
		} `json:"events"`
	}
	if err := json.Unmarshal(output.Bytes(), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 1 {
		t.Fatalf("unexpected events: %#v", parsed.Events)
	}
	event := parsed.Events[0]
	if event.Timezone != "Africa/Windhoek" || event.StartLocal != "2026-04-08T09:00:00+02:00" || event.EndLocal != "2026-04-08T09:20:00+02:00" {
		t.Fatalf("unexpected localized fields: %#v", event)
	}
}

func TestCalendarEventsCmd_DefaultsToPrimary(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/primary/events") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event"},
				},
				"nextPageToken": "",
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer closeServer()

	var output bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &CalendarEventsCmd{
		From: "2025-01-01T00:00:00Z",
		To:   "2025-01-02T00:00:00Z",
		Max:  10,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := output.String()
	if !strings.Contains(out, "\"events\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCalendarEventsCmd_CalendarsFlag(t *testing.T) {
	var mu sync.Mutex
	calls := make(map[string]int)

	svc, closeServer := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Family"},
					{"id": "c3", "summary": "Other"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c1/events") && r.Method == http.MethodGet:
			mu.Lock()
			calls["c1"]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e1", "summary": "Event 1"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c2/events") && r.Method == http.MethodGet:
			mu.Lock()
			calls["c2"]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "e2", "summary": "Event 2"},
				},
			})
			return
		case strings.Contains(r.URL.Path, "/calendars/c3/events") && r.Method == http.MethodGet:
			mu.Lock()
			calls["c3"]++
			mu.Unlock()
			http.Error(w, "unexpected calendar", http.StatusBadRequest)
			return
		default:
			http.NotFound(w, r)
			return
		}
	})))
	defer closeServer()

	var output bytes.Buffer
	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc)
	flags := &RootFlags{Account: "a@b.com"}

	cmd := &CalendarEventsCmd{
		Calendars: "1,Family",
		From:      "2025-01-01T00:00:00Z",
		To:        "2025-01-02T00:00:00Z",
		Max:       10,
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := output.String()

	var parsed struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if len(parsed.Events) != 2 {
		t.Fatalf("unexpected events: %#v", parsed.Events)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls["c1"] == 0 || calls["c2"] == 0 || calls["c3"] != 0 {
		t.Fatalf("unexpected calendar calls: %#v", calls)
	}
}

func TestCalendarEventsCmd_EventTypesAcrossCalendars(t *testing.T) {
	// The --event-types filter must reach the events.list call for every
	// calendar on the multi-calendar paths (--cal/--calendars and --all), not
	// just the single-calendar path covered by
	// TestCalendarEventsListCall_EventTypesFilter.
	want := []string{eventTypeBirthday, eventTypeDefault}

	var mu sync.Mutex
	captured := make(map[string][]string) // calendarID -> eventTypes query on its events request

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Family"},
					{"id": "c3", "summary": "Other"},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/events") && r.Method == http.MethodGet:
			calID := ""
			parts := strings.Split(r.URL.Path, "/")
			for i, p := range parts {
				if p == "calendars" && i+1 < len(parts) {
					calID = parts[i+1]
					break
				}
			}
			mu.Lock()
			captured[calID] = r.URL.Query()["eventTypes"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	})

	run := func(t *testing.T, mutate func(*CalendarEventsCmd)) map[string][]string {
		t.Helper()
		mu.Lock()
		captured = make(map[string][]string)
		mu.Unlock()
		svc, closeServer := newCalendarServiceForTest(t, withPrimaryCalendar(handler))
		defer closeServer()

		ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &bytes.Buffer{}, io.Discard), svc)
		cmd := &CalendarEventsCmd{
			From:       "2025-01-01T00:00:00Z",
			To:         "2025-01-02T00:00:00Z",
			Max:        10,
			EventTypes: []string{"birthday", "default"},
		}
		mutate(cmd)
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("Run: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()
		return maps.Clone(captured)
	}

	assertReached := func(t *testing.T, got map[string][]string, cals ...string) {
		t.Helper()
		for _, cal := range cals {
			ets, ok := got[cal]
			if !ok {
				t.Fatalf("calendar %s was not queried; captured=%v", cal, got)
			}
			if !slices.Equal(ets, want) {
				t.Fatalf("calendar %s eventTypes = %v, want %v", cal, ets, want)
			}
		}
	}

	t.Run("--calendars", func(t *testing.T) {
		got := run(t, func(c *CalendarEventsCmd) { c.Calendars = "1,Family" })
		assertReached(t, got, "c1", "c2")
	})

	t.Run("--all", func(t *testing.T) {
		got := run(t, func(c *CalendarEventsCmd) { c.All = true })
		assertReached(t, got, "c1", "c2", "c3")
	})
}

func TestCalendarEventsCmd_ListSelectors(t *testing.T) {
	handler := withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "c1", "summary": "Work", "timeZone": "UTC"}},
			})
			return
		case r.URL.Path == "/calendars/c1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "c1", "timeZone": "UTC"})
			return
		case strings.Contains(r.URL.Path, "/calendars/c1/events") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"id": "e1", "summary": "Event"}}})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	for _, tc := range []struct {
		name     string
		selector []string
	}{
		{name: "flag", selector: []string{"--cal", "Work"}},
		{name: "positional", selector: []string{"Work"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, closeServer := newCalendarServiceForTest(t, handler)
			t.Cleanup(closeServer)
			var output bytes.Buffer
			ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, &output, io.Discard), svc)
			args := append([]string{"list"}, tc.selector...)
			args = append(args, "--from", "2025-01-01T00:00:00Z", "--to", "2025-01-02T00:00:00Z")
			if err := runKong(t, &CalendarEventsCmd{}, args, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
				t.Fatalf("calendar events list: %v", err)
			}
			if out := output.String(); !strings.Contains(out, `"events"`) || !strings.Contains(out, `"Event"`) {
				t.Fatalf("unexpected output: %q", out)
			}
		})
	}
}

func TestResolveCalendarIDs_IndexOutOfRange(t *testing.T) {
	svc, closeServer := newCalendarServiceForTest(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer closeServer()

	_, err := resolveCalendarIDs(context.Background(), defaultConfigStoreForTest(t), svc, []string{"2"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestResolveCalendarIDs_AmbiguousName(t *testing.T) {
	svc, closeServer := newTestCalendarService(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Work"},
					{"id": "c3", "summary": "Family"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer closeServer()

	_, err := resolveCalendarIDs(context.Background(), defaultConfigStoreForTest(t), svc, []string{"Work"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestResolveCalendarIDs_UnrecognizedName(t *testing.T) {
	svc, closeServer := newTestCalendarService(t, withPrimaryCalendar(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendarList") &&
			!strings.Contains(r.URL.Path, "/calendarList/primary") &&
			r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{
					{"id": "c1", "summary": "Work"},
					{"id": "c2", "summary": "Family"},
				},
			})
			return
		}
		http.NotFound(w, r)
	})))
	defer closeServer()

	// Test single unrecognized name
	_, err := resolveCalendarIDs(context.Background(), defaultConfigStoreForTest(t), svc, []string{"NonExistent"})
	if err == nil {
		t.Fatalf("expected error for unrecognized calendar name")
	}
	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "unrecognized calendar name(s)") {
		t.Fatalf("expected error message to mention unrecognized calendar, got: %v", err)
	}
	if !strings.Contains(err.Error(), "NonExistent") {
		t.Fatalf("expected error message to include the unrecognized name, got: %v", err)
	}

	// Test multiple unrecognized names
	_, err = resolveCalendarIDs(context.Background(), defaultConfigStoreForTest(t), svc, []string{"Work", "Unknown1", "Unknown2"})
	if err == nil {
		t.Fatalf("expected error for unrecognized calendar names")
	}
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Unknown1") || !strings.Contains(err.Error(), "Unknown2") {
		t.Fatalf("expected error message to include all unrecognized names, got: %v", err)
	}

	// Test valid names still work
	ids, err := resolveCalendarIDs(context.Background(), defaultConfigStoreForTest(t), svc, []string{"Work", "Family"})
	if err != nil {
		t.Fatalf("unexpected error for valid calendar names: %v", err)
	}
	if len(ids) != 2 || ids[0] != "c1" || ids[1] != "c2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}
