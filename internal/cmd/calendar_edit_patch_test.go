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

	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
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

func TestCalendarUpdateBuildPatchResourceAttendee(t *testing.T) {
	cmd := &CalendarUpdateCmd{}
	parser, err := kong.New(cmd, kong.Writers(io.Discard, io.Discard))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	kctx, err := parser.Parse([]string{
		"cal1",
		"evt1",
		"--attendees", "room@resource.calendar.google.com;resource;optional;comment=Project room",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
	if err != nil {
		t.Fatalf("buildUpdatePatch: %v", err)
	}
	if !changed || len(patch.Attendees) != 1 {
		t.Fatalf("unexpected attendees patch: %#v", patch)
	}
	attendee := patch.Attendees[0]
	if attendee.Email != "room@resource.calendar.google.com" || !attendee.Resource || !attendee.Optional || attendee.Comment != "Project room" {
		t.Fatalf("unexpected resource attendee: %#v", attendee)
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

	patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
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

func TestCalendarUpdateBuildPatch_Attachments(t *testing.T) {
	t.Run("replace", func(t *testing.T) {
		cmd := &CalendarUpdateCmd{}
		parser, err := kong.New(cmd, kong.Writers(io.Discard, io.Discard))
		if err != nil {
			t.Fatalf("kong.New: %v", err)
		}
		kctx, err := parser.Parse([]string{
			"cal1",
			"evt1",
			"--attachment", " https://drive.google.com/file/d/one ",
			"--attachment", "https://drive.google.com/file/d/two",
		})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}

		patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
		if err != nil {
			t.Fatalf("buildUpdatePatch: %v", err)
		}
		if !changed || len(patch.Attachments) != 2 {
			t.Fatalf("unexpected attachment patch: %#v", patch)
		}
		if patch.Attachments[0].FileUrl != "https://drive.google.com/file/d/one" ||
			patch.Attachments[1].FileUrl != "https://drive.google.com/file/d/two" {
			t.Fatalf("unexpected attachments: %#v", patch.Attachments)
		}
		if !patchHasAttachmentsMutation(patch) {
			t.Fatal("expected attachment mutation")
		}
	})

	t.Run("clear", func(t *testing.T) {
		cmd := &CalendarUpdateCmd{}
		parser, err := kong.New(cmd, kong.Writers(io.Discard, io.Discard))
		if err != nil {
			t.Fatalf("kong.New: %v", err)
		}
		kctx, err := parser.Parse([]string{"cal1", "evt1", "--attachment="})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}

		patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
		if err != nil {
			t.Fatalf("buildUpdatePatch: %v", err)
		}
		if !changed || patch.Attachments == nil || len(patch.Attachments) != 0 {
			t.Fatalf("unexpected clear patch: %#v", patch)
		}
		if !hasForceSendField(patch.ForceSendFields, "Attachments") {
			t.Fatalf("expected Attachments force-send field: %#v", patch.ForceSendFields)
		}
		if !patchHasAttachmentsMutation(patch) {
			t.Fatal("expected clear attachment mutation")
		}
	})
}
