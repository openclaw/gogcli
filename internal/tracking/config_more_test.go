package tracking

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func setupTrackingConfigEnv(t *testing.T) *ConfigStore {
	t.Helper()
	home := t.TempDir()

	return newTrackingConfigTestStore(
		t,
		filepath.Join(home, "xdg", config.AppName),
		filepath.Join(home, "state"),
		filepath.Join(home, "xdg"),
		false,
	)
}

func newTrackingConfigTestStore(t *testing.T, configDir, stateDir, legacyConfigBase string, explicitState bool) *ConfigStore {
	t.Helper()

	secretStore, _ := newTestSecretStore(t)

	store, err := NewConfigStore(config.Layout{
		ConfigDir:     configDir,
		StateDir:      stateDir,
		ExplicitState: explicitState,
	}, legacyConfigBase, secretStore)
	if err != nil {
		t.Fatalf("new config store: %v", err)
	}

	return store
}

func TestLoadConfigMissingReturnsDisabled(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	cfg, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Enabled {
		t.Fatalf("expected disabled config")
	}
}

func TestConfigStoreHydrationRequiresSecretStore(t *testing.T) {
	root := t.TempDir()

	store, err := NewConfigStore(config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		StateDir:       filepath.Join(root, "state"),
		ExplicitConfig: true,
		ExplicitState:  true,
	}, "", nil)
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	if err := store.Save("a@b.com", &Config{
		Enabled:          true,
		WorkerURL:        "https://example.com",
		SecretsInKeyring: true,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := store.Load("a@b.com"); !errors.Is(err, errNilSecretStore) {
		t.Fatalf("Load error = %v", err)
	}

	if _, err := store.LoadMetadata("a@b.com"); err != nil {
		t.Fatalf("LoadMetadata: %v", err)
	}
}

func TestSaveConfigSecretsInKeyring(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	if err := store.secrets.SaveSecrets("a@b.com", "track", "admin"); err != nil {
		t.Fatalf("SaveSecrets: %v", err)
	}

	cfg := &Config{
		Enabled:          true,
		WorkerURL:        "https://example.com",
		SecretsInKeyring: true,
		TrackingKey:      "should-clear",
		AdminKey:         "should-clear",
	}
	if err := store.Save("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	path := store.Path()
	var data []byte
	var readErr error

	if data, readErr = os.ReadFile(path); readErr != nil {
		t.Fatalf("read config: %v", readErr)
	}

	if strings.Contains(string(data), "tracking_key") || strings.Contains(string(data), "admin_key") {
		t.Fatalf("expected secrets omitted from config file")
	}

	loaded, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.TrackingKey != "track" || loaded.AdminKey != "admin" {
		t.Fatalf("unexpected secrets: %#v", loaded)
	}
}

func TestShouldLoadTrackingSecrets(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
		want bool
	}{
		{name: "nil", cfg: nil, want: false},
		{name: "explicit keyring", cfg: &Config{SecretsInKeyring: true, TrackingKey: "file", AdminKey: "file"}, want: true},
		{name: "legacy empty file secrets", cfg: &Config{}, want: true},
		{name: "legacy whitespace secrets", cfg: &Config{TrackingKey: " ", AdminKey: "\t"}, want: true},
		{name: "file tracking key", cfg: &Config{TrackingKey: "file"}, want: false},
		{name: "file admin key", cfg: &Config{AdminKey: "file"}, want: false},
		{name: "file both keys", cfg: &Config{TrackingKey: "file", AdminKey: "admin"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldLoadTrackingSecrets(tt.cfg); got != tt.want {
				t.Fatalf("shouldLoadTrackingSecrets = %t, want %t", got, tt.want)
			}

			if got := tt.cfg.NeedsSecretStore(); got != tt.want {
				t.Fatalf("NeedsSecretStore = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestLoadConfigPrefersFileSecretsWhenKeyringHasStaleValues(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	if err := store.secrets.SaveSecrets("a@b.com", "stale-track", "stale-admin"); err != nil {
		t.Fatalf("SaveSecrets: %v", err)
	}

	cfg := &Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: "file-track",
		AdminKey:    "file-admin",
	}
	if err := store.Save("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.TrackingKey != "file-track" || loaded.AdminKey != "file-admin" {
		t.Fatalf("expected file secrets, got %#v", loaded)
	}
}

func TestLoadConfigFallsBackToKeyringWhenLegacySecretsAreEmpty(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	if err := store.secrets.SaveSecrets("a@b.com", "track", "admin"); err != nil {
		t.Fatalf("SaveSecrets: %v", err)
	}

	cfg := &Config{
		Enabled:   true,
		WorkerURL: "https://example.com",
	}
	if err := store.Save("a@b.com", cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.TrackingKey != "track" || loaded.AdminKey != "admin" {
		t.Fatalf("expected keyring fallback, got %#v", loaded)
	}
}

func TestLoadConfigLegacyFallback(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	legacy := store.legacyPath

	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
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

	cfg, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.WorkerURL != "https://example.com" || cfg.TrackingKey != "track" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestConfigStoreFallbackPrecedence(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	write := func(path, workerURL string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}

		payload, err := json.Marshal(&Config{
			Enabled:     true,
			WorkerURL:   workerURL,
			TrackingKey: "track",
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	write(store.legacyPath, "https://legacy.example.com")
	write(store.previousPath, "https://previous.example.com")

	cfg, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("load previous: %v", err)
	}

	if cfg.WorkerURL != "https://previous.example.com" {
		t.Fatalf("worker URL = %q, want previous", cfg.WorkerURL)
	}

	write(store.path, "https://current.example.com")

	cfg, err = store.Load("a@b.com")
	if err != nil {
		t.Fatalf("load current: %v", err)
	}

	if cfg.WorkerURL != "https://current.example.com" {
		t.Fatalf("worker URL = %q, want current", cfg.WorkerURL)
	}
}

func TestConfigStoreLegacyPathIgnoresActiveConfigOverride(t *testing.T) {
	base := t.TempDir()

	store := newTrackingConfigTestStore(
		t,
		filepath.Join(base, "explicit-config"),
		filepath.Join(base, "state"),
		filepath.Join(base, "system-config"),
		false,
	)
	if strings.Contains(store.legacyPath, "explicit-config") {
		t.Fatalf("legacy path depends on active config override: %q", store.legacyPath)
	}

	if err := os.MkdirAll(filepath.Dir(store.legacyPath), 0o700); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}

	payload, err := json.Marshal(&Config{
		Enabled:     true,
		WorkerURL:   "https://legacy.example.com",
		TrackingKey: "track",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if writeErr := os.WriteFile(store.legacyPath, payload, 0o600); writeErr != nil {
		t.Fatalf("write legacy: %v", writeErr)
	}

	cfg, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.WorkerURL != "https://legacy.example.com" {
		t.Fatalf("worker URL = %q", cfg.WorkerURL)
	}
}

func TestLoadConfigSkipsLegacyFallbackWithExplicitGOGHome(t *testing.T) {
	base := t.TempDir()
	store := newTrackingConfigTestStore(
		t,
		filepath.Join(base, "config"),
		filepath.Join(base, "state"),
		"",
		true,
	)
	legacy := filepath.Join(base, "gog", "tracking.json")

	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}

	payload, err := json.Marshal(&Config{
		Enabled:     true,
		WorkerURL:   "https://legacy.example.com",
		TrackingKey: "track",
		AdminKey:    "admin",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err = os.WriteFile(legacy, payload, 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	cfg, err := store.Load("a@b.com")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Enabled || cfg.WorkerURL != "" {
		t.Fatalf("loaded legacy config despite explicit GOG_HOME: %#v", cfg)
	}
}

func TestLegacyConfigPathUsesXDGConfigHome(t *testing.T) {
	store := setupTrackingConfigEnv(t)
	path := store.legacyPath

	if !strings.Contains(path, filepath.Join("xdg", "gog", "tracking.json")) {
		t.Fatalf("expected XDG-based legacy path, got %q", path)
	}
}

func TestLegacyConfigPathIgnoresRelativeXDGConfigHome(t *testing.T) {
	base := t.TempDir()
	store := newTrackingConfigTestStore(
		t,
		filepath.Join(base, "system-config", config.AppName),
		filepath.Join(base, "state"),
		filepath.Join(base, "system-config"),
		false,
	)
	path := store.legacyPath

	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute legacy path, got %q", path)
	}

	if strings.Contains(path, "relative-xdg") {
		t.Fatalf("expected relative XDG_CONFIG_HOME to be ignored, got %q", path)
	}
}

func TestSaveConfigMissingAccount(t *testing.T) {
	store := setupTrackingConfigEnv(t)

	if err := store.Save("", &Config{}); err == nil {
		t.Fatalf("expected error")
	}
}
