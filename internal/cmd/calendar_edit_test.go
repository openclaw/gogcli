package cmd

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func parseKongContext(t *testing.T, cmd any, args []string) *kong.Context {
	t.Helper()

	parser, err := kong.New(
		cmd,
		kong.Vars(kong.Vars{
			"auth_services": googleauth.UserServiceCSV(),
		}),
		kong.Writers(io.Discard, io.Discard),
	)
	if err != nil {
		t.Fatalf("kong new: %v", err)
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("kong parse: %v", err)
	}

	return kctx
}

func hasForceSendField(fields []string, field string) bool {
	for _, f := range fields {
		if f == field {
			return true
		}
	}
	return false
}

func TestCalendarUpdatePatchClearsRecurrence(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{"cal1", "evt1", "--rrule", " "})

	patch, _, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if patch == nil {
		t.Fatal("expected patch")
		return
	}
	if patch.Recurrence == nil || len(patch.Recurrence) != 0 {
		t.Fatalf("expected empty recurrence, got %#v", patch.Recurrence)
	}
	if !hasForceSendField(patch.ForceSendFields, "Recurrence") {
		t.Fatalf("expected Recurrence in ForceSendFields")
	}
}

func TestCalendarUpdatePatchClearsReminders(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{"cal1", "evt1", "--reminder", " "})

	patch, _, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if patch == nil {
		t.Fatal("expected patch")
		return
	}
	if patch.Reminders == nil || !patch.Reminders.UseDefault {
		t.Fatalf("expected reminders.UseDefault=true, got %#v", patch.Reminders)
	}
	encoded, err := json.Marshal(patch.Reminders)
	if err != nil {
		t.Fatalf("marshal reminder patch: %v", err)
	}
	if string(encoded) != `{"overrides":null,"useDefault":true}` {
		t.Fatalf("restore-default reminder payload = %s", encoded)
	}
	if !hasForceSendField(patch.ForceSendFields, "Reminders") {
		t.Fatalf("expected Reminders in ForceSendFields")
	}
}

func TestCalendarUpdatePatchClearsPreviousTimeRepresentation(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantStart string
		wantEnd   string
		wantNulls []string
	}{
		{
			name:      "timed to all-day",
			args:      []string{"cal1", "evt1", "--from", "2030-01-01", "--to", "2030-01-02", "--all-day"},
			wantStart: "2030-01-01",
			wantEnd:   "2030-01-02",
			wantNulls: []string{"DateTime", "TimeZone"},
		},
		{
			name:      "all-day to timed",
			args:      []string{"cal1", "evt1", "--from", "2030-01-01T09:00:00Z", "--to", "2030-01-01T10:00:00Z"},
			wantStart: "2030-01-01T09:00:00Z",
			wantEnd:   "2030-01-01T10:00:00Z",
			wantNulls: []string{"Date"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &CalendarUpdateCmd{}
			kctx := parseKongContext(t, cmd, test.args)
			patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
			if err != nil {
				t.Fatalf("buildUpdatePatch: %v", err)
			}
			if !changed || patch.Start == nil || patch.End == nil {
				t.Fatalf("unexpected time patch: changed=%t patch=%#v", changed, patch)
			}

			if cmd.AllDay {
				if patch.Start.Date != test.wantStart || patch.End.Date != test.wantEnd {
					t.Fatalf("unexpected all-day range: start=%#v end=%#v", patch.Start, patch.End)
				}
			} else if patch.Start.DateTime != test.wantStart || patch.End.DateTime != test.wantEnd {
				t.Fatalf("unexpected timed range: start=%#v end=%#v", patch.Start, patch.End)
			}

			for _, field := range test.wantNulls {
				if !hasStringValue(patch.Start.NullFields, field) || !hasStringValue(patch.End.NullFields, field) {
					t.Fatalf("expected %s to be cleared: start=%#v end=%#v", field, patch.Start, patch.End)
				}
			}
		})
	}
}

func TestCalendarUpdatePatchKeepsTimeClearsWhenAddingRecurrence(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{
		"cal1", "evt1",
		"--from", "2030-01-01",
		"--to", "2030-01-02",
		"--all-day",
		"--rrule", "RRULE:FREQ=DAILY",
	})
	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if !changed || len(patch.Recurrence) != 1 {
		t.Fatalf("unexpected recurring patch: changed=%t patch=%#v", changed, patch)
	}

	fallbackStart := &calendar.EventDateTime{DateTime: "2029-12-31T09:00:00Z", TimeZone: "UTC"}
	fallbackEnd := &calendar.EventDateTime{DateTime: "2029-12-31T10:00:00Z", TimeZone: "UTC"}
	patch.Start = normalizeRecurringPatchDateTime(patch.Start, fallbackStart)
	patch.End = normalizeRecurringPatchDateTime(patch.End, fallbackEnd)

	for _, field := range []string{"DateTime", "TimeZone"} {
		if !hasStringValue(patch.Start.NullFields, field) || !hasStringValue(patch.End.NullFields, field) {
			t.Fatalf("expected %s to stay cleared: start=%#v end=%#v", field, patch.Start, patch.End)
		}
	}
}

func TestCalendarUpdatePatchDisablesReminders(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{"cal1", "evt1", "--no-reminders"})

	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if !changed || patch.Reminders == nil || patch.Reminders.UseDefault || patch.Reminders.Overrides == nil {
		t.Fatalf("expected explicitly disabled reminders, got changed=%t reminders=%#v", changed, patch.Reminders)
	}
	for _, field := range []string{"UseDefault", "Overrides"} {
		if !hasForceSendField(patch.Reminders.ForceSendFields, field) {
			t.Fatalf("expected %s in reminders ForceSendFields", field)
		}
	}
}

func TestCalendarReminderFlagsAreMutuallyExclusive(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{
			name: "create empty reminder",
			args: []string{"--dry-run", "calendar", "create", "primary", "--summary", "test", "--from", "2030-01-01", "--to", "2030-01-02", "--all-day", "--reminder=", "--no-reminders"},
		},
		{
			name: "update empty reminder",
			args: []string{"--dry-run", "calendar", "update", "primary", "event", "--reminder=", "--no-reminders"},
		},
		{
			name: "update custom reminder",
			args: []string{"--dry-run", "calendar", "update", "primary", "event", "--reminder", "popup:30m", "--no-reminders"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := executeWithTestRuntime(t, test.args, nil)
			if result.err == nil || !strings.Contains(result.err.Error(), "can't be used together") {
				t.Fatalf("expected conflicting reminder flags, got %v", result.err)
			}
			if code := ExitCode(result.err); code != 2 {
				t.Fatalf("expected usage exit code 2, got %d", code)
			}
		})
	}
}

func TestCalendarUpdatePatchExplicitTimezones(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{
		"cal1",
		"evt1",
		"--from", "2026-08-13T13:40:00+02:00",
		"--to", "2026-08-13T17:00:00-04:00",
		"--start-timezone", "Europe/Rome",
		"--end-timezone", "America/New_York",
	})

	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed patch")
	}
	if patch.Start == nil || patch.Start.TimeZone != "Europe/Rome" {
		t.Fatalf("expected start timezone Europe/Rome, got %#v", patch.Start)
	}
	if patch.End == nil || patch.End.TimeZone != "America/New_York" {
		t.Fatalf("expected end timezone America/New_York, got %#v", patch.End)
	}
}

func TestCalendarUpdatePatchTimezoneRequiresTimeField(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	kctx := parseKongContext(t, cmd, []string{
		"cal1",
		"evt1",
		"--start-timezone", "Europe/Rome",
	})

	if _, _, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx)); err == nil {
		t.Fatalf("expected --start-timezone without --from to fail")
	}
}
