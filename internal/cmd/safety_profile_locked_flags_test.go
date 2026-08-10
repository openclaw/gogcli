package cmd

import (
	"strings"
	"testing"
)

const lockedFlagsProfile = `
name: locked
locked-flags:
  sanitize-content: true
gmail:
  get: true
  search: true
`

// A lock must hold whatever spelling reaches it, including the value it already
// has: accepting a matching value would make the lock depend on what the caller
// asked for.
func TestLockedFlag_RejectsEveryFormOfSettingIt(t *testing.T) {
	for _, arg := range []string{
		"--sanitize-content=false",
		"--sanitize-content=true",
		"--sanitize-content",
		"--sanitize=false",
		"--safe",
	} {
		t.Run(arg, func(t *testing.T) {
			withBakedSafetyProfile(t, lockedFlagsProfile)
			result := executeWithTestRuntime(t, []string{
				"--json", "--account", "a@b.com",
				"gmail", "get", "m1", arg,
			}, nil)
			if result.err == nil {
				t.Fatalf("%s must be rejected, got stdout=%q", arg, result.stdout)
			}
			if !strings.Contains(result.err.Error(), "--sanitize-content is locked") {
				t.Fatalf("%s: err = %v", arg, result.err)
			}
		})
	}
}

// Locking a flag one command declares must not disturb commands that do not, or
// locking a per-command flag would break the rest of the CLI.
func TestLockedFlag_InertOnCommandsWithoutTheFlag(t *testing.T) {
	withBakedSafetyProfile(t, lockedFlagsProfile)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "search", "newer_than:1d",
	}, nil)
	if result.err != nil && strings.Contains(result.err.Error(), "is locked") {
		t.Fatalf("lock leaked into a command without the flag: %v", result.err)
	}
}

// Stock builds compile the no-profile stub, so locks must not apply at all.
func TestLockedFlag_IgnoredWithoutBakedProfile(t *testing.T) {
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1", "--sanitize-content=false",
	}, nil)
	if result.err != nil && strings.Contains(result.err.Error(), "is locked") {
		t.Fatalf("lock applied without a baked profile: %v", result.err)
	}
}

// A value the flag cannot parse must fail loudly; silently skipping it would leave
// the flag unlocked while the profile claims otherwise.
func TestLockedFlag_UnparsableValueFails(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  sanitize-content: notabool
gmail:
  get: true
`)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1",
	}, nil)
	if result.err == nil {
		t.Fatalf("unparsable locked value must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "locked flag --sanitize-content") {
		t.Fatalf("err = %v", result.err)
	}
}

// Non-bool locks go through the same flag parser, so prove an int actually lands.
func TestLockedFlag_NonBoolValueReachesCommand(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  inline-max-bytes: -1
gmail:
  attachment: true
`)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "attachment", "m1", "a1",
	}, nil)
	if result.err == nil {
		t.Fatalf("locked inline-max-bytes must reach the command, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "--inline-max-bytes must be non-negative") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestLockedFlag_RejectsCommandLineOverride(t *testing.T) {
	withBakedSafetyProfile(t, lockedFlagsProfile)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1", "--sanitize-content=false",
	}, nil)
	if result.err == nil {
		t.Fatalf("locked flag override must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "--sanitize-content is locked") {
		t.Fatalf("err = %v", result.err)
	}
}

func TestLockedFlag_RejectsAliasOverride(t *testing.T) {
	withBakedSafetyProfile(t, lockedFlagsProfile)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1", "--safe=false",
	}, nil)
	if result.err == nil {
		t.Fatalf("alias override must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "--sanitize-content is locked") {
		t.Fatalf("err = %v", result.err)
	}
}

// The locked value must reach the command, not merely block overrides. gmail get
// refuses --format raw when sanitize is on, so that refusal proves the profile set
// the flag even though the command line never mentioned it.
func TestLockedFlag_LockedValueReachesCommand(t *testing.T) {
	withBakedSafetyProfile(t, lockedFlagsProfile)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1", "--format", "raw",
	}, nil)
	if result.err == nil {
		t.Fatalf("locked sanitize-content must reject --format raw, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "cannot be used with --format raw") {
		t.Fatalf("err = %v", result.err)
	}
	// The caller never passed the flag, so the printed message has to say where the
	// value came from. The note is added at display time, not to the error itself.
	if !strings.Contains(result.stderr, `note: --sanitize-content locked by baked safety profile "locked"`) {
		t.Fatalf("stderr lacks the locked-flag note: %q", result.stderr)
	}
}

// Without the profile the same command line is accepted, so the rejection above
// comes from the lock rather than from the flag being unusable.
func TestLockedFlag_UnlockedProfileAllowsOverride(t *testing.T) {
	withBakedSafetyProfile(t, "name: unlocked\ngmail:\n  get: true\n")
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1", "--sanitize-content=false",
	}, nil)
	if result.err != nil && strings.Contains(result.err.Error(), "is locked") {
		t.Fatalf("unlocked profile must not reject the flag: %v", result.err)
	}
}

const lockedJSONProfile = `
name: locked
locked-flags:
  json: true
version: true
`

// --json and --plain are mutually exclusive, so asking for the competing mode is
// asking for output the profile forbids. Refusing it rather than quietly producing
// the locked mode keeps typing --plain as visible as typing the locked flag itself.
func TestLockedFlag_LockedJSONRejectsExplicitPlain(t *testing.T) {
	withBakedSafetyProfile(t, lockedJSONProfile)
	result := executeWithTestRuntime(t, []string{"--plain", "version"}, nil)
	if result.err == nil {
		t.Fatalf("--plain against a locked json must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), `--plain conflicts with --json, locked by baked safety profile "locked"`) {
		t.Fatalf("err = %v", result.err)
	}
}

func TestLockedFlag_LockedJSONBeatsEnvironmentPlain(t *testing.T) {
	t.Setenv("GOG_PLAIN", "1")
	withBakedSafetyProfile(t, lockedJSONProfile)
	result := executeWithTestRuntime(t, []string{"version"}, nil)
	if result.err != nil {
		t.Fatalf("locked json with GOG_PLAIN must not fail: %v (stderr=%q)", result.err, result.stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(result.stdout), "{") {
		t.Fatalf("expected JSON output, got %q", result.stdout)
	}
}

func TestLockedFlag_LockedPlainRejectsExplicitJSON(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  plain: true
version: true
`)
	result := executeWithTestRuntime(t, []string{"--json", "version"}, nil)
	if result.err == nil {
		t.Fatalf("--json against a locked plain must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), `--json conflicts with --plain, locked by baked safety profile "locked"`) {
		t.Fatalf("err = %v", result.err)
	}
}

// The locked mode still applies when nothing competes on the command line.
func TestLockedFlag_LockedPlainAppliesWithoutCompetingFlag(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  plain: true
version: true
`)
	result := executeWithTestRuntime(t, []string{"version"}, nil)
	if result.err != nil {
		t.Fatalf("locked plain must apply: %v (stderr=%q)", result.err, result.stderr)
	}
	if strings.HasPrefix(strings.TrimSpace(result.stdout), "{") {
		t.Fatalf("expected plain output, got %q", result.stdout)
	}
}

// A locked name that matches no flag locks nothing, so the binary must refuse to run
// rather than report a guarantee it cannot keep.
func TestLockedFlag_NonexistentNameRefusesToRun(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  sanitize-contnet: true
gmail:
  get: true
`)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "get", "m1",
	}, nil)
	if result.err == nil {
		t.Fatalf("a misspelled locked flag must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "locks 1 flag(s) but only 0 exist") {
		t.Fatalf("err = %v", result.err)
	}
}

// Commands that build partial requests ask which flags were given, so a locked value
// has to count as given or it never reaches the request. Lock enforcement asks the
// narrower question and must not see itself as an override.
func TestLockedFlag_CountsAsProvidedButNotAsTyped(t *testing.T) {
	previous := lockedFlagNames
	lockedFlagNames = map[string]bool{"summary": true}
	t.Cleanup(func() { lockedFlagNames = previous })

	if !flagProvided(nil, "summary") {
		t.Fatal("a locked flag must count as provided")
	}
	if flagOnCommandLine(nil, "summary") {
		t.Fatal("a locked flag must not count as typed on the command line")
	}
}

// --home is read from argv before Kong parses, so a lock on it would leave the
// caller's own config and credential roots in place while the profile claimed
// otherwise. Refusing is the honest outcome.
func TestLockedFlag_HomeIsRefused(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  home: /tmp/locked-home
version: true
`)
	result := executeWithTestRuntime(t, []string{"version"}, nil)
	if result.err == nil {
		t.Fatalf("locking --home must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "locks --home") {
		t.Fatalf("err = %v", result.err)
	}
}

// Kong validates required flags before locks run, so a locked required flag can
// never be satisfied: omitted it fails parsing, supplied it is refused as an override.
func TestLockedFlag_RequiredFlagIsRefused(t *testing.T) {
	withBakedSafetyProfile(t, `
name: locked
locked-flags:
  to: someone@example.com
gmail:
  send: true
`)
	result := executeWithTestRuntime(t, []string{
		"--json", "--account", "a@b.com",
		"gmail", "send", "--subject", "x", "--body", "y",
	}, nil)
	if result.err == nil {
		t.Fatalf("locking a required flag must fail, got stdout=%q", result.stdout)
	}
	if !strings.Contains(result.err.Error(), "locks required flag") {
		t.Fatalf("err = %v", result.err)
	}
}
