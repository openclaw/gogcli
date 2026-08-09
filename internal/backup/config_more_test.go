package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	appconfig "github.com/openclaw/gogcli/internal/config"
)

var errTestHomeUnavailable = errors.New("home unavailable")

func TestNewConfigStoreRejectsRelativePaths(t *testing.T) {
	home := t.TempDir()
	if _, err := NewConfigStore(appconfig.Layout{ConfigDir: "relative"}, func() (string, error) { return home, nil }); err == nil || !strings.Contains(err.Error(), "backup config directory") {
		t.Fatalf("expected relative config dir error, got %v", err)
	}

	if _, err := NewConfigStore(appconfig.Layout{ConfigDir: t.TempDir()}, nil); err == nil || !strings.Contains(err.Error(), "backup home resolver") {
		t.Fatalf("expected missing home resolver error, got %v", err)
	}
}

func TestConfigStoreSkipsLegacyFallbackWithExplicitLayout(t *testing.T) {
	home := t.TempDir()

	legacyPath := filepath.Join(home, ".gog", "backup.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("mkdir legacy config: %v", err)
	}

	if err := os.WriteFile(legacyPath, []byte(`{"repo":"/legacy","remote":"https://legacy.example/repo.git","identity":"/legacy.key"}`), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	store, err := NewConfigStore(appconfig.Layout{
		ConfigDir:      filepath.Join(home, "isolated", "config"),
		ExplicitConfig: true,
	}, func() (string, error) { return home, nil })
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	cfg, err := store.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Repo == "/legacy" || cfg.Identity == "/legacy.key" {
		t.Fatalf("loaded legacy config despite explicit GOG_HOME: %#v", cfg)
	}
}

func TestConfigStoreLoadsLegacyConfigAndExpandsPaths(t *testing.T) {
	home := t.TempDir()

	legacyPath := filepath.Join(home, ".gog", "backup.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("mkdir legacy config: %v", err)
	}

	if err := os.WriteFile(legacyPath, []byte(`{
  "repo": "~/legacy-repo",
  "remote": "https://legacy.example/repo.git",
  "identity": "~/.gog/legacy.key"
}`), 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	store, err := NewConfigStore(appconfig.Layout{ConfigDir: t.TempDir()}, func() (string, error) {
		return home, nil
	})
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	cfg, err := store.ResolveOptions(Options{SuppressDefaultRemote: true})
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}

	if cfg.Repo != filepath.Join(home, "legacy-repo") {
		t.Fatalf("repo = %q", cfg.Repo)
	}

	if cfg.Identity != filepath.Join(home, ".gog", "legacy.key") {
		t.Fatalf("identity = %q", cfg.Identity)
	}

	if cfg.Remote != "https://legacy.example/repo.git" {
		t.Fatalf("remote = %q", cfg.Remote)
	}
}

func TestConfigStoreAbsoluteCustomConfigDoesNotResolveHome(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "backup.json")
	cfg := Config{
		Repo:     filepath.Join(t.TempDir(), "repo"),
		Identity: filepath.Join(t.TempDir(), "age.key"),
	}

	var calls atomic.Int32

	store, err := NewConfigStore(appconfig.Layout{
		ConfigDir:      t.TempDir(),
		ExplicitConfig: true,
	}, func() (string, error) {
		calls.Add(1)
		return "", errTestHomeUnavailable
	})
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	if saveErr := store.Save(configPath, cfg); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	resolved, err := store.ResolveOptions(Options{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("ResolveOptions: %v", err)
	}

	if resolved.Repo != cfg.Repo || resolved.Identity != cfg.Identity {
		t.Fatalf("resolved = %#v", resolved)
	}

	if calls.Load() != 0 {
		t.Fatalf("home resolver called %d times", calls.Load())
	}
}

func TestConfigStoreSuppressesOnlyImplicitDefaultRemote(t *testing.T) {
	home := t.TempDir()

	store, err := NewConfigStore(appconfig.Layout{
		ConfigDir:      t.TempDir(),
		ExplicitConfig: true,
	}, func() (string, error) {
		return home, nil
	})
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}

	cfg, err := store.ResolveOptions(Options{SuppressDefaultRemote: true})
	if err != nil {
		t.Fatalf("ResolveOptions default: %v", err)
	}

	if cfg.Remote != "" {
		t.Fatalf("implicit default remote = %q", cfg.Remote)
	}

	if saveErr := store.Save("", Config{Repo: "~/repo", Remote: defaultRemote, Identity: "~/.gog/age.key"}); saveErr != nil {
		t.Fatalf("Save: %v", saveErr)
	}

	cfg, err = store.ResolveOptions(Options{SuppressDefaultRemote: true})
	if err != nil {
		t.Fatalf("ResolveOptions explicit: %v", err)
	}

	if cfg.Remote != defaultRemote {
		t.Fatalf("explicit default remote = %q", cfg.Remote)
	}
}
