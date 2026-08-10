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
`

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
