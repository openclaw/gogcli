package backup

import (
	"path/filepath"
	"testing"

	appconfig "github.com/openclaw/gogcli/internal/config"
)

func testConfigStore(t *testing.T, configPath string) *ConfigStore {
	t.Helper()

	configDir := t.TempDir()
	if configPath != "" {
		configDir = filepath.Dir(configPath)
	}
	home := t.TempDir()

	store, err := NewConfigStore(appconfig.Layout{
		ConfigDir:      configDir,
		ExplicitConfig: true,
	}, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	return store
}

func testOptions(t *testing.T, opts Options) Options {
	t.Helper()
	opts.ConfigStore = testConfigStore(t, opts.ConfigPath)

	return opts
}

func saveTestConfig(t *testing.T, path string, cfg Config) {
	t.Helper()

	if err := testConfigStore(t, path).Save(path, cfg); err != nil {
		t.Fatalf("save backup config: %v", err)
	}
}
