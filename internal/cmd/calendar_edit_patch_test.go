package cmd

import (
	"io"
	"testing"

	"github.com/alecthomas/kong"
)

func TestCalendarUpdateBuildPatch(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	parser, err := kong.New(cmd, kong.Writers(io.Discard, io.Discard))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	kctx, err := parser.Parse([]string{
		"cal1",
		"evt1",
		"--summary", "New Summary",
		"--description", "Desc",
		"--location", "Loc",
		"--from", "2025-01-01",
		"--to", "2025-01-02",
		"--attendees", "a@example.com",
		"--rrule", "RRULE:FREQ=DAILY",
		"--reminder", "popup:30m",
		"--event-color", "1",
		"--visibility", "private",
		"--transparency", "transparent",
		"--guests-can-invite",
		"--guests-can-modify",
		"--guests-can-see-others",
		"--private-prop", "k=v",
		"--shared-prop", "s=v",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	patch, changed, err := cmd.buildUpdatePatch(kctx, nil, false)
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}
	if patch.Summary != "New Summary" || patch.Description != "Desc" || patch.Location != "Loc" {
		t.Fatalf("unexpected patch fields: %#v", patch)
	}
	if patch.Visibility != "private" || patch.Transparency != "transparent" {
		t.Fatalf("unexpected visibility/transparency: %#v", patch)
	}
	if patch.ExtendedProperties == nil {
		t.Fatalf("expected extended properties")
	}
}

func TestCalendarUpdateBuildPatch_ClearFields(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	parser, err := kong.New(cmd, kong.Writers(io.Discard, io.Discard))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	kctx, err := parser.Parse([]string{
		"cal1",
		"evt1",
		"--rrule=",
		"--reminder=",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	patch, changed, err := cmd.buildUpdatePatch(kctx, nil, false)
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed")
	}
	if len(patch.ForceSendFields) == 0 {
		t.Fatalf("expected force send fields")
	}
}
