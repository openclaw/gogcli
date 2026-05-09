package tracking

import (
	"os"
	"strings"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	setupTrackingConfigEnv(t)

	account := "test@example.com"

	if err := SaveSecrets(account, "testkey123", "adminkey456"); err != nil {
		t.Fatalf("SaveSecrets failed: %v", err)
	}

	cfg := &Config{
		Enabled:          true,
		WorkerURL:        "https://test.workers.dev",
		WorkerName:       "gog-email-tracker-test",
		DatabaseName:     "gog-email-tracker-test",
		DatabaseID:       "db-id-123",
		SecretsInKeyring: true,
	}

	if err := SaveConfig(account, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(account)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.WorkerURL != cfg.WorkerURL {
		t.Errorf("WorkerURL mismatch: got %q, want %q", loaded.WorkerURL, cfg.WorkerURL)
	}

	if loaded.WorkerName != cfg.WorkerName {
		t.Errorf("WorkerName mismatch: got %q, want %q", loaded.WorkerName, cfg.WorkerName)
	}

	if loaded.DatabaseID != cfg.DatabaseID {
		t.Errorf("DatabaseID mismatch: got %q, want %q", loaded.DatabaseID, cfg.DatabaseID)
	}

	if loaded.TrackingKey != "testkey123" {
		t.Errorf("TrackingKey mismatch: got %q, want %q", loaded.TrackingKey, "testkey123")
	}

	if loaded.AdminKey != "adminkey456" {
		t.Errorf("AdminKey mismatch: got %q, want %q", loaded.AdminKey, "adminkey456")
	}

	if !loaded.IsConfigured() {
		t.Error("IsConfigured should return true")
	}

	path, pathErr := ConfigPath()
	if pathErr != nil {
		t.Fatalf("ConfigPath: %v", pathErr)
	}

	b, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	s := string(b)
	if strings.Contains(s, "tracking_key") || strings.Contains(s, "admin_key") {
		t.Fatalf("expected secrets omitted from config file, got:\n%s", s)
	}

	if !strings.Contains(s, "\"version\"") || !strings.Contains(s, "\"updated_at\"") {
		t.Fatalf("expected version metadata in config file, got:\n%s", s)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	setupTrackingConfigEnv(t)

	cfg, err := LoadConfig("missing@example.com")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Enabled {
		t.Error("Expected Enabled to be false for missing config")
	}

	if cfg.IsConfigured() {
		t.Error("IsConfigured should return false for missing config")
	}
}

func TestLoadConfigDifferentAccount(t *testing.T) {
	setupTrackingConfigEnv(t)

	cfg := &Config{Enabled: true, WorkerURL: "https://test.workers.dev"}
	if err := SaveConfig("a@example.com", cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig("b@example.com")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Enabled {
		t.Error("Expected Enabled to be false for missing account config")
	}
}
