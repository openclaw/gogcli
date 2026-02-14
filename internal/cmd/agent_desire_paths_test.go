package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestRootDesirePaths_HelpParses(t *testing.T) {
	tests := [][]string{
		{"send", "--help"},
		{"ls", "--help"},
		{"search", "--help"},
		{"download", "--help"},
		{"upload", "--help"},
		{"open", "--help"},
		{"login", "--help"},
		{"logout", "--help"},
		{"status", "--help"},
		{"me", "--help"},
		{"whoami", "--help"},
		{"exit-codes", "--help"},
		{"agent", "--help"},
	}

	for _, args := range tests {
		args := args
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_ = captureStdout(t, func() {
				_ = captureStderr(t, func() {
					if err := Execute(args); err != nil {
						t.Fatalf("Execute(%v): %v", args, err)
					}
				})
			})
		})
	}
}

func TestDesirePaths_GlobalFlagAliases(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--machine", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected json output with --machine, got: %q", out)
	}

	out = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--tsv", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected text output with --tsv, got: %q", out)
	}
}

func TestDesirePaths_DryRunAlias_ExitsBeforeAuth(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{
				"--json",
				"--dryrun",
				"send",
				"--to", "to@example.com",
				"--subject", "Hello",
				"--body", "Test",
			}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "\"dry_run\": true") {
		t.Fatalf("expected dry-run json output, got: %q", out)
	}
}

func TestDesirePaths_CursorAlias_Parses(t *testing.T) {
	parser, _, err := newParser("test parser")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	if _, err := parser.Parse([]string{"drive", "ls", "--cursor", "tok"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
}

func TestDesirePaths_RewriteFields_KeepsCalendarEventsWithGlobalFlagValue(t *testing.T) {
	in := []string{"--account", "foo@example.com", "calendar", "events", "--fields", "items(id)"}
	got := rewriteDesirePathArgs(in)
	want := []string{"--account", "foo@example.com", "calendar", "events", "--fields", "items(id)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, want)
	}
}

func TestDesirePaths_RewriteFields_RewritesNonCalendarCommands(t *testing.T) {
	in := []string{"--account", "foo@example.com", "drive", "ls", "--fields=id,name"}
	got := rewriteDesirePathArgs(in)
	want := []string{"--account", "foo@example.com", "drive", "ls", "--select=id,name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, want)
	}
}

func TestDesirePaths_RewriteFields_DoesNotRewriteAfterDoubleDash(t *testing.T) {
	in := []string{"open", "--", "--fields"}
	got := rewriteDesirePathArgs(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
	}
}

func TestDesirePaths_RewriteFields_KeepsCalendarEventsAlias(t *testing.T) {
	in := []string{"-a", "foo@example.com", "cal", "ls", "--fields", "items(id)"}
	got := rewriteDesirePathArgs(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
	}
}

func TestDesirePaths_RewriteFields_KeepsCalendarEventsWithPickAndProject(t *testing.T) {
	cases := [][]string{
		{"--pick", "id", "calendar", "events", "--fields", "items(id)"},
		{"--project", "id", "calendar", "events", "--fields", "items(id)"},
	}
	for _, in := range cases {
		in := in
		t.Run(strings.Join(in, " "), func(t *testing.T) {
			got := rewriteDesirePathArgs(in)
			if !reflect.DeepEqual(got, in) {
				t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
			}
		})
	}
}

func TestDesirePaths_CalendarAliases_AreUnambiguous(t *testing.T) {
	calendarField, ok := reflect.TypeOf(CalendarCmd{}).FieldByName("Calendars")
	if !ok {
		t.Fatalf("missing Calendars field")
	}
	if aliases := calendarField.Tag.Get("aliases"); strings.Contains(aliases, "list") || strings.Contains(aliases, "ls") {
		t.Fatalf("calendar calendars must not claim list/ls aliases: %q", aliases)
	}

	eventsField, ok := reflect.TypeOf(CalendarCmd{}).FieldByName("Events")
	if !ok {
		t.Fatalf("missing Events field")
	}
	aliases := eventsField.Tag.Get("aliases")
	if !strings.Contains(aliases, "list") || !strings.Contains(aliases, "ls") {
		t.Fatalf("calendar events should keep list/ls aliases: %q", aliases)
	}
}

func TestDesirePaths_GroupsMembers_DoesNotReuseListAliases(t *testing.T) {
	membersField, ok := reflect.TypeOf(GroupsCmd{}).FieldByName("Members")
	if !ok {
		t.Fatalf("missing Members field")
	}
	if aliases := membersField.Tag.Get("aliases"); strings.Contains(aliases, "list") || strings.Contains(aliases, "ls") {
		t.Fatalf("groups members must not claim list/ls aliases: %q", aliases)
	}
}
