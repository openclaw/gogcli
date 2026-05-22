package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigPathRejectsInvalidOverride(t *testing.T) {
	t.Setenv("GOG_CONFIG_DIR", "relative")

	if _, err := LoadConfig(""); err == nil || !strings.Contains(err.Error(), "GOG_CONFIG_DIR") {
		t.Fatalf("expected LoadConfig to reject invalid override, got %v", err)
	}

	if err := SaveConfig("", DefaultConfig()); err == nil || !strings.Contains(err.Error(), "GOG_CONFIG_DIR") {
		t.Fatalf("expected SaveConfig to reject invalid override, got %v", err)
	}
}

func TestLoadConfigSkipsLegacyFallbackWithExplicitGOGHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOG_HOME", filepath.Join(home, "isolated"))

	legacyPath := filepath.Join(home, ".gog", "backup.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("mkdir legacy config: %v", err)
	}

	if err := os.WriteFile(legacyPath, []byte(`{"repo":"/legacy","remote":"https://legacy.example/repo.git","identity":"/legacy.key"}`), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Repo == "/legacy" || cfg.Identity == "/legacy.key" {
		t.Fatalf("loaded legacy config despite explicit GOG_HOME: %#v", cfg)
	}
}
