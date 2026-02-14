package cmd

import (
	"strings"
	"testing"
)

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
