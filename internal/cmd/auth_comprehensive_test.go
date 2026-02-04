package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

// ============================================================================
// AuthAliasListCmd Tests - auth_alias.go line 23
// ============================================================================

func TestAuthAliasListCmd_TextOutput_NoAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &AuthAliasListCmd{}, []string{}, ctx, &RootFlags{}); err != nil {
		t.Fatalf("list: %v", err)
	}

	if !strings.Contains(stderr.String(), "No account aliases") {
		t.Fatalf("expected 'No account aliases' in stderr, got: %q", stderr.String())
	}
}

func TestAuthAliasListCmd_TextOutput_WithAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// First set an alias
	if err := config.SetAccountAlias("work", "work@example.com"); err != nil {
		t.Fatalf("SetAccountAlias: %v", err)
	}
	if err := config.SetAccountAlias("personal", "personal@example.com"); err != nil {
		t.Fatalf("SetAccountAlias: %v", err)
	}

	// Use captureStdout since tableWriter writes to os.Stdout
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "alias", "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})

	if !strings.Contains(out, "ALIAS") || !strings.Contains(out, "EMAIL") {
		t.Fatalf("expected table headers, got: %q", out)
	}
	if !strings.Contains(out, "work") || !strings.Contains(out, "work@example.com") {
		t.Fatalf("expected work alias in output, got: %q", out)
	}
	if !strings.Contains(out, "personal") || !strings.Contains(out, "personal@example.com") {
		t.Fatalf("expected personal alias in output, got: %q", out)
	}
}

func TestAuthAliasListCmd_JSONOutput_Empty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &AuthAliasListCmd{}, []string{}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	var resp struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if len(resp.Aliases) != 0 {
		t.Fatalf("expected empty aliases, got: %#v", resp.Aliases)
	}
}

// ============================================================================
// AuthAliasSetCmd Tests - auth_alias.go line 55
// ============================================================================

func TestAuthAliasSetCmd_TextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	var stdout bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &AuthAliasSetCmd{}, []string{"myalias", "test@example.com"}, ctx, &RootFlags{}); err != nil {
		t.Fatalf("set: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "alias\tmyalias") {
		t.Fatalf("expected 'alias\\tmyalias' in output, got: %q", out)
	}
	if !strings.Contains(out, "email\ttest@example.com") {
		t.Fatalf("expected 'email\\ttest@example.com' in output, got: %q", out)
	}
}

func TestAuthAliasSetCmd_EmptyAlias(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &AuthAliasSetCmd{Alias: "   ", Email: "test@example.com"}
	err = cmd.Run(ctx)
	if err == nil {
		t.Fatalf("expected error for empty alias")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected usage error (exit code 2), got: %v", err)
	}
}

func TestAuthAliasSetCmd_AliasWithAtSign(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &AuthAliasSetCmd{Alias: "bad@alias", Email: "test@example.com"}
	err = cmd.Run(ctx)
	if err == nil {
		t.Fatalf("expected error for alias with '@'")
	}
	if !strings.Contains(err.Error(), "must not contain '@'") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestAuthAliasSetCmd_ReservedAlias(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	for _, reserved := range []string{"auto", "AUTO", "default", "DEFAULT"} {
		cmd := &AuthAliasSetCmd{Alias: reserved, Email: "test@example.com"}
		err = cmd.Run(ctx)
		if err == nil {
			t.Fatalf("expected error for reserved alias %q", reserved)
		}
		if !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("unexpected error for %q: %v", reserved, err)
		}
	}
}

func TestAuthAliasSetCmd_EmptyEmail(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &AuthAliasSetCmd{Alias: "myalias", Email: "   "}
	err = cmd.Run(ctx)
	if err == nil {
		t.Fatalf("expected error for empty email")
	}
	if !strings.Contains(err.Error(), "empty email") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// AuthAliasUnsetCmd Tests - auth_alias.go line 89
// ============================================================================

func TestAuthAliasUnsetCmd_TextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// First set an alias
	if err := config.SetAccountAlias("todelete", "delete@example.com"); err != nil {
		t.Fatalf("SetAccountAlias: %v", err)
	}

	var stdout bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &AuthAliasUnsetCmd{}, []string{"todelete"}, ctx, &RootFlags{}); err != nil {
		t.Fatalf("unset: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "deleted\ttrue") {
		t.Fatalf("expected 'deleted\\ttrue' in output, got: %q", out)
	}
	if !strings.Contains(out, "alias\ttodelete") {
		t.Fatalf("expected 'alias\\ttodelete' in output, got: %q", out)
	}
}

func TestAuthAliasUnsetCmd_EmptyAlias(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &AuthAliasUnsetCmd{Alias: "   "}
	err = cmd.Run(ctx)
	if err == nil {
		t.Fatalf("expected error for empty alias")
	}

	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected usage error (exit code 2), got: %v", err)
	}
}

func TestAuthAliasUnsetCmd_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := &AuthAliasUnsetCmd{Alias: "nonexistent"}
	err = cmd.Run(ctx)
	if err == nil {
		t.Fatalf("expected error for nonexistent alias")
	}
	if !strings.Contains(err.Error(), "alias not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAliasUnsetCmd_JSONOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// First set an alias
	if err := config.SetAccountAlias("tounset", "unset@example.com"); err != nil {
		t.Fatalf("SetAccountAlias: %v", err)
	}

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &AuthAliasUnsetCmd{}, []string{"tounset"}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("unset: %v", err)
		}
	})

	var resp struct {
		Deleted bool   `json:"deleted"`
		Alias   string `json:"alias"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if !resp.Deleted || resp.Alias != "tounset" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

// ============================================================================
// AuthKeyringCmd Tests - auth_keyring.go line 21
// ============================================================================

func TestAuthKeyringCmd_ShowCurrentConfig_NoArgs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &AuthKeyringCmd{}, []string{}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "path\t") {
		t.Fatalf("expected 'path' in output, got: %q", out)
	}
	if !strings.Contains(out, "keyring_backend\t") {
		t.Fatalf("expected 'keyring_backend' in output, got: %q", out)
	}
	if !strings.Contains(out, "source\t") {
		t.Fatalf("expected 'source' in output, got: %q", out)
	}
	if !strings.Contains(stderr.String(), "Hint:") {
		t.Fatalf("expected hint in stderr, got: %q", stderr.String())
	}
}

func TestAuthKeyringCmd_ShowCurrentConfig_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &AuthKeyringCmd{}, []string{}, ctx, nil); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var resp struct {
		KeyringBackend string `json:"keyring_backend"`
		Source         string `json:"source"`
		Path           string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if resp.KeyringBackend != "file" {
		t.Fatalf("expected backend 'file', got: %q", resp.KeyringBackend)
	}
	if resp.Source != "env" {
		t.Fatalf("expected source 'env', got: %q", resp.Source)
	}
}

func TestAuthKeyringCmd_TooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	err = runKong(t, &AuthKeyringCmd{}, []string{"auto", "extra"}, ctx, nil)
	if err == nil {
		t.Fatalf("expected error for too many args")
	}
	if !strings.Contains(err.Error(), "too many args") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthKeyringCmd_DefaultConvertsToAuto(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err = runKong(t, &AuthKeyringCmd{}, []string{"default"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.KeyringBackend != "auto" {
		t.Fatalf("expected 'auto', got: %q", cfg.KeyringBackend)
	}
}

func TestAuthKeyringCmd_JSONOutput_AfterSet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")
	t.Setenv("GOG_KEYRING_PASSWORD", "")

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := runKong(t, &AuthKeyringCmd{}, []string{"keychain"}, ctx, nil); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var resp struct {
		Written        bool   `json:"written"`
		Path           string `json:"path"`
		KeyringBackend string `json:"keyring_backend"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if !resp.Written {
		t.Fatalf("expected written=true")
	}
	if resp.KeyringBackend != "keychain" {
		t.Fatalf("expected backend 'keychain', got: %q", resp.KeyringBackend)
	}
}

func TestAuthKeyringCmd_EnvVarOverridesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "")

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &AuthKeyringCmd{}, []string{"keychain"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	// Should warn about env var override
	if !strings.Contains(stderr.String(), "GOG_KEYRING_BACKEND=file overrides config") {
		t.Fatalf("expected env override warning in stderr, got: %q", stderr.String())
	}
}

func TestAuthKeyringCmd_NilUI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	// Test with nil UI context (u == nil)
	ctx := context.Background()

	// No args - should not panic
	if err := (&AuthKeyringCmd{}).Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}

	// With backend arg - should not panic
	if err := (&AuthKeyringCmd{Backend: "auto"}).Run(ctx); err != nil {
		t.Fatalf("run with backend: %v", err)
	}
}

// ============================================================================
// bestServiceAccountPathAndMtime Tests - auth.go line 876
// ============================================================================

func TestBestServiceAccountPathAndMtime_ServiceAccountPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	email := "test@example.com"

	// Get the expected service account path
	saPath, err := config.ServiceAccountPath(email)
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}

	// Create the config directory
	if err = os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a service account file
	if err = os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Get the file's mtime for comparison
	stat, err := os.Stat(saPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	expectedMtime := stat.ModTime()

	// Now test the function
	path, mtime, ok := bestServiceAccountPathAndMtime(email)
	if !ok {
		t.Fatalf("expected to find service account")
	}
	if path != saPath {
		t.Fatalf("expected path %q, got %q", saPath, path)
	}
	if !mtime.Equal(expectedMtime) {
		t.Fatalf("expected mtime %v, got %v", expectedMtime, mtime)
	}
}

func TestBestServiceAccountPathAndMtime_KeepServiceAccountPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	email := "keep@example.com"

	// Get the expected keep service account path
	keepPath, err := config.KeepServiceAccountPath(email)
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}

	// Create the config directory
	if err := os.MkdirAll(filepath.Dir(keepPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a keep service account file
	if err := os.WriteFile(keepPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Test the function - should find the keep path (ServiceAccountPath is checked first, then KeepServiceAccountPath)
	path, mtime, ok := bestServiceAccountPathAndMtime(email)
	if !ok {
		t.Fatalf("expected to find service account")
	}
	if path != keepPath {
		t.Fatalf("expected path %q, got %q", keepPath, path)
	}
	if mtime.IsZero() {
		t.Fatalf("expected non-zero mtime")
	}
}

func TestBestServiceAccountPathAndMtime_KeepLegacyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	email := "legacy@example.com"

	// Get the expected legacy keep service account path
	legacyPath, err := config.KeepServiceAccountLegacyPath(email)
	if err != nil {
		t.Fatalf("KeepServiceAccountLegacyPath: %v", err)
	}

	// Create the config directory
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a legacy keep service account file
	if err := os.WriteFile(legacyPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Test the function - should find the legacy path after checking the other paths
	path, mtime, ok := bestServiceAccountPathAndMtime(email)
	if !ok {
		t.Fatalf("expected to find service account")
	}
	if path != legacyPath {
		t.Fatalf("expected path %q, got %q", legacyPath, path)
	}
	if mtime.IsZero() {
		t.Fatalf("expected non-zero mtime")
	}
}

func TestBestServiceAccountPathAndMtime_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	email := "notfound@example.com"

	path, mtime, ok := bestServiceAccountPathAndMtime(email)
	if ok {
		t.Fatalf("expected not to find service account, got path=%q mtime=%v", path, mtime)
	}
	if path != "" {
		t.Fatalf("expected empty path, got %q", path)
	}
	if !mtime.IsZero() {
		t.Fatalf("expected zero mtime, got %v", mtime)
	}
}

func TestBestServiceAccountPathAndMtime_PriorityOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	email := "priority@example.com"

	// Get all paths
	saPath, _ := config.ServiceAccountPath(email)
	keepPath, _ := config.KeepServiceAccountPath(email)
	legacyPath, _ := config.KeepServiceAccountLegacyPath(email)

	// Create the config directory
	if err := os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write all three files
	for _, p := range []string{saPath, keepPath, legacyPath} {
		if err := os.WriteFile(p, []byte(`{"type":"service_account"}`), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	// Test that ServiceAccountPath takes priority
	path, _, ok := bestServiceAccountPathAndMtime(email)
	if !ok {
		t.Fatalf("expected to find service account")
	}
	if path != saPath {
		t.Fatalf("expected ServiceAccountPath %q to take priority, got %q", saPath, path)
	}

	// Remove ServiceAccountPath and check KeepServiceAccountPath takes priority
	if err := os.Remove(saPath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	path, _, ok = bestServiceAccountPathAndMtime(email)
	if !ok {
		t.Fatalf("expected to find service account")
	}
	if path != keepPath {
		t.Fatalf("expected KeepServiceAccountPath %q to take priority, got %q", keepPath, path)
	}

	// Remove KeepServiceAccountPath and check legacy path is found
	if err := os.Remove(keepPath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	path, _, ok = bestServiceAccountPathAndMtime(email)
	if !ok {
		t.Fatalf("expected to find service account")
	}
	if path != legacyPath {
		t.Fatalf("expected legacy path %q, got %q", legacyPath, path)
	}
}

// ============================================================================
// AuthStatus with Service Account Tests
// ============================================================================

func TestAuthStatus_WithServiceAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	email := "sa@example.com"
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, email, secrets.Token{RefreshToken: "rt", Email: email})
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	// Create service account file
	saPath, _ := config.ServiceAccountPath(email)
	if err := os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := (&AuthStatusCmd{}).Run(ctx, &RootFlags{Account: email}); err != nil {
			t.Fatalf("status: %v", err)
		}
	})

	var resp struct {
		Account struct {
			Email                    string `json:"email"`
			AuthPreferred            string `json:"auth_preferred"`
			ServiceAccountConfigured bool   `json:"service_account_configured"`
			ServiceAccountPath       string `json:"service_account_path"`
		} `json:"account"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if !resp.Account.ServiceAccountConfigured {
		t.Fatalf("expected service_account_configured=true")
	}
	if resp.Account.AuthPreferred != "service_account" {
		t.Fatalf("expected auth_preferred='service_account', got %q", resp.Account.AuthPreferred)
	}
	if resp.Account.ServiceAccountPath != saPath {
		t.Fatalf("expected service_account_path=%q, got %q", saPath, resp.Account.ServiceAccountPath)
	}
}

// ============================================================================
// AuthList with Service Account Tests
// ============================================================================

func TestAuthList_WithServiceAccountOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	email := "saonly@example.com"

	// Create service account file
	saPath, _ := config.ServiceAccountPath(email)
	if err := os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := (&AuthListCmd{}).Run(ctx); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	var resp struct {
		Accounts []struct {
			Email     string   `json:"email"`
			Auth      string   `json:"auth"`
			Services  []string `json:"services"`
			CreatedAt string   `json:"created_at"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if resp.Accounts[0].Email != email {
		t.Fatalf("expected email %q, got %q", email, resp.Accounts[0].Email)
	}
	if resp.Accounts[0].Auth != "service_account" {
		t.Fatalf("expected auth='service_account', got %q", resp.Accounts[0].Auth)
	}
	if resp.Accounts[0].CreatedAt == "" {
		t.Fatalf("expected created_at to be set from file mtime")
	}
}

func TestAuthList_WithOAuthAndServiceAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	email := "both@example.com"
	store := newMemSecretsStore()
	_ = store.SetToken(config.DefaultClientName, email, secrets.Token{
		RefreshToken: "rt",
		Email:        email,
		Services:     []string{"gmail"},
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	// Create service account file
	saPath, _ := config.ServiceAccountPath(email)
	if err := os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	u, _ := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := (&AuthListCmd{}).Run(ctx); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	var resp struct {
		Accounts []struct {
			Email string `json:"email"`
			Auth  string `json:"auth"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nout=%q", err, out)
	}
	if len(resp.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(resp.Accounts))
	}
	if resp.Accounts[0].Auth != "oauth+service_account" {
		t.Fatalf("expected auth='oauth+service_account', got %q", resp.Accounts[0].Auth)
	}
}

func TestAuthList_ServiceAccountCheck_Text(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origOpen := openSecretsStore
	origCheck := checkRefreshToken
	t.Cleanup(func() {
		openSecretsStore = origOpen
		checkRefreshToken = origCheck
	})

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }
	checkRefreshToken = func(context.Context, string, string, []string, time.Duration) error {
		return nil
	}

	email := "sacheck@example.com"

	// Create service account file
	saPath, _ := config.ServiceAccountPath(email)
	if err := os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "list", "--check"}); err != nil {
				t.Fatalf("list --check: %v", err)
			}
		})
	})

	// Service account should show as valid with note that it's not checked
	if !strings.Contains(out, email) {
		t.Fatalf("expected email in output: %q", out)
	}
	if !strings.Contains(out, "\ttrue\t") {
		t.Fatalf("expected true in output: %q", out)
	}
	if !strings.Contains(out, "service account (not checked)") {
		t.Fatalf("expected 'service account (not checked)' in output: %q", out)
	}
}

func TestAuthList_Text_WithServiceAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	email := "satext@example.com"

	// Create service account file
	saPath, _ := config.ServiceAccountPath(email)
	if err := os.MkdirAll(filepath.Dir(saPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(saPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})

	// Should show service account auth type
	if !strings.Contains(out, email) {
		t.Fatalf("expected email in output: %q", out)
	}
	if !strings.Contains(out, "service_account") || !strings.Contains(out, "service-account") {
		t.Fatalf("expected service_account or service-account in output: %q", out)
	}
}
