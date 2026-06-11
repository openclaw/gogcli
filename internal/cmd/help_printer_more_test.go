package cmd

import (
	"io"
	"testing"

	"github.com/muesli/termenv"
)

func TestHelpColorModeEquals(t *testing.T) {
	if mode := helpColorMode([]string{"--color=never"}); mode != "never" {
		t.Fatalf("expected never, got %q", mode)
	}
}

func TestHelpColorModeDefault(t *testing.T) {
	if mode := helpColorMode(nil); mode != "auto" {
		t.Fatalf("expected auto, got %q", mode)
	}
}

func TestHelpProfileAutoDefault(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	if got := helpProfile(io.Discard, ""); got != termenv.Ascii {
		t.Fatalf("expected ascii profile for auto, got %v", got)
	}
}

func TestColorizeHelpNoColor(t *testing.T) {
	in := "Usage: gog\nFlags:\n"
	out := colorizeHelp(in, termenv.Ascii)
	if out != in {
		t.Fatalf("expected no color changes")
	}
}

func TestColorizeHelpSections(t *testing.T) {
	in := "Flags:\nArguments:\nAutomation:\nBuild: dev\nConfig:\nRead\nCommands:\n  foo [flags]\n    does thing\n"
	out := colorizeHelp(in, termenv.TrueColor)
	if out == in {
		t.Fatalf("expected colorized output")
	}
}

func TestColorizeCommandSummaryLineEdges(t *testing.T) {
	line := "foo [flags]"
	if got := colorizeCommandSummaryLine(line, func(s string) string { return s }, func(s string) string { return s }); got != line {
		t.Fatalf("expected passthrough for non-command line")
	}

	line = "  "
	if got := colorizeCommandSummaryLine(line, func(s string) string { return s }, func(s string) string { return s }); got != line {
		t.Fatalf("expected passthrough for empty command line")
	}

	line = "   arg"
	if got := colorizeCommandSummaryLine(line, func(s string) string { return s }, func(s string) string { return s }); got != line {
		t.Fatalf("expected passthrough for unnamed command line")
	}
}
