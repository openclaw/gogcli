package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestAuthKeyringSet_WritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")
	t.Setenv("GOG_KEYRING_PASSWORD", "")

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui new: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	ctx = withTestRuntime(ctx, func(*app.Runtime) {})

	if err = runKong(t, &AuthKeyringCmd{}, []string{"file"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	store := defaultConfigStoreForTest(t)
	path := store.Path()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Contains(b, []byte(`"keyring_backend": "file"`)) {
		t.Fatalf("expected keyring_backend=file, got:\n%s", string(b))
	}

	info, err := secrets.ResolveKeyringBackendInfoWithOptions(secrets.OpenOptions{
		Layout:  store.Layout(),
		Config:  store,
		Backend: "",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if info.Value != "file" || info.Source != "config" {
		t.Fatalf("expected file/config, got %q/%q", info.Value, info.Source)
	}
}

func TestAuthKeyring_WritesConfig_Shorthand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")
	t.Setenv("GOG_KEYRING_PASSWORD", "")

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui new: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	ctx = withTestRuntime(ctx, func(*app.Runtime) {})

	if err = runKong(t, &AuthKeyringCmd{}, []string{"set", "file"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	cfg, err := defaultConfigStoreForTest(t).Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if cfg.KeyringBackend != "file" {
		t.Fatalf("expected file, got %q", cfg.KeyringBackend)
	}
}

func TestAuthKeyringWritesInjectedConfigStore(t *testing.T) {
	t.Setenv("GOG_KEYRING_BACKEND", "")

	root := t.TempDir()
	layout := config.Layout{ConfigDir: filepath.Join(root, "config")}
	store := config.NewConfigStore(layout)

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui new: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	ctx = withTestRuntime(ctx, func(*app.Runtime) {})
	ctx = app.WithRuntime(ctx, &app.Runtime{Layout: layout, Config: store})

	if err = runKong(t, &AuthKeyringCmd{}, []string{"file"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("read injected config: %v", err)
	}
	if cfg.KeyringBackend != "file" {
		t.Fatalf("keyring backend = %q, want file", cfg.KeyringBackend)
	}
}

func TestAuthKeyring_FileBackendPasswordHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui new: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})
	ctx = withTestRuntime(ctx, func(*app.Runtime) {})

	t.Setenv("GOG_KEYRING_PASSWORD", "pw")
	if err = runKong(t, &AuthKeyringCmd{}, []string{"file"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("GOG_KEYRING_PASSWORD found in environment")) {
		t.Fatalf("expected password env note, got:\n%s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()

	t.Setenv("GOG_KEYRING_PASSWORD", "")
	if err = runKong(t, &AuthKeyringCmd{}, []string{"file"}, ctx, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("requires GOG_KEYRING_PASSWORD")) &&
		!bytes.Contains(stderr.Bytes(), []byte("Hint: set GOG_KEYRING_PASSWORD")) {
		t.Fatalf("expected password hint, got:\n%s", stderr.String())
	}
}

func TestAuthKeyringSet_InvalidBackend(t *testing.T) {
	var stdout, stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &stdout, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui new: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{})

	err = runKong(t, &AuthKeyringCmd{}, []string{"nope"}, ctx, nil)
	if err == nil {
		t.Fatalf("expected error")
	}

	var ee *ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("expected usage exit 2, got: %v", err)
	}
}
