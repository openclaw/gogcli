package cmd

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestWrapParseError(t *testing.T) {
	if wrapParseError(nil) != nil {
		t.Fatalf("expected nil wrap")
	}

	plainErr := errors.New("plain")
	if got := wrapParseError(plainErr); !errors.Is(got, plainErr) {
		t.Fatalf("expected passthrough error")
	}

	type cli struct {
		Name string `arg:""`
	}
	parser, err := kong.New(&cli{}, kong.Writers(io.Discard, io.Discard))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	_, parseErr := parser.Parse([]string{})
	if parseErr == nil {
		t.Fatalf("expected parse error")
	}

	wrapped := wrapParseError(parseErr)
	var ee *ExitError
	if !errors.As(wrapped, &ee) || ee == nil {
		t.Fatalf("expected ExitError")
	}
	if ee.Code != 2 {
		t.Fatalf("expected code 2, got %d", ee.Code)
	}
	var pe *kong.ParseError
	if !errors.As(ee.Err, &pe) {
		t.Fatalf("expected wrapped parse error, got %v", ee.Err)
	}
}

func TestBoolString(t *testing.T) {
	if got := boolString(true); got != "true" {
		t.Fatalf("expected true, got %q", got)
	}
	if got := boolString(false); got != "false" {
		t.Fatalf("expected false, got %q", got)
	}
}

func TestHelpDescription(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "auto")

	out := helpDescription()
	if !strings.Contains(out, "Config:") {
		t.Fatalf("expected config block, got: %q", out)
	}
	if !strings.Contains(out, "keyring backend: auto") {
		t.Fatalf("expected keyring backend line, got: %q", out)
	}
}

func TestEnableCommandsBlocks(t *testing.T) {
	err := Execute([]string{"--enable-commands", "calendar", "tasks", "list", "l1"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}
