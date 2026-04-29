package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/muesli/termenv"
)

func TestHelpColorMode(t *testing.T) {
	orig := os.Getenv("GOG_COLOR")
	t.Cleanup(func() { _ = os.Setenv("GOG_COLOR", orig) })

	_ = os.Setenv("GOG_COLOR", "always")
	if mode := helpColorMode([]string{"--plain"}); mode != "always" {
		t.Fatalf("expected env override, got %q", mode)
	}

	_ = os.Setenv("GOG_COLOR", "")
	if mode := helpColorMode([]string{"--json"}); mode != "never" {
		t.Fatalf("expected json to force never, got %q", mode)
	}

	if mode := helpColorMode([]string{"--color", "always"}); mode != "always" {
		t.Fatalf("expected always, got %q", mode)
	}
}

func TestInjectBuildLine(t *testing.T) {
	origVersion := version
	origCommit := commit
	t.Cleanup(func() {
		version = origVersion
		commit = origCommit
	})

	version = "1.2.3"
	commit = "abc"

	in := "Usage: gog\nFlags:\n"
	out := injectBuildLine(in)
	if !bytes.Contains([]byte(out), []byte("Build: 1.2.3 (abc)")) {
		t.Fatalf("build line missing: %q", out)
	}

	again := injectBuildLine(out)
	if again != out {
		t.Fatalf("injectBuildLine should be idempotent")
	}
}

func TestRewriteCommandSummaries(t *testing.T) {
	type fooCmd struct {
		Bar struct{} `cmd:"" help:"bar"`
	}
	root := &fooCmd{}
	parser, err := kong.New(root, kong.Writers(io.Discard, io.Discard))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	ctx, err := parser.Parse([]string{"bar"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	in := "Commands:\n  bar do-thing\n"
	out := rewriteCommandSummaries(in, ctx.Selected())
	if out == in || !bytes.Contains([]byte(out), []byte("  do-thing")) {
		t.Fatalf("unexpected rewrite: %q", out)
	}
}

func TestColorizeCommandSummaryLine(t *testing.T) {
	line := "  foo [flags]"
	out := colorizeCommandSummaryLine(line, func(s string) string { return "<" + s + ">" }, func(s string) string { return "[" + s + "]" })
	if out == line {
		t.Fatalf("expected colorized output")
	}
}

func TestGuessColumnsEnv(t *testing.T) {
	orig := os.Getenv("COLUMNS")
	t.Cleanup(func() { _ = os.Setenv("COLUMNS", orig) })

	_ = os.Setenv("COLUMNS", "123")
	if got := guessColumns(&bytes.Buffer{}); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
}

func TestHelpProfile(t *testing.T) {
	if got := helpProfile(io.Discard, "never"); got != termenv.Ascii {
		t.Fatalf("expected ascii profile")
	}
}

func TestHelpProfileNoColorEnv(t *testing.T) {
	orig := os.Getenv("NO_COLOR")
	t.Cleanup(func() { _ = os.Setenv("NO_COLOR", orig) })

	_ = os.Setenv("NO_COLOR", "1")
	if got := helpProfile(io.Discard, "always"); got != termenv.Ascii {
		t.Fatalf("expected ascii profile with NO_COLOR")
	}
}

func TestHelpProfileAlways(t *testing.T) {
	orig := os.Getenv("NO_COLOR")
	t.Cleanup(func() { _ = os.Setenv("NO_COLOR", orig) })

	_ = os.Setenv("NO_COLOR", "")
	if got := helpProfile(io.Discard, "always"); got != termenv.TrueColor {
		t.Fatalf("expected truecolor profile")
	}
}

func TestHelpOptionsEnv(t *testing.T) {
	orig := os.Getenv("GOG_HELP")
	t.Cleanup(func() { _ = os.Setenv("GOG_HELP", orig) })

	_ = os.Setenv("GOG_HELP", "full")
	if opts := helpOptions(); opts.NoExpandSubcommands {
		t.Fatalf("expected full help to expand subcommands")
	}
}

func TestColorizeHelp(t *testing.T) {
	in := "Usage: gog\nCommands:\n  foo [flags]\n"
	out := colorizeHelp(in, termenv.TrueColor)
	if out == in {
		t.Fatalf("expected colorized output")
	}
}

func TestRemoveEmptyCommandGroups(t *testing.T) {
	in := "Read\n  search [flags]\n    Search\n\nOrganize\n"
	out := removeEmptyCommandGroups(in)
	if strings.Contains(out, "Organize") {
		t.Fatalf("expected empty group removed, got: %q", out)
	}
	if !strings.Contains(out, "Read") || !strings.Contains(out, "search") {
		t.Fatalf("expected non-empty group retained, got: %q", out)
	}
}
