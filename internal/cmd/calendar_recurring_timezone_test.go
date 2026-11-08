package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

type recurringCreateResult struct {
	event   calendar.Event
	inserts int
	err     error
}

func runRecurringCalendarCreate(t *testing.T, calendarTimezone string, args ...string) recurringCreateResult {
	t.Helper()
	return runRecurringCalendarCommand(t, &CalendarCreateCmd{}, calendarTimezone, append([]string{"primary", "--summary", "Standup"}, args...)...)
}

func runRecurringCalendarCommand(t *testing.T, cmd any, calendarTimezone string, args ...string) recurringCreateResult {
	t.Helper()
	var result recurringCreateResult
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/calendars/primary/events"):
			result.inserts++
			_ = json.NewDecoder(r.Body).Decode(&result.event)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ev1"})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/calendars/primary"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "primary", "timeZone": calendarTimezone})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := withCalendarTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), newCalendarServiceFromServer(t, srv))
	result.err = runKong(t, cmd, args, ctx, &RootFlags{Account: "a@b.com"})
	return result
}

func assertRecurringZones(t *testing.T, got calendar.Event, wantStart, wantEnd string) {
	t.Helper()
	if got.Start == nil || got.Start.TimeZone != wantStart {
		t.Fatalf("start = %#v, want timeZone %q", got.Start, wantStart)
	}
	if got.End == nil || got.End.TimeZone != wantEnd {
		t.Fatalf("end = %#v, want timeZone %q", got.End, wantEnd)
	}
}

func TestCalendarCreateCmd_RecurringOffsetUsesMatchingCalendarTimezone(t *testing.T) {
	// -08:00 is Pacific standard time in November. A fixed Etc/GMT+8 zone would
	// expand the series an hour later in local time once daylight saving starts.
	result := runRecurringCalendarCreate(t, "America/Los_Angeles",
		"--from", "2026-11-10T09:00:00-08:00",
		"--to", "2026-11-10T09:30:00-08:00",
		"--rrule", "RRULE:FREQ=WEEKLY;BYDAY=TU")
	if result.err != nil {
		t.Fatalf("create: %v", result.err)
	}
	assertRecurringZones(t, result.event, "America/Los_Angeles", "America/Los_Angeles")
	if result.event.Start.DateTime != "2026-11-10T09:00:00-08:00" {
		t.Fatalf("start dateTime changed: %q", result.event.Start.DateTime)
	}
}

func TestCalendarCreateCmd_RecurringHalfHourOffsetUsesCalendarTimezone(t *testing.T) {
	// No Etc/GMT zone exists for +05:30, so the request used to carry no
	// timeZone and Google rejected it ("Missing time zone definition").
	result := runRecurringCalendarCreate(t, "Asia/Kolkata",
		"--from", "2026-11-10T09:00:00+05:30",
		"--to", "2026-11-10T09:30:00+05:30",
		"--rrule", "RRULE:FREQ=DAILY")
	if result.err != nil {
		t.Fatalf("create: %v", result.err)
	}
	assertRecurringZones(t, result.event, "Asia/Kolkata", "Asia/Kolkata")
}

func TestCalendarCreateCmd_RecurringUnresolvableOffsetIsUsageError(t *testing.T) {
	result := runRecurringCalendarCreate(t, "UTC",
		"--from", "2026-11-10T09:00:00+05:30",
		"--to", "2026-11-10T09:30:00+05:30",
		"--rrule", "RRULE:FREQ=DAILY")
	var exitErr *ExitError
	if !errors.As(result.err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %v, want usage error", result.err)
	}
	if !strings.Contains(result.err.Error(), "--timezone") {
		t.Fatalf("err = %v, want --timezone guidance", result.err)
	}
	if result.inserts != 0 {
		t.Fatalf("inserted %d events before rejecting", result.inserts)
	}
}

func TestCalendarCreateCmd_RecurringKeepsUTCAndExplicitTimezones(t *testing.T) {
	utc := runRecurringCalendarCreate(t, "Europe/London",
		"--from", "2026-11-10T09:00:00Z",
		"--to", "2026-11-10T09:30:00Z",
		"--rrule", "RRULE:FREQ=WEEKLY")
	if utc.err != nil {
		t.Fatalf("create: %v", utc.err)
	}
	assertRecurringZones(t, utc.event, "UTC", "UTC")

	explicit := runRecurringCalendarCreate(t, "America/Los_Angeles",
		"--from", "2026-11-10T09:00:00-08:00",
		"--to", "2026-11-10T09:30:00-08:00",
		"--start-timezone", "America/Tijuana",
		"--rrule", "RRULE:FREQ=WEEKLY")
	if explicit.err != nil {
		t.Fatalf("create: %v", explicit.err)
	}
	assertRecurringZones(t, explicit.event, "America/Tijuana", "America/Los_Angeles")
}

func TestCalendarCreateCmd_SingleOffsetEventKeepsFixedZone(t *testing.T) {
	result := runRecurringCalendarCreate(t, "America/Los_Angeles",
		"--from", "2026-11-10T09:00:00-08:00",
		"--to", "2026-11-10T09:30:00-08:00")
	if result.err != nil {
		t.Fatalf("create: %v", result.err)
	}
	assertRecurringZones(t, result.event, "Etc/GMT+8", "Etc/GMT+8")
}

func TestCalendarFocusTimeCmd_RecurringOffsetUsesMatchingCalendarTimezone(t *testing.T) {
	result := runRecurringCalendarCommand(t, &CalendarFocusTimeCmd{}, "America/Los_Angeles",
		"primary",
		"--from", "2026-11-10T09:00:00-08:00",
		"--to", "2026-11-10T11:00:00-08:00",
		"--rrule", "RRULE:FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR")
	if result.err != nil {
		t.Fatalf("focus-time: %v", result.err)
	}
	assertRecurringZones(t, result.event, "America/Los_Angeles", "America/Los_Angeles")
}

func TestCalendarCreateCmd_DryRunDeclaresRecurringTimezoneLookup(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "calendar", "create", "primary",
		"--summary", "Standup",
		"--from", "2026-11-10T09:00:00-08:00",
		"--to", "2026-11-10T09:30:00-08:00",
		"--rrule", "RRULE:FREQ=WEEKLY",
	}, nil)
	if result.err != nil {
		t.Fatalf("dry-run: %v", result.err)
	}
	var got struct {
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("json: %v (%q)", err, result.stdout)
	}
	lookup, ok := got.Request["recurring_timezone_lookup"].(map[string]any)
	if !ok {
		t.Fatalf("request = %#v, want recurring_timezone_lookup", got.Request)
	}
	start, _ := lookup["start"].(map[string]any)
	if start["offset"] != "-08:00" || start["fallback_timezone"] != "Etc/GMT+8" {
		t.Fatalf("start lookup = %#v", start)
	}

	single := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "calendar", "create", "primary",
		"--summary", "Standup",
		"--from", "2026-11-10T09:00:00-08:00",
		"--to", "2026-11-10T09:30:00-08:00",
		"--timezone", "America/Los_Angeles",
		"--rrule", "RRULE:FREQ=WEEKLY",
	}, nil)
	if single.err != nil {
		t.Fatalf("dry-run: %v", single.err)
	}
	if strings.Contains(single.stdout, "recurring_timezone_lookup") {
		t.Fatalf("explicit --timezone should need no lookup: %s", single.stdout)
	}
}

func TestCalendarFocusTimeCmd_DryRunDeclaresRecurringTimezoneLookup(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "calendar", "focus-time", "primary",
		"--from", "2026-11-10T09:00:00+05:30",
		"--to", "2026-11-10T11:00:00+05:30",
		"--rrule", "RRULE:FREQ=DAILY",
	}, nil)
	if result.err != nil {
		t.Fatalf("dry-run: %v", result.err)
	}
	if !strings.Contains(result.stdout, `"recurring_timezone_lookup"`) || !strings.Contains(result.stdout, `"no time zone is sent"`) {
		t.Fatalf("dry-run output lacks the timezone lookup: %s", result.stdout)
	}
}
