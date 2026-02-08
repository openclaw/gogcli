package cmd

import (
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
