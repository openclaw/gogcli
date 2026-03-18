package tracking

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTrackingConfigEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")
}

func TestLoadConfigMissingReturnsDisabled(t *testing.T) {
	setupTrackingConfigEnv(t)

	cfg, err := LoadConfig("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Enabled {
		t.Fatalf("expected disabled config")
	}
}

func TestSaveConfigSecretsInKeyring(t *testing.T) {
	setupTrackingConfigEnv(t)

	if err := SaveSecrets("a@b.com", "track", "admin"); err != nil {
		t.Fatalf("SaveSecrets: %v", err)
	}

	cfg := &Config{
		Enabled:          true,
		WorkerURL:        "https://example.com",
		SecretsInKeyring: true,
		TrackingKey:      "should-clear",
		AdminKey:         "should-clear",
	}
	if err := SaveConfig("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	var data []byte
	var readErr error

	if data, readErr = os.ReadFile(path); readErr != nil {
		t.Fatalf("read config: %v", readErr)
	}

	if strings.Contains(string(data), "tracking_key") || strings.Contains(string(data), "admin_key") {
		t.Fatalf("expected secrets omitted from config file")
	}

	loaded, err := LoadConfig("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.TrackingKey != "track" || loaded.AdminKey != "admin" {
		t.Fatalf("unexpected secrets: %#v", loaded)
	}
}

func TestLoadConfigLegacyFallback(t *testing.T) {
	setupTrackingConfigEnv(t)

	legacy, err := legacyConfigPath()
	if err != nil {
		t.Fatalf("legacyConfigPath: %v", err)
	}

	if err = os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}

	payload, err := json.Marshal(&Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: "track",
		AdminKey:    "admin",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err = os.WriteFile(legacy, payload, 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	cfg, err := LoadConfig("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.WorkerURL != "https://example.com" || cfg.TrackingKey != "track" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLegacyConfigPathUsesXDGConfigHome(t *testing.T) {
	setupTrackingConfigEnv(t)

	path, err := legacyConfigPath()
	if err != nil {
		t.Fatalf("legacyConfigPath: %v", err)
	}

	if !strings.Contains(path, filepath.Join("xdg", "gog", "tracking.json")) {
		t.Fatalf("expected XDG-based legacy path, got %q", path)
	}
}

func TestSaveConfigMissingAccount(t *testing.T) {
	setupTrackingConfigEnv(t)

	if err := SaveConfig("", &Config{}); err == nil {
		t.Fatalf("expected error")
	}
}
