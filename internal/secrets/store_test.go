package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestResolveKeyringBackendInfo_Default(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	info, err := ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "auto" {
		t.Fatalf("expected auto, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceDefault {
		t.Fatalf("expected source default, got %q", info.Source)
	}
}

func TestResolveKeyringBackendInfo_Config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}

	if writeErr := os.WriteFile(path, []byte(`{ keyring_backend: "file" }`), 0o600); writeErr != nil {
		t.Fatalf("write config: %v", writeErr)
	}

	info, err := ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "file" {
		t.Fatalf("expected file, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceConfig {
		t.Fatalf("expected source config, got %q", info.Source)
	}
}

func TestResolveKeyringBackendInfo_EnvOverridesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "keychain")

	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}

	if writeErr := os.WriteFile(path, []byte(`{ keyring_backend: "file" }`), 0o600); writeErr != nil {
		t.Fatalf("write config: %v", writeErr)
	}

	info, err := ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "keychain" {
		t.Fatalf("expected keychain, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceEnv {
		t.Fatalf("expected source env, got %q", info.Source)
	}
}

func TestAllowedBackends_Invalid(t *testing.T) {
	_, err := allowedBackends(KeyringBackendInfo{Value: "nope"})
	if err == nil {
		t.Fatalf("expected error")
	}

	if !errors.Is(err, errInvalidKeyringBackend) {
		t.Fatalf("expected invalid backend error, got %v", err)
	}
}
