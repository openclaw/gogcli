package cmd

import (
	"strings"
	"testing"
)

func captureHelpOutput(t *testing.T, args ...string) string {
	t.Helper()
	return captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute(args); err != nil {
				t.Fatalf("Execute(%v): %v", args, err)
			}
		})
	})
}

func requireHelpContains(t *testing.T, out string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(out, part) {
			t.Fatalf("expected help to contain %q, got: %q", part, out)
		}
	}
}

func TestHelpSnapshot_Auth(t *testing.T) {
	out := captureHelpOutput(t, "auth", "--help")
	requireHelpContains(t, out,
		"\n  add",
		"\n  credentials",
		"\n  tokens",
		"\n  alias",
	)
}

func TestHelpSnapshot_DocsUpdate(t *testing.T) {
	out := captureHelpOutput(t, "docs", "update", "--help")
	requireHelpContains(t, out,
		"--tab-id",
		"--text",
		"--file",
		"--index",
	)
}

func TestHelpSnapshot_Sheets(t *testing.T) {
	out := captureHelpOutput(t, "sheets", "--help")
	requireHelpContains(t, out,
		"\n  add-tab",
		"\n  rename-tab",
		"\n  delete-tab",
		"\n  named-ranges",
		"\n  update-note",
	)
}

func TestHelpSnapshot_Forms(t *testing.T) {
	out := captureHelpOutput(t, "forms", "--help")
	requireHelpContains(t, out,
		"\n  create",
		"\n  update",
		"\n  add-question",
		"\n  watch",
	)
}

func TestHelpSnapshot_Admin(t *testing.T) {
	out := captureHelpOutput(t, "admin", "--help")
	requireHelpContains(t, out,
		"\n  users",
		"\n  groups",
	)
}
