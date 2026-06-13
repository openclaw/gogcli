package tracking

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	account := "test@example.com"

	if err := store.secrets.SaveSecrets(account, "testkey123", "adminkey456"); err != nil {
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

	if err := store.Save(account, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := store.Load(account)
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

	path := store.Path()

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
	store := setupTrackingConfigEnv(t)

	cfg, err := store.Load("missing@example.com")
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
	store := setupTrackingConfigEnv(t)

	cfg := &Config{Enabled: true, WorkerURL: "https://test.workers.dev"}
	if err := store.Save("a@example.com", cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := store.Load("b@example.com")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Enabled {
		t.Error("Expected Enabled to be false for missing account config")
	}
}

func TestSaveConfigReturnsReadError(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	path := store.Path()

	if mkdirErr := os.MkdirAll(path, 0o700); mkdirErr != nil {
		t.Fatalf("mkdir config path: %v", mkdirErr)
	}

	err := store.Save("a@example.com", &Config{Enabled: true, WorkerURL: "https://worker.example.com"})
	if err == nil || !strings.Contains(err.Error(), "read tracking config") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestSaveConfigConcurrentKeepsAccounts(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	const count = 12
	var wg sync.WaitGroup
	errCh := make(chan error, count)

	for i := range count {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errCh <- store.Save(
				fmt.Sprintf("user%d@example.com", i),
				&Config{Enabled: true, WorkerURL: "https://worker.example.com"},
			)
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("SaveConfig: %v", err)
		}
	}

	path := store.Path()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var fileCfg fileConfig
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(fileCfg.Accounts) != count {
		t.Fatalf("accounts=%d want %d: %#v", len(fileCfg.Accounts), count, fileCfg.Accounts)
	}
}
