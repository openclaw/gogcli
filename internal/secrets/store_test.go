package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/steipete/gogcli/internal/config"
)

var errKeyringOpenBlocked = errors.New("keyring open blocked")

// keyringConfig creates a keyring.Config for testing.
// KeychainTrustApplication is false to match production config (see store.go).
func keyringConfig(keyringDir string) keyring.Config {
	return keyring.Config{
		ServiceName:              keyringServiceName(),
		KeychainTrustApplication: false,
		AllowedBackends:          []keyring.BackendType{keyring.FileBackend},
		FileDir:                  keyringDir,
		FilePasswordFunc:         fileKeyringPasswordFunc(),
	}
}

func TestKeyringServiceName(t *testing.T) {
	t.Setenv(keyringServiceNameEnv, "")

	if got := keyringServiceName(); got != config.AppName {
		t.Fatalf("expected default service name %q, got %q", config.AppName, got)
	}

	t.Setenv(keyringServiceNameEnv, " custom-gog ")

	if got := keyringServiceName(); got != "custom-gog" {
		t.Fatalf("expected env service name, got %q", got)
	}
}

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

	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err = os.WriteFile(path, []byte(`{ keyring_backend: "file" }`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
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

	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err = os.WriteFile(path, []byte(`{ keyring_backend: "file" }`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
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

func TestKeyringDbusGuards(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		backend     string
		dbusAddr    string
		wantForce   bool
		wantTimeout bool
	}{
		{
			name:        "linux auto no dbus",
			goos:        "linux",
			backend:     "auto",
			dbusAddr:    "",
			wantForce:   true,
			wantTimeout: false,
		},
		{
			name:        "linux auto with dbus",
			goos:        "linux",
			backend:     "auto",
			dbusAddr:    "unix:path=/run/user/1000/bus",
			wantForce:   false,
			wantTimeout: true,
		},
		{
			name:        "windows auto no dbus",
			goos:        "windows",
			backend:     "auto",
			dbusAddr:    "",
			wantForce:   false,
			wantTimeout: false,
		},
		{
			name:        "linux explicit file no dbus",
			goos:        "linux",
			backend:     "file",
			dbusAddr:    "",
			wantForce:   false,
			wantTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := KeyringBackendInfo{Value: tt.backend}
			if got := shouldForceFileBackend(tt.goos, info, tt.dbusAddr); got != tt.wantForce {
				t.Fatalf("shouldForceFileBackend=%v, want %v", got, tt.wantForce)
			}

			if got := shouldUseKeyringTimeout(tt.goos, info, tt.dbusAddr); got != tt.wantTimeout {
				t.Fatalf("shouldUseKeyringTimeout=%v, want %v", got, tt.wantTimeout)
			}
		})
	}
}

func TestOpenKeyringWithTimeout_Success(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")

	keyringDir, err := config.EnsureKeyringDir()
	if err != nil {
		t.Fatalf("EnsureKeyringDir: %v", err)
	}

	cfg := keyringConfig(keyringDir)

	// Should complete well within the timeout
	ring, err := openKeyringWithTimeout(cfg, 5*time.Second)
	if err != nil {
		t.Fatalf("openKeyringWithTimeout: %v", err)
	}

	if ring == nil {
		t.Fatal("expected non-nil keyring")
	}
}

func TestOpenKeyringWithTimeout_Timeout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")

	keyringDir, err := config.EnsureKeyringDir()
	if err != nil {
		t.Fatalf("EnsureKeyringDir: %v", err)
	}

	cfg := keyringConfig(keyringDir)

	blockCh := make(chan struct{})
	originalOpen := keyringOpenFunc
	keyringOpenFunc = func(_ keyring.Config) (keyring.Keyring, error) {
		<-blockCh
		return nil, errKeyringOpenBlocked
	}

	t.Cleanup(func() { keyringOpenFunc = originalOpen })

	_, err = openKeyringWithTimeout(cfg, 10*time.Millisecond)

	close(blockCh)

	if err == nil {
		t.Fatalf("expected timeout error")
	}

	if !errors.Is(err, errKeyringTimeout) {
		t.Fatalf("expected keyring timeout error, got: %v", err)
	}

	if !strings.Contains(err.Error(), "GOG_KEYRING_BACKEND=file") {
		t.Fatalf("expected timeout error with GOG_KEYRING_BACKEND guidance, got: %v", err)
	}
}

func TestOpenKeyring_NoDBus_ForcesFileBackend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("D-Bus detection only applies on Linux")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "")        // auto
	t.Setenv("GOG_KEYRING_PASSWORD", "testpw") // for file backend
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")   // no D-Bus

	// Should succeed using file backend (not hang on D-Bus)
	store, err := OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault with no D-Bus: %v", err)
	}

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestOpenKeyring_ExplicitBackend_IgnoresDBusDetection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_KEYRING_BACKEND", "file") // explicit file
	t.Setenv("GOG_KEYRING_PASSWORD", "testpw")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "") // no D-Bus (shouldn't matter)

	// Should succeed with explicit file backend
	store, err := OpenDefault()
	if err != nil {
		t.Fatalf("OpenDefault with explicit file backend: %v", err)
	}

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}
