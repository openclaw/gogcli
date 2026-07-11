package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path := NewConfigStore(testSystemLayout(t, PathKindConfig)).Path()

	if filepath.Base(path) != "config.json" {
		t.Fatalf("unexpected config file: %q", filepath.Base(path))
	}

	if filepath.Base(filepath.Dir(path)) != AppName {
		t.Fatalf("unexpected config dir: %q", filepath.Dir(path))
	}
}

func TestReadConfig_Missing(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.KeyringBackend != "" {
		t.Fatalf("expected empty config, got %q", cfg.KeyringBackend)
	}
}

func TestReadConfig_JSON5(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	path := store.Path()
	data := `{
  // allow comments + trailing commas
  keyring_backend: "file",
}`

	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if got := strings.TrimSpace(cfg.KeyringBackend); got != "file" {
		t.Fatalf("expected keyring_backend=file, got %q", got)
	}
}

func TestReadConfig_MCPPolicy(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	data := `{
  mcp: {
    allow_tools: ["read"],
    accounts: {
      "personal@example.com": {
        allow_tools: ["docs.*"],
        allow_write: true,
      },
    },
  },
}`
	if err := os.WriteFile(store.Path(), []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.MCP == nil || len(cfg.MCP.AllowTools) != 1 {
		t.Fatalf("MCP policy = %#v", cfg.MCP)
	}

	personal := cfg.MCP.Accounts["personal@example.com"]
	if !personal.AllowWrite || len(personal.AllowTools) != 1 || personal.AllowTools[0] != "docs.*" {
		t.Fatalf("personal MCP policy = %#v", personal)
	}
}

func TestConfigStorePreservesExplicitEmptyMCPAllowTools(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	if err := store.Write(File{MCP: &MCPConfig{MCPPolicy: MCPPolicy{AllowTools: []string{}}}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.MCP == nil || cfg.MCP.AllowTools == nil || len(cfg.MCP.AllowTools) != 0 {
		t.Fatalf("explicit empty allow_tools was not preserved: %#v", cfg.MCP)
	}
}

func TestConfigStoreWriteAndUpdate(t *testing.T) {
	t.Parallel()

	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})
	if err := store.Write(File{KeyringBackend: "file"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := store.Update(func(cfg *File) error {
		cfg.DefaultTimezone = "UTC"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	cfg, err := store.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if cfg.KeyringBackend != "file" || cfg.DefaultTimezone != "UTC" {
		t.Fatalf("config = %#v", cfg)
	}
}
