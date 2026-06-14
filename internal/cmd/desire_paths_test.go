package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestRootDesirePathsHelpParses(t *testing.T) {
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

func TestDesirePaths_RewriteHelp(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "root", in: []string{"help"}, want: []string{"--help"}},
		{name: "command", in: []string{"help", "drive", "ls"}, want: []string{"drive", "ls", "--help"}},
		{name: "global flag", in: []string{"--color", "never", "help", "gmail"}, want: []string{"--color", "never", "gmail", "--help"}},
		{name: "help ignores trailing args", in: []string{"drive", "--help", "nonsense"}, want: []string{"drive", "--help"}},
		{name: "help after delimiter is data", in: []string{"open", "--", "--help"}, want: []string{"open", "--", "--help"}},
		{name: "global value named help", in: []string{"--account", "help", "version"}, want: []string{"--account", "help", "version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rewriteHelpArgs(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("rewriteHelpArgs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
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
	model := desirePathModel(t)
	cases := [][]string{
		{"--account", "foo@example.com", "calendar", "events", "--fields", "items(id)"},
		{"--home", "/tmp/gog-home", "calendar", "events", "--fields", "items(id)"},
		{"--access-token", "ya29.test-token", "drive", "ls", "--fields", "files(id,name)"},
	}
	for _, in := range cases {
		in := in
		t.Run(strings.Join(in, " "), func(t *testing.T) {
			got := rewriteDesirePathArgs(model, in)
			if !reflect.DeepEqual(got, in) {
				t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
			}
		})
	}
}

func TestDesirePaths_RewriteFields_RewritesNonCalendarCommands(t *testing.T) {
	in := []string{"--account", "foo@example.com", "gmail", "search", "newer_than:1d", "--fields=id,name"}
	got := rewriteDesirePathArgs(desirePathModel(t), in)
	want := []string{"--account", "foo@example.com", "gmail", "search", "newer_than:1d", "--select=id,name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, want)
	}
}

func TestDesirePaths_RewriteFields_KeepsCommandsWithLocalFields(t *testing.T) {
	model := desirePathModel(t)
	cases := [][]string{
		{"ls", "--fields=files(id,name)"},
		{"list", "--fields=files(id,name)"},
		{"drive", "ls", "--fields=files(id,name)"},
		{"drive", "ls", "--max", "1", "--fields=files(id,name)"},
		{"drive", "get", "file123", "--fields", "id,name,mimeType"},
		{"drive", "raw", "file123", "--fields", "id,name"},
		{"drive", "labels", "list", "--fields", "labels(id,name)"},
		{"drive", "labels", "get", "labels/123", "--fields", "id,name"},
		{"drive", "labels", "file", "list", "file123", "--fields", "labels(id,name)"},
		{"sites", "get", "site123", "--fields", "id,name,webViewLink"},
	}
	for _, in := range cases {
		in := in
		t.Run(strings.Join(in, " "), func(t *testing.T) {
			got := rewriteDesirePathArgs(model, in)
			if !reflect.DeepEqual(got, in) {
				t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
			}
		})
	}
}

func TestDesirePaths_RewriteFields_RewritesGlobalPositionFields(t *testing.T) {
	in := []string{"--fields=id,name", "drive", "ls"}
	got := rewriteDesirePathArgs(desirePathModel(t), in)
	want := []string{"--select=id,name", "drive", "ls"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, want)
	}
}

func TestDesirePaths_RewriteFields_DoesNotRewriteAfterDoubleDash(t *testing.T) {
	in := []string{"open", "--", "--fields"}
	got := rewriteDesirePathArgs(desirePathModel(t), in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
	}
}

func TestDesirePaths_RewriteFields_KeepsCalendarEventsAlias(t *testing.T) {
	in := []string{"-a", "foo@example.com", "cal", "ls", "--fields", "items(id)"}
	got := rewriteDesirePathArgs(desirePathModel(t), in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
	}
}

func TestDesirePaths_RewriteFields_KeepsCalendarEventsWithPickAndProject(t *testing.T) {
	model := desirePathModel(t)
	cases := [][]string{
		{"--pick", "id", "calendar", "events", "--fields", "items(id)"},
		{"--project", "id", "calendar", "events", "--fields", "items(id)"},
	}
	for _, in := range cases {
		in := in
		t.Run(strings.Join(in, " "), func(t *testing.T) {
			got := rewriteDesirePathArgs(model, in)
			if !reflect.DeepEqual(got, in) {
				t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
			}
		})
	}
}

func TestDesirePaths_RewriteFields_KeepsEveryModelOwnedFieldsFlag(t *testing.T) {
	model := desirePathModel(t)
	paths := localFlagCommandPaths(model.Node, "fields")
	if len(paths) == 0 {
		t.Fatal("expected commands with local --fields flags")
	}
	for _, path := range paths {
		path := path
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			in := append(append([]string(nil), path...), "--fields=id,name")
			if got := rewriteDesirePathArgs(model, in); !reflect.DeepEqual(got, in) {
				t.Fatalf("unexpected rewrite: got=%v want=%v", got, in)
			}
		})
	}
}

func TestDesirePaths_RewriteFields_DoesNotMutateModelActivation(t *testing.T) {
	model := desirePathModel(t)
	before := modelFlagActivation(model.Node)

	_ = rewriteDesirePathArgs(model, []string{
		"--account", "foo@example.com",
		"drive", "ls", "--max", "1",
		"--fields=files(id,name)",
	})

	if after := modelFlagActivation(model.Node); !reflect.DeepEqual(after, before) {
		t.Fatalf("model flag activation changed: before=%v after=%v", before, after)
	}
}

func desirePathModel(t *testing.T) *kong.Application {
	t.Helper()
	parser, _, err := newParser("test parser")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	return parser.Model
}

func modelFlagActivation(root *kong.Node) map[*kong.Flag]bool {
	active := make(map[*kong.Flag]bool)
	var walk func(*kong.Node)
	walk = func(node *kong.Node) {
		for _, flag := range node.Flags {
			active[flag] = flag.Active
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return active
}

func localFlagCommandPaths(root *kong.Node, flagName string) [][]string {
	var paths [][]string
	var walk func(*kong.Node, []string)
	walk = func(node *kong.Node, prefixes []string) {
		for _, child := range node.Children {
			if child.Type != kong.CommandNode {
				continue
			}
			spellings := acceptedCommandSpellings(node, child)
			childPrefixes := make([]string, 0, len(prefixes)*len(spellings))
			for _, prefix := range prefixes {
				for _, spelling := range spellings {
					childPrefixes = append(childPrefixes, strings.TrimSpace(prefix+" "+spelling))
				}
			}
			if nodeHasLocalFlag(child, flagName) {
				for _, path := range childPrefixes {
					paths = append(paths, strings.Fields(path))
				}
			}
			walk(child, childPrefixes)
		}
	}
	walk(root, []string{""})
	return paths
}

func acceptedCommandSpellings(parent, child *kong.Node) []string {
	spellings := []string{child.Name}
	for _, alias := range child.Aliases {
		conflicts := false
		for _, sibling := range parent.Children {
			if sibling.Type == kong.CommandNode && sibling.Name == alias {
				conflicts = true
				break
			}
		}
		if !conflicts {
			spellings = append(spellings, alias)
		}
	}
	return spellings
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
