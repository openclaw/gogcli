package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
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

	runtime := normalizedRuntime(newDefaultRuntime())
	if err := bindRuntimeLayoutResolver(runtime, ""); err != nil {
		t.Fatalf("bindRuntimeLayoutResolver: %v", err)
	}
	out := helpDescription(runtime)
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

func TestRootHomeEqualsFlagOverridesPathRoot(t *testing.T) {
	setTestConfigHome(t)
	home := t.TempDir()

	out := captureStdout(t, func() {
		if err := Execute([]string{"--json", "--home=" + home, "config", "path"}); err != nil {
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

func TestRootHomeFlagRejectsRelativePathWithConfigOverride(t *testing.T) {
	setTestConfigHome(t)
	t.Setenv("GOG_CONFIG_DIR", t.TempDir())

	err := Execute([]string{"--home", "relative", "config", "path"})
	if err == nil || !strings.Contains(err.Error(), "--home") {
		t.Fatalf("expected --home error, got %v", err)
	}
}

func TestExecuteHomeResolversAreIndependent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homes := []string{filepath.Join(root, "a"), filepath.Join(root, "b")}
	type result struct {
		path string
		err  error
	}
	results := make([]result, len(homes))

	var wg sync.WaitGroup
	for i, home := range homes {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			runtime := &app.Runtime{
				IO: app.IO{
					In:  strings.NewReader(""),
					Out: &stdout,
					Err: &stderr,
				},
				KeyringOptions: testKeyringOptions(),
			}
			runErr := executeWithRuntime(
				[]string{"--json", "--home", home, "config", "path"},
				runtime,
			)
			if runErr != nil {
				results[i].err = errors.New(stderr.String() + ": " + runErr.Error())
				return
			}

			var payload struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				results[i].err = err
				return
			}
			results[i].path = payload.Path
		}()
	}
	wg.Wait()

	for i, result := range results {
		if result.err != nil {
			t.Fatalf("execute %d: %v", i, result.err)
		}
		assertPathUnderRoot(t, result.path, homes[i], "config", "config.json")
	}
}

func TestVersionDoesNotRequireLayoutResolution(t *testing.T) {
	t.Parallel()

	unavailable := func() (string, error) {
		return "", errors.New("directory unavailable")
	}
	runtime := &app.Runtime{
		LayoutResolver: config.NewResolver(config.Env{}, config.UserDirs{
			GOOS:      "linux",
			HomeDir:   unavailable,
			ConfigDir: unavailable,
			CacheDir:  unavailable,
		}),
		KeyringOptions: testKeyringOptions(),
	}
	result := executeWithTestRuntime(t, []string{"--json", "version"}, runtime)
	if result.err != nil {
		t.Fatalf("version: %v\nstderr=%q", result.err, result.stderr)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("version JSON: %v\nstdout=%q", err, result.stdout)
	}
	if payload["version"] == "" {
		t.Fatalf("version payload = %#v", payload)
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
		{"--gmail-no-send", "gmail", "reply", "msg-1", "--body", "B"},
		{"--gmail-no-send", "gmail", "reply-all", "msg-1", "--body", "B"},
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
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := store.Write(config.File{GmailNoSend: true}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	runtime := &app.Runtime{Config: store}
	tests := [][]string{
		{"gmail", "send", "--to", "a@example.com", "--subject", "S", "--body", "B"},
		{"gmail", "autoreply", "from:a@example.com", "--subject", "S", "--body", "B"},
		{"gmail", "forward", "msg-1", "--to", "a@example.com"},
		{"gmail", "drafts", "send", "draft-1"},
	}
	for _, args := range tests {
		result := executeWithTestRuntime(t, args, runtime)
		err := result.err
		if err == nil {
			t.Fatalf("expected error for %v", args)
		}
		if !strings.Contains(err.Error(), "gmail_no_send") {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
	}
}

func TestConfigNoSendAccountBlocksBeforeDryRun(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := store.Write(config.File{
		AccountAliases: map[string]string{"work": "blocked@example.com"},
		NoSendAccounts: map[string]bool{"blocked@example.com": true},
	}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	runtime := &app.Runtime{Config: store}
	tests := [][]string{
		{"gmail", "send", "--account", "blocked@example.com", "--to", "a@example.com", "--subject", "S", "--body", "B", "--dry-run"},
		{"gmail", "autoreply", "from:a@example.com", "--account", "blocked@example.com", "--subject", "S", "--body", "B", "--dry-run"},
		{"gmail", "forward", "msg-1", "--account", "blocked@example.com", "--to", "a@example.com", "--dry-run"},
		{"gmail", "drafts", "send", "draft-1", "--account", "blocked@example.com", "--dry-run"},
		// Alias resolving to a guarded account is blocked too.
		{"gmail", "send", "--account", "work", "--to", "a@example.com", "--subject", "S", "--body", "B", "--dry-run"},
	}
	for _, args := range tests {
		result := executeWithTestRuntime(t, args, runtime)
		err := result.err
		if err == nil {
			t.Fatalf("expected error for %v", args)
		}
		if !strings.Contains(err.Error(), "no-send") {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
	}
}

func TestConfigNoSendAccountDoesNotOverblockDryRun(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := store.Write(config.File{NoSendAccounts: map[string]bool{"blocked@example.com": true}}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	runtime := &app.Runtime{Config: store}
	tests := [][]string{
		// Send from a different account is not blocked.
		{"gmail", "send", "--account", "other@example.com", "--to", "a@example.com", "--subject", "S", "--body", "B", "--dry-run"},
		// Non-send commands for the guarded account are not blocked.
		{"gmail", "drafts", "create", "--account", "blocked@example.com", "--to", "a@example.com", "--subject", "S", "--body", "B", "--dry-run"},
	}
	for _, args := range tests {
		result := executeWithTestRuntime(t, args, runtime)
		if result.err != nil {
			t.Fatalf("expected success for %v, got %v\nstderr=%q", args, result.err, result.stderr)
		}
	}
}

func TestGmailSendDryRunWithoutGuardsSkipsAccountResolution(t *testing.T) {
	t.Parallel()

	// No per-account guards configured: dry-run must succeed without any
	// account being resolvable (resolution would otherwise read the
	// keyring before the dry-run exit).
	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	runtime := &app.Runtime{Config: store}
	args := []string{"gmail", "send", "--to", "a@example.com", "--subject", "S", "--body", "B", "--dry-run"}
	result := executeWithTestRuntime(t, args, runtime)
	if result.err != nil {
		t.Fatalf("expected success for %v, got %v\nstderr=%q", args, result.err, result.stderr)
	}
}

func TestGmailSendDryRunWithInactiveGuardsSkipsAccountResolution(t *testing.T) {
	t.Parallel()

	store := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := store.Write(config.File{NoSendAccounts: map[string]bool{"inactive@example.com": false}}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	runtime := &app.Runtime{Config: store}
	runtime.Auth.OpenSecretsStore = func() (secrets.Store, error) {
		t.Fatal("inactive no-send entries must not trigger account resolution")
		return nil, errors.New("unexpected account resolution")
	}
	args := []string{"gmail", "send", "--to", "a@example.com", "--subject", "S", "--body", "B", "--dry-run"}
	result := executeWithTestRuntime(t, args, runtime)
	if result.err != nil {
		t.Fatalf("expected success for %v, got %v\nstderr=%q", args, result.err, result.stderr)
	}
}

func TestConfigIndependentCommandsDoNotRequireHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	for _, args := range [][]string{
		{"version"},
		{"config", "keys"},
	} {
		result := executeWithTestRuntime(t, args, &app.Runtime{})
		if result.err != nil {
			t.Fatalf("%v: %v\nstderr=%q", args, result.err, result.stderr)
		}
	}
}
