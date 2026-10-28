package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/safetyprofile"
)

const deniedValuesProfile = `
name: denied
denied-values:
  gmail.thread.modify:
    add: [TRASH, SPAM]
    add-label: [TRASH, SPAM]
  gmail.messages.modify:
    add: [TRASH]
  calendar.create:
    send-updates: [all, externalOnly]
gmail:
  thread:
    modify: true
  messages:
    modify: true
  batch:
    modify: true
calendar:
  create: true
`

func threadModify(extra ...string) []string {
	return append([]string{"--json", "--account", "a@b.com", "gmail", "thread", "modify", "t1"}, extra...)
}

func calendarCreate(extra ...string) []string {
	return append([]string{
		"--json", "--account", "a@b.com", "calendar", "create", "primary",
		"--summary", "x", "--from", "2026-11-01T10:00:00Z", "--to", "2026-11-01T11:00:00Z",
	}, extra...)
}

func requireDenied(t *testing.T, args []string, want string) {
	t.Helper()
	withBakedSafetyProfile(t, deniedValuesProfile)
	result := executeWithTestRuntime(t, args, nil)
	if result.err == nil {
		t.Fatalf("%v must be refused, got stdout=%q", args, result.stdout)
	}
	if got := result.err.Error(); !strings.Contains(got, want) {
		t.Fatalf("err = %q, want it to contain %q", got, want)
	}
	if code := ExitCode(result.err); code != 2 {
		t.Fatalf("exit code = %d, want 2 (err=%v)", code, result.err)
	}
}

func requireNotDenied(t *testing.T, args []string) {
	t.Helper()
	withBakedSafetyProfile(t, deniedValuesProfile)
	result := executeWithTestRuntime(t, args, nil)
	if result.err != nil && strings.Contains(result.err.Error(), "is denied") {
		t.Fatalf("%v must not be refused: %v", args, result.err)
	}
}

func TestDeniedValues_RefusesEverySpellingOfADeniedValue(t *testing.T) {
	for name, extra := range map[string][]string{
		"upper":       {"--add", "TRASH"},
		"lower":       {"--add", "trash"},
		"padded":      {"--add", " Trash "},
		"equals form": {"--add=trash"},
		"csv":         {"--add", "TRASH,INBOX"},
		"csv later":   {"--add", "INBOX, trash"},
		"repeated":    {"--add", "INBOX", "--add", "SPAM"},
		"add-label":   {"--add-label", "trash"},
		"dry-run":     {"--dry-run", "--add", "TRASH"},
	} {
		t.Run(name, func(t *testing.T) {
			requireDenied(t, threadModify(extra...), `is denied for "gmail thread modify" by baked safety profile "denied"`)
		})
	}
}

func TestDeniedValues_MessageEchoesTheValueAsTyped(t *testing.T) {
	requireDenied(t, threadModify("--add", "  Trash  "), `flag --add value "Trash" is denied for "gmail thread modify" by baked safety profile "denied"`)
	requireDenied(t, threadModify("--add-label", "SPAM"), `flag --add-label value "SPAM" is denied for "gmail thread modify" by baked safety profile "denied"`)
}

// Aliases resolve to the canonical command path before the rule is looked up.
func TestDeniedValues_RefusesThroughCommandAliases(t *testing.T) {
	requireDenied(t, []string{"--json", "--account", "a@b.com", "gmail", "thread", "edit", "t1", "--add", "TRASH"}, `for "gmail thread modify"`)
	requireDenied(t, []string{"--json", "--account", "a@b.com", "gmail", "messages", "set", "m1", "--add", "TRASH"}, `for "gmail messages modify"`)
}

func TestDeniedValues_RefusesCalendarSendUpdates(t *testing.T) {
	for _, value := range []string{"all", "ALL", "externalOnly", "externalonly"} {
		t.Run(value, func(t *testing.T) {
			requireDenied(t, calendarCreate("--send-updates", value), `flag --send-updates value "`+value+`" is denied for "calendar create"`)
		})
	}
	requireDenied(t, calendarCreate("--dry-run", "--send-updates", "all"), `is denied for "calendar create"`)
}

func TestDeniedValues_AllowsOtherValues(t *testing.T) {
	requireNotDenied(t, threadModify("--add", "INBOX"))
	requireNotDenied(t, threadModify("--add", "INBOX", "--remove", "TRASH"))
	requireNotDenied(t, threadModify("--remove", "TRASH"))
	requireNotDenied(t, threadModify("--add-label", "Label_1"))
	requireNotDenied(t, calendarCreate("--send-updates", "none"))
	requireNotDenied(t, calendarCreate())
	requireNotDenied(t, calendarCreate("--dry-run"))
}

// A denied value is not a denied substring: a label whose name merely contains it is fine.
func TestDeniedValues_MatchesWholeValuesOnly(t *testing.T) {
	requireNotDenied(t, threadModify("--add", "TRASHCAN"))
	requireNotDenied(t, threadModify("--add", "NOT-SPAM"))
}

// --add-label takes a label name literally, commas included, so a denied value
// containing a comma must match the whole value and not only its parts.
func TestDeniedValues_MatchesADeniedValueContainingAComma(t *testing.T) {
	withBakedSafetyProfile(t, `
name: denied
denied-values:
  gmail.thread.modify:
    add-label: ["Archive, Old"]
gmail:
  thread:
    modify: true
`)
	result := executeWithTestRuntime(t, threadModify("--add-label", "archive, old"), nil)
	if result.err == nil || !strings.Contains(result.err.Error(), `flag --add-label value "archive, old" is denied`) {
		t.Fatalf("a denied value containing a comma must be refused, got err=%v stdout=%q", result.err, result.stdout)
	}
}

// The rule is keyed to a command path, so another command with a flag of the same
// name is untouched.
func TestDeniedValues_InertOnOtherCommands(t *testing.T) {
	requireNotDenied(t, []string{
		"--json", "--account", "a@b.com", "gmail", "batch", "modify", "m1", "--add", "TRASH",
	})
}

// Stock builds compile the no-profile stub, so nothing is denied at all.
func TestDeniedValues_IgnoredWithoutBakedProfile(t *testing.T) {
	result := executeWithTestRuntime(t, threadModify("--add", "TRASH"), nil)
	if result.err != nil && strings.Contains(result.err.Error(), "is denied") {
		t.Fatalf("denied value applied without a baked profile: %v", result.err)
	}
}

func TestDeniedValues_NonexistentPathRefusesToRun(t *testing.T) {
	withBakedSafetyProfile(t, `
name: denied
denied-values:
  gmail.thread.mdoify:
    add: [TRASH]
gmail:
  thread:
    modify: true
`)
	result := executeWithTestRuntime(t, threadModify("--add", "INBOX"), nil)
	if result.err == nil {
		t.Fatalf("a misspelled command path must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "denies values for 1 flag(s) but only 0 exist") {
		t.Fatalf("err = %v", result.err)
	}
	if code := ExitCode(result.err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

func TestDeniedValues_NonexistentFlagRefusesToRun(t *testing.T) {
	withBakedSafetyProfile(t, `
name: denied
denied-values:
  gmail.thread.modify:
    ad: [TRASH]
gmail:
  thread:
    modify: true
`)
	result := executeWithTestRuntime(t, threadModify("--add", "INBOX"), nil)
	if result.err == nil {
		t.Fatalf("a misspelled flag must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "denies values for 1 flag(s) but only 0 exist") {
		t.Fatalf("err = %v", result.err)
	}
}

// A rule on a flag the command does not declare itself (a root flag) locks nothing.
func TestDeniedValues_RootFlagRefusesToRun(t *testing.T) {
	withBakedSafetyProfile(t, `
name: denied
denied-values:
  gmail.thread.modify:
    account: [a@b.com]
gmail:
  thread:
    modify: true
`)
	result := executeWithTestRuntime(t, threadModify("--add", "INBOX"), nil)
	if result.err == nil {
		t.Fatalf("a rule on a flag the command does not declare must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "denies values for 1 flag(s) but only 0 exist") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestDeniedValues_BooleanFlagRefusesToRun(t *testing.T) {
	// Quoted: an unquoted YAML boolean is rejected at parse time, and the failure
	// under test is the flag type rather than the value type.
	withBakedSafetyProfile(t, `
name: denied
denied-values:
  calendar.create:
    all-day: ["true"]
calendar:
  create: true
`)
	result := executeWithTestRuntime(t, calendarCreate(), nil)
	if result.err == nil {
		t.Fatalf("a boolean flag cannot be denied by value, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "only string and list flags can have denied values") {
		t.Fatalf("err = %v", result.err)
	}
}

// The count and type checks must not fire for a profile that uses none of this.
func TestDeniedValues_ProfileWithoutRulesIsUnaffected(t *testing.T) {
	withBakedSafetyProfile(t, "name: plain\ngmail:\n  thread:\n    modify: true\n")
	result := executeWithTestRuntime(t, threadModify("--add", "TRASH"), nil)
	if result.err != nil && strings.Contains(result.err.Error(), "denied") {
		t.Fatalf("profile without denied-values refused a value: %v", result.err)
	}
}

func TestAgentSafeProfileDeniesDestructiveAndNotifyingValues(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "safety-profiles", "agent-safe.yaml"))
	if err != nil {
		t.Fatalf("read agent-safe profile: %v", err)
	}
	profile, err := safetyprofile.Parse(string(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := map[string][]string{}
	for _, rule := range profile.DeniedValues {
		got[rule.Path+" --"+rule.Flag] = rule.Values
	}
	trashSpam := []string{"spam", "trash"}
	notify := []string{"all", "externalonly"}
	want := map[string][]string{
		"gmail.messages.modify --add":     trashSpam,
		"gmail.thread.modify --add":       trashSpam,
		"gmail.thread.modify --add-label": trashSpam,
		"gmail.labels.modify --add":       trashSpam,
		"gmail.batch.modify --add":        trashSpam,
		"gmail.batch.modify --add-label":  trashSpam,
		"calendar.create --send-updates":  notify,
		"calendar.update --send-updates":  notify,
		"calendar.move --send-updates":    notify,
		// --auto-decline defaults to all, and declining a conflicting invitation
		// notifies its organizer, so only an explicit --auto-decline none passes.
		"calendar.focus-time --auto-decline": {"all", "new"},
	}
	// create and update can also write focus-time and out-of-office events, whose
	// auto-decline defaults to all, either by --event-type or by type inference from
	// any focus or out-of-office flag.
	anyValue := []string{"*"}
	for _, command := range []string{"calendar.create", "calendar.update"} {
		want[command+" --event-type"] = []string{"focus", "focus-time", "focus_time", "focustime", "ooo", "out-of-office", "out_of_office", "outofoffice"}
		// create ignores an empty auto-decline, so it can pass none. update infers the
		// type from a supplied flag and defaults an empty value to all, so any use is refused.
		autoDecline := []string{"all", "new"}
		if command == "calendar.update" {
			autoDecline = anyValue
		}
		want[command+" --focus-auto-decline"] = autoDecline
		want[command+" --ooo-auto-decline"] = autoDecline
		want[command+" --focus-decline-message"] = anyValue
		want[command+" --focus-chat-status"] = anyValue
		want[command+" --ooo-decline-message"] = anyValue
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agent-safe denied-values = %#v, want %#v", got, want)
	}
}

// Every rule in the shipped preset must name a real flag, or the binary refuses to
// run. Executing any command proves the pre-parse validation accepts the profile.
func TestAgentSafeProfileDeniedValueRulesAllExist(t *testing.T) {
	setTestConfigHome(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "safety-profiles", "agent-safe.yaml"))
	if err != nil {
		t.Fatalf("read agent-safe profile: %v", err)
	}
	withBakedSafetyProfile(t, string(raw))
	result := executeWithTestRuntime(t, []string{"version"}, nil)
	if result.err != nil {
		t.Fatalf("agent-safe must start: %v (stderr=%q)", result.err, result.stderr)
	}
}

// "*" refuses any non-empty value; an omitted or empty flag is not a value.
func TestDeniedValues_AnyValueRefusesNonEmptyValues(t *testing.T) {
	const profile = `
name: denied
denied-values:
  gmail.thread.modify:
    add: ["*"]
gmail:
  thread:
    modify: true
`
	for _, extra := range [][]string{{"--add", "x"}, {"--add", " x "}, {"--add", ","}, {"--add", "INBOX", "--add", "x"}} {
		withBakedSafetyProfile(t, profile)
		result := executeWithTestRuntime(t, threadModify(extra...), nil)
		if result.err == nil || !strings.Contains(result.err.Error(), "is denied for \"gmail thread modify\"") {
			t.Fatalf("%v must be refused, got err=%v", extra, result.err)
		}
		if code := ExitCode(result.err); code != 2 {
			t.Fatalf("%v exit code = %d, want 2", extra, code)
		}
	}
	// An empty value typed on the command line is refused too: update treats a
	// supplied but empty flag as supplied.
	for _, extra := range [][]string{{"--add", ""}, {"--add", "  "}, {"--add="}} {
		withBakedSafetyProfile(t, profile)
		result := executeWithTestRuntime(t, threadModify(extra...), nil)
		if result.err == nil || !strings.Contains(result.err.Error(), "is denied for \"gmail thread modify\"") {
			t.Fatalf("%v must be refused, got err=%v", extra, result.err)
		}
	}
	for _, extra := range [][]string{{"--remove", "x"}, nil} {
		withBakedSafetyProfile(t, profile)
		result := executeWithTestRuntime(t, threadModify(extra...), nil)
		if result.err != nil && strings.Contains(result.err.Error(), "is denied") {
			t.Fatalf("%v must not be refused: %v", extra, result.err)
		}
	}
}

func agentSafeProfile(t *testing.T) {
	t.Helper()
	setTestConfigHome(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "safety-profiles", "agent-safe.yaml"))
	if err != nil {
		t.Fatalf("read agent-safe profile: %v", err)
	}
	withBakedSafetyProfile(t, string(raw))
}

func calendarUpdate(extra ...string) []string {
	return append([]string{"--json", "--account", "a@b.com", "calendar", "update", "primary", "evt1"}, extra...)
}

// Focus-time and out-of-office events auto-decline conflicting invitations by default,
// which notifies the organizers, and create/update reach them by --event-type or by
// inference from any focus or out-of-office flag.
func TestAgentSafeProfileRefusesFocusAndOutOfOfficeRoutes(t *testing.T) {
	for name, args := range map[string][]string{
		"create focus-time":       calendarCreate("--event-type", "focus-time"),
		"create focus alias":      calendarCreate("--event-type", " Focus "),
		"create focus_time":       calendarCreate("--event-type", "focus_time"),
		"create ooo":              calendarCreate("--event-type", "OOO"),
		"create out-of-office":    calendarCreate("--event-type", "out-of-office"),
		"create outofoffice":      calendarCreate("--event-type", "outofoffice"),
		"create focus chat":       calendarCreate("--focus-chat-status", "doNotDisturb"),
		"create focus message":    calendarCreate("--focus-decline-message", "hi"),
		"create ooo message":      calendarCreate("--ooo-decline-message", "hi"),
		"create focus all":        calendarCreate("--focus-auto-decline", "all"),
		"create focus new":        calendarCreate("--focus-auto-decline", "NEW"),
		"create ooo all":          calendarCreate("--ooo-auto-decline", "all"),
		"create focus dry-run":    calendarCreate("--dry-run", "--event-type", "focus"),
		"update focus-time":       calendarUpdate("--event-type", "focus"),
		"update ooo":              calendarUpdate("--event-type", "ooo"),
		"update focus chat":       calendarUpdate("--focus-chat-status", "doNotDisturb"),
		"update ooo message":      calendarUpdate("--ooo-decline-message", "hi"),
		"update ooo all":          calendarUpdate("--ooo-auto-decline", "all"),
		"update focus all dryrun": calendarUpdate("--dry-run", "--focus-auto-decline", "all"),
		// update infers the type from a flag that was supplied even when its value is
		// empty, and an empty auto-decline then defaults to all.
		"update focus none":        calendarUpdate("--focus-auto-decline", "none"),
		"update ooo none":          calendarUpdate("--ooo-auto-decline", "none"),
		"update empty focus auto":  calendarUpdate("--focus-auto-decline", ""),
		"update empty ooo auto":    calendarUpdate("--ooo-auto-decline", ""),
		"update empty focus msg":   calendarUpdate("--focus-decline-message", ""),
		"update empty focus chat":  calendarUpdate("--focus-chat-status", ""),
		"update empty ooo message": calendarUpdate("--ooo-decline-message", ""),
	} {
		t.Run(name, func(t *testing.T) {
			agentSafeProfile(t)
			result := executeWithTestRuntime(t, args, nil)
			if result.err == nil || !strings.Contains(result.err.Error(), "is denied for") {
				t.Fatalf("agent-safe must refuse %v, got err=%v stdout=%q", args, result.err, result.stdout)
			}
			if code := ExitCode(result.err); code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
		})
	}
}

// An explicit none sends no declines, so it must keep working, as must ordinary events.
func TestAgentSafeProfileAllowsFocusWithoutAutoDecline(t *testing.T) {
	for name, args := range map[string][]string{
		"create focus none":   calendarCreate("--dry-run", "--focus-auto-decline", "none"),
		"create ooo none":     calendarCreate("--dry-run", "--ooo-auto-decline", "none"),
		"create default type": calendarCreate("--dry-run", "--event-type", "default"),
		"create plain":        calendarCreate("--dry-run"),
		"update summary":      calendarUpdate("--dry-run", "--summary", "y"),
	} {
		t.Run(name, func(t *testing.T) {
			agentSafeProfile(t)
			result := executeWithTestRuntime(t, args, nil)
			if result.err != nil && strings.Contains(result.err.Error(), "is denied") {
				t.Fatalf("agent-safe must allow %v: %v", args, result.err)
			}
		})
	}
}

func TestAgentSafeProfileRefusesTrashAndNotifyingSends(t *testing.T) {
	setTestConfigHome(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "safety-profiles", "agent-safe.yaml"))
	if err != nil {
		t.Fatalf("read agent-safe profile: %v", err)
	}
	withBakedSafetyProfile(t, string(raw))
	for _, args := range [][]string{
		threadModify("--add", "TRASH"),
		calendarCreate("--send-updates", "all"),
	} {
		result := executeWithTestRuntime(t, args, nil)
		if result.err == nil || !strings.Contains(result.err.Error(), "is denied") {
			t.Fatalf("agent-safe must refuse %v, got err=%v", args, result.err)
		}
	}
}
