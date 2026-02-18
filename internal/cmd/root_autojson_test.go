package cmd

import (
	"strings"
	"testing"
)

// NOTE: captureStdout replaces os.Stdout with an os.Pipe, which is always
// non-TTY. This means we can test the "piped stdout → auto-JSON" path but
// cannot test the "real TTY → no auto-JSON" path in unit tests. The TTY
// guard (term.IsTerminal) is effectively always false here.

func TestAutoJSON_Version_DefaultsToJSONWhenEnabled(t *testing.T) {
	t.Setenv("GOG_AUTO_JSON", "1")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected json output, got: %q", out)
	}
	if !strings.Contains(out, "\"version\"") {
		t.Fatalf("expected version field in json output, got: %q", out)
	}
}

func TestAutoJSON_Version_RespectsExplicitPlainFlag(t *testing.T) {
	t.Setenv("GOG_AUTO_JSON", "1")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.HasPrefix(strings.TrimSpace(out), "{") || strings.Contains(out, "\"version\"") {
		t.Fatalf("expected text output (not json), got: %q", out)
	}
}

func TestAutoJSON_Version_DefaultsToJSONWhenForceOutputEnabled(t *testing.T) {
	t.Setenv("GOG_FORCE_OUTPUT", "1")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected json output, got: %q", out)
	}
	if !strings.Contains(out, "\"version\"") {
		t.Fatalf("expected version field in json output, got: %q", out)
	}
}

func TestAutoJSON_Version_PlainOverridesForceOutput(t *testing.T) {
	t.Setenv("GOG_FORCE_OUTPUT", "1")

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--plain", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.HasPrefix(strings.TrimSpace(out), "{") || strings.Contains(out, "\"version\"") {
		t.Fatalf("expected text output (not json), got: %q", out)
	}
}

func TestAutoJSON_Version_PlainOverridesNonInteractive(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--non-interactive", "--plain", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if strings.HasPrefix(strings.TrimSpace(out), "{") || strings.Contains(out, "\"version\"") {
		t.Fatalf("expected text output (not json), got: %q", out)
	}
}

func TestAutoJSON_Version_DefaultsToJSONWhenNoInputAndPiped(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--non-interactive", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected json output, got: %q", out)
	}
	if !strings.Contains(out, "\"version\"") {
		t.Fatalf("expected version field in json output, got: %q", out)
	}
}
