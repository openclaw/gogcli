package cmd

import (
	"io"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/googleauth"
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

	patch, _, err := cmd.buildUpdatePatch(kctx)
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

	patch, _, err := cmd.buildUpdatePatch(kctx)
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
	if !hasForceSendField(patch.ForceSendFields, "Reminders") {
		t.Fatalf("expected Reminders in ForceSendFields")
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

	patch, changed, err := cmd.buildUpdatePatch(kctx)
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

	if _, _, err := cmd.buildUpdatePatch(kctx); err == nil {
		t.Fatalf("expected --start-timezone without --from to fail")
	}
}
