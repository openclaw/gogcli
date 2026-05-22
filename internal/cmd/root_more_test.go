package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/config"
)

func setTestConfigHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
}

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
	setTestConfigHome(t)
	t.Setenv("GOG_KEYRING_BACKEND", "auto")

	out := helpDescription()
	if !strings.Contains(out, "Config:") {
		t.Fatalf("expected config block, got: %q", out)
	}
	if !strings.Contains(out, "keyring backend: auto") {
		t.Fatalf("expected keyring backend line, got: %q", out)
	}
}

func TestRootHomeFlagOverridesPathRoot(t *testing.T) {
	setTestConfigHome(t)
	home := t.TempDir()

	out := captureStdout(t, func() {
		if err := Execute([]string{"--json", "--home", home, "config", "path"}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal config path: %v\n%s", err, out)
	}
	assertPathUnderRoot(t, payload.Path, home, "config", "config.json")
}

func TestRootHomeFlagOverridesHelpDescription(t *testing.T) {
	setTestConfigHome(t)
	home := t.TempDir()

	out := captureStdout(t, func() {
		if err := Execute([]string{"--home", home, "--help"}); err != nil {
			t.Fatalf("Execute: %v", err)
		}
	})
	path := helpConfigPath(out)
	if path == "" {
		t.Fatalf("expected help config path, got %s", out)
	}
	assertPathUnderRoot(t, path, home, "config", "config.json")
}

func TestRootHomeFlagAppliesBeforeSubcommandHelp(t *testing.T) {
	setTestConfigHome(t)

	err := Execute([]string{"config", "--home", "relative", "--help"})
	if err == nil || !strings.Contains(err.Error(), "--home") {
		t.Fatalf("expected --home error, got %v", err)
	}
}

func helpConfigPath(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "file: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "file: "))
		}
	}
	return ""
}

func assertPathUnderRoot(t *testing.T, got, root string, elems ...string) {
	t.Helper()

	cleanGot := filepath.Clean(got)
	if !strings.HasSuffix(cleanGot, filepath.Join(elems...)) {
		t.Fatalf("expected path with suffix %q, got %q", filepath.Join(elems...), got)
	}

	gotRoot := cleanGot
	for range elems {
		gotRoot = filepath.Dir(gotRoot)
	}
	if samePath(gotRoot, root) {
		return
	}
	t.Fatalf("expected path root %q, got %q from path %q", root, gotRoot, got)
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if aEval, err := filepath.EvalSymlinks(a); err == nil {
		a = filepath.Clean(aEval)
	}
	if bEval, err := filepath.EvalSymlinks(b); err == nil {
		b = filepath.Clean(bEval)
	}
	return strings.EqualFold(a, b)
}

func TestRootHomePreScanSkipsGlobalFlagValues(t *testing.T) {
	if home, ok := preScanHomeArg([]string{"--account", "--home", "config", "path"}); ok {
		t.Fatalf("unexpected home override %q", home)
	}

	home, ok := preScanHomeArg([]string{"--account", "user@example.com", "--home=/tmp/gog", "config", "path"})
	if !ok || home != "/tmp/gog" {
		t.Fatalf("home=%q ok=%t, want /tmp/gog true", home, ok)
	}

	home, ok = preScanHomeArg([]string{"config", "--home", "/tmp/gog", "--help"})
	if !ok || home != "/tmp/gog" {
		t.Fatalf("home=%q ok=%t, want /tmp/gog true", home, ok)
	}
}

func TestRootHomeFlagRejectsRelativePath(t *testing.T) {
	setTestConfigHome(t)
	err := Execute([]string{"--home", "relative", "config", "path"})
	if err == nil || !strings.Contains(err.Error(), "--home") {
		t.Fatalf("expected --home error, got %v", err)
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

func TestEnableCommandsAllowsDottedSubcommand(t *testing.T) {
	setTestConfigHome(t)
	err := Execute([]string{"--enable-commands", "config.no-send", "config", "no-send", "list"})
	if err != nil {
		t.Fatalf("expected dotted allowlist to permit command, got %v", err)
	}
}

func TestDisableCommandsBlocksDottedSubcommand(t *testing.T) {
	setTestConfigHome(t)
	err := Execute([]string{"--disable-commands", "config.no-send", "config", "no-send", "list"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommandFiltersHonorCanonicalAlias(t *testing.T) {
	setTestConfigHome(t)
	if err := Execute([]string{"--enable-commands", "docs.set-page-layout", "docs", "page-layout", "doc1", "--dry-run"}); err != nil {
		t.Fatalf("expected alias allowlist to permit canonical command, got %v", err)
	}

	err := Execute([]string{"--disable-commands", "docs.set-page-layout", "docs", "page-layout", "doc1", "--dry-run"})
	if err == nil {
		t.Fatalf("expected alias denylist to block canonical command")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGmailNoSendBlocksBeforeAuth(t *testing.T) {
	setTestConfigHome(t)
	tests := [][]string{
		{"--gmail-no-send", "gmail", "send", "--to", "a@example.com", "--subject", "S", "--body", "B"},
		{"--gmail-no-send", "gmail", "autoreply", "from:a@example.com", "--subject", "S", "--body", "B"},
		{"--gmail-no-send", "gmail", "forward", "msg-1", "--to", "a@example.com"},
		{"--gmail-no-send", "gmail", "fwd", "msg-1", "--to", "a@example.com"},
		{"--gmail-no-send", "gmail", "drafts", "send", "draft-1"},
	}
	for _, args := range tests {
		err := Execute(args)
		if err == nil {
			t.Fatalf("expected error for %v", args)
		}
		if !strings.Contains(err.Error(), "no-send") {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
	}
}

func TestConfigGmailNoSendBlocksBeforeAuth(t *testing.T) {
	setTestConfigHome(t)
	if err := config.WriteConfig(config.File{GmailNoSend: true}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	tests := [][]string{
		{"gmail", "send", "--to", "a@example.com", "--subject", "S", "--body", "B"},
		{"gmail", "autoreply", "from:a@example.com", "--subject", "S", "--body", "B"},
		{"gmail", "forward", "msg-1", "--to", "a@example.com"},
		{"gmail", "drafts", "send", "draft-1"},
	}
	for _, args := range tests {
		err := Execute(args)
		if err == nil {
			t.Fatalf("expected error for %v", args)
		}
		if !strings.Contains(err.Error(), "gmail_no_send") {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
	}
}
