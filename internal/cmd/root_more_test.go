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

// ---------- Subcommand allowlist tests ----------

func TestEnableCommandsUnit(t *testing.T) {
	tests := []struct {
		name    string
		allow   string
		cmdPath []string
		want    bool
	}{
		// Backward compat: top-level entry allows all subcommands
		{"top-level gmail allows search", "gmail", []string{"gmail", "search"}, true},
		{"top-level gmail allows send", "gmail", []string{"gmail", "send"}, true},
		{"top-level gmail allows drafts create", "gmail", []string{"gmail", "drafts", "create"}, true},
		{"top-level calendar allows events", "calendar", []string{"calendar", "events"}, true},

		// Top-level blocks other top-level
		{"calendar does not allow tasks", "calendar", []string{"tasks", "list"}, false},
		{"gmail does not allow calendar", "gmail", []string{"calendar", "events"}, false},

		// Dotted path: exact subcommand
		{"gmail.search allows gmail search", "gmail.search", []string{"gmail", "search"}, true},
		{"gmail.search blocks gmail send", "gmail.search", []string{"gmail", "send"}, false},
		{"gmail.search blocks gmail drafts create", "gmail.search", []string{"gmail", "drafts", "create"}, false},

		// Dotted path: nested subcommand
		{"gmail.drafts.create allows gmail drafts create", "gmail.drafts.create", []string{"gmail", "drafts", "create"}, true},
		{"gmail.drafts.create blocks gmail drafts send", "gmail.drafts.create", []string{"gmail", "drafts", "send"}, false},
		{"gmail.drafts.create blocks gmail drafts list", "gmail.drafts.create", []string{"gmail", "drafts", "list"}, false},
		{"gmail.drafts.create blocks gmail search", "gmail.drafts.create", []string{"gmail", "search"}, false},

		// Dotted path: intermediate allows all children
		{"gmail.drafts allows gmail drafts create", "gmail.drafts", []string{"gmail", "drafts", "create"}, true},
		{"gmail.drafts allows gmail drafts send", "gmail.drafts", []string{"gmail", "drafts", "send"}, true},
		{"gmail.drafts allows gmail drafts list", "gmail.drafts", []string{"gmail", "drafts", "list"}, true},
		{"gmail.drafts blocks gmail search", "gmail.drafts", []string{"gmail", "search"}, false},
		{"gmail.drafts blocks gmail send", "gmail.drafts", []string{"gmail", "send"}, false},

		// Note: "*" and "all" are short-circuited in enforceEnabledCommands
		// before matchEnabledCommand is called, so they are not tested here.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allow := parseEnabledCommands(tt.allow)
			got := matchEnabledCommand(allow, tt.cmdPath)
			if got != tt.want {
				t.Errorf("matchEnabledCommand(%v, %v) = %v, want %v", tt.allow, tt.cmdPath, got, tt.want)
			}
		})
	}
}

func TestEnableCommandsMixed(t *testing.T) {
	// Mixed: "gmail.search,gmail.drafts.create,calendar"
	allow := parseEnabledCommands("gmail.search,gmail.drafts.create,calendar")

	tests := []struct {
		name    string
		cmdPath []string
		want    bool
	}{
		{"gmail search allowed", []string{"gmail", "search"}, true},
		{"gmail drafts create allowed", []string{"gmail", "drafts", "create"}, true},
		{"calendar events allowed", []string{"calendar", "events"}, true},
		{"calendar list allowed", []string{"calendar", "list"}, true},
		{"gmail send blocked", []string{"gmail", "send"}, false},
		{"gmail drafts send blocked", []string{"gmail", "drafts", "send"}, false},
		{"gmail drafts list blocked", []string{"gmail", "drafts", "list"}, false},
		{"tasks list blocked", []string{"tasks", "list"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchEnabledCommand(allow, tt.cmdPath)
			if got != tt.want {
				t.Errorf("matchEnabledCommand(mixed, %v) = %v, want %v", tt.cmdPath, got, tt.want)
			}
		})
	}
}

func TestEnableCommandsSubcommandE2E(t *testing.T) {
	// End-to-end: gmail.search should allow "gmail search" but block "gmail send"
	// "gmail search" requires a query arg; pass one so kong can parse it.
	// The command will fail on auth, but should NOT fail on allowlist.
	err := Execute([]string{"--enable-commands", "gmail.search", "gmail", "search", "test"})
	if err != nil && strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("gmail search should be allowed by gmail.search, got: %v", err)
	}

	// "gmail send" should be blocked by the allowlist.
	err = Execute([]string{"--enable-commands", "gmail.search", "gmail", "send", "--to", "x@x.com", "--subject", "s", "--body", "b"})
	if err == nil {
		t.Fatalf("expected error for blocked gmail send")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected 'not enabled' error, got: %v", err)
	}
}

func TestEnableCommandsDottedDraftsE2E(t *testing.T) {
	// gmail.drafts.create should allow "gmail drafts create" but block "gmail drafts send"
	err := Execute([]string{"--enable-commands", "gmail.drafts.create", "gmail", "drafts", "create", "--to", "x@x.com", "--subject", "s", "--body", "b"})
	if err != nil && strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("gmail drafts create should be allowed, got: %v", err)
	}

	// gmail drafts send should be blocked
	err = Execute([]string{"--enable-commands", "gmail.drafts.create", "gmail", "drafts", "send", "draftid"})
	if err == nil {
		t.Fatalf("expected error for blocked gmail drafts send")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected 'not enabled' error, got: %v", err)
	}
}

func TestEnableCommandsTopLevelBackwardCompat(t *testing.T) {
	// Top-level "gmail" should still allow all gmail subcommands (backward compat)
	err := Execute([]string{"--enable-commands", "gmail", "gmail", "search", "test"})
	if err != nil && strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("top-level gmail should allow gmail search, got: %v", err)
	}

	err = Execute([]string{"--enable-commands", "gmail", "gmail", "send", "--to", "x@x.com", "--subject", "s", "--body", "b"})
	if err != nil && strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("top-level gmail should allow gmail send, got: %v", err)
	}
}
