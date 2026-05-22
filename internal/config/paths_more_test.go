//nolint:wsl_v5
package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDerivedPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))

	base, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	dataBase, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	stateBase, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}

	keyringDir, err := KeyringDir()
	if err != nil {
		t.Fatalf("KeyringDir: %v", err)
	}

	if !strings.HasPrefix(keyringDir, dataBase) {
		t.Fatalf("expected keyring under %q, got %q", dataBase, keyringDir)
	}

	watchDir, err := GmailWatchDir()
	if err != nil {
		t.Fatalf("GmailWatchDir: %v", err)
	}

	if !strings.HasPrefix(watchDir, stateBase) {
		t.Fatalf("expected watch dir under %q, got %q", stateBase, watchDir)
	}

	attachmentsDir, err := GmailAttachmentsDir()
	if err != nil {
		t.Fatalf("GmailAttachmentsDir: %v", err)
	}

	if !strings.HasPrefix(attachmentsDir, base) {
		t.Fatalf("expected attachments dir under %q, got %q", base, attachmentsDir)
	}

	downloadsDir, err := DriveDownloadsDir()
	if err != nil {
		t.Fatalf("DriveDownloadsDir: %v", err)
	}

	if !strings.HasPrefix(downloadsDir, base) {
		t.Fatalf("expected downloads dir under %q, got %q", base, downloadsDir)
	}
}

func TestGOGHomeSplitsConfigDataStateCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOG_HOME", filepath.Join(home, "gog-home"))
	t.Setenv("GOG_CONFIG_DIR", "")
	t.Setenv("GOG_DATA_DIR", "")
	t.Setenv("GOG_STATE_DIR", "")
	t.Setenv("GOG_CACHE_DIR", "")

	tests := []struct {
		name string
		fn   func() (string, error)
		want string
	}{
		{name: "config", fn: Dir, want: filepath.Join(home, "gog-home", "config")},
		{name: "data", fn: DataDir, want: filepath.Join(home, "gog-home", "data")},
		{name: "state", fn: StateDir, want: filepath.Join(home, "gog-home", "state")},
		{name: "cache", fn: CacheDir, want: filepath.Join(home, "gog-home", "cache")},
		{name: "credentials", fn: ClientCredentialsPath, want: filepath.Join(home, "gog-home", "data", "credentials.json")},
		{name: "keyring", fn: KeyringDir, want: filepath.Join(home, "gog-home", "data", "keyring")},
		{name: "watch", fn: GmailWatchDir, want: filepath.Join(home, "gog-home", "state", "gmail-watch")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn()
			if err != nil {
				t.Fatalf("path: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGOGPerKindOverrideWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOG_HOME", filepath.Join(home, "gog-home"))
	t.Setenv("GOG_DATA_DIR", filepath.Join(home, "data-direct"))

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if dataDir != filepath.Join(home, "data-direct") {
		t.Fatalf("unexpected data dir: %q", dataDir)
	}

	credentialsPath, err := ClientCredentialsPath()
	if err != nil {
		t.Fatalf("ClientCredentialsPath: %v", err)
	}
	if credentialsPath != filepath.Join(home, "data-direct", "credentials.json") {
		t.Fatalf("unexpected credentials path: %q", credentialsPath)
	}
}

func TestXDGDataKeepsLegacyKeyringFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))

	legacyDir := filepath.Join(home, "xdg-config", AppName, "keyring")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy keyring: %v", err)
	}

	keyringDir, err := KeyringDir()
	if err != nil {
		t.Fatalf("KeyringDir: %v", err)
	}
	if keyringDir != legacyDir {
		t.Fatalf("got %q, want legacy keyring %q", keyringDir, legacyDir)
	}
}

func TestXDGDataPrefersLegacyKeyringWhenBothExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))

	legacyDir := filepath.Join(home, "xdg-config", AppName, "keyring")
	primaryDir := filepath.Join(home, "xdg-data", AppName, "keyring")
	for _, dir := range []string{legacyDir, primaryDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir keyring dir: %v", err)
		}
	}

	keyringDir, err := KeyringDir()
	if err != nil {
		t.Fatalf("KeyringDir: %v", err)
	}
	if keyringDir != legacyDir {
		t.Fatalf("got %q, want legacy keyring %q", keyringDir, legacyDir)
	}
}

func TestXDGStateKeepsLegacyGmailWatchFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))

	legacyDir := filepath.Join(home, "xdg-config", AppName, "state", "gmail-watch")
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatalf("mkdir legacy watch dir: %v", err)
	}

	watchDir, err := GmailWatchDir()
	if err != nil {
		t.Fatalf("GmailWatchDir: %v", err)
	}
	if watchDir != legacyDir {
		t.Fatalf("got %q, want legacy watch dir %q", watchDir, legacyDir)
	}
}

func TestGOGOverrideRejectsRelativePath(t *testing.T) {
	t.Setenv("GOG_DATA_DIR", "relative")

	if _, err := DataDir(); err == nil || !strings.Contains(err.Error(), "GOG_DATA_DIR") {
		t.Fatalf("expected relative override error, got %v", err)
	}
}

func TestRelativeXDGConfigAndCacheAreIgnored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "relative-config")
	t.Setenv("XDG_CACHE_HOME", "relative-cache")

	configDir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if !filepath.IsAbs(configDir) || strings.Contains(configDir, "relative-config") {
		t.Fatalf("unexpected config dir: %q", configDir)
	}

	cacheDir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if !filepath.IsAbs(cacheDir) || strings.Contains(cacheDir, "relative-cache") {
		t.Fatalf("unexpected cache dir: %q", cacheDir)
	}
}

func TestXDGKindEnvPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg-data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "xdg-state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))

	assertPath := func(name string, got string, want string) {
		t.Helper()
		if got != want {
			t.Fatalf("%s: got %q, want %q", name, got, want)
		}
	}
	configDir, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	stateDir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	cacheDir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	assertPath("config", configDir, filepath.Join(home, "xdg-config", AppName))
	assertPath("data", dataDir, filepath.Join(home, "xdg-data", AppName))
	assertPath("state", stateDir, filepath.Join(home, "xdg-state", AppName))
	assertPath("cache", cacheDir, filepath.Join(home, "xdg-cache", AppName))
}

func TestKeepServiceAccountLegacyPathMore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path, err := KeepServiceAccountLegacyPath("A@B.com")
	if err != nil {
		t.Fatalf("KeepServiceAccountLegacyPath: %v", err)
	}

	if !strings.Contains(filepath.Base(path), "keep-sa-A@B.com") {
		t.Fatalf("unexpected legacy filename: %q", filepath.Base(path))
	}
}

func TestKeepServiceAccountPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path, err := KeepServiceAccountPath("A@B.com")
	if err != nil {
		t.Fatalf("KeepServiceAccountPath: %v", err)
	}

	expected := base64.RawURLEncoding.EncodeToString([]byte("a@b.com"))
	if !strings.Contains(filepath.Base(path), "keep-sa-"+expected) {
		t.Fatalf("unexpected service account path: %q", filepath.Base(path))
	}
}

func TestServiceAccountPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path, err := ServiceAccountPath("A@B.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}

	expected := base64.RawURLEncoding.EncodeToString([]byte("a@b.com"))
	if !strings.Contains(filepath.Base(path), "sa-"+expected) {
		t.Fatalf("unexpected service account path: %q", filepath.Base(path))
	}
}

func TestListServiceAccountEmails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	dir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	enc := base64.RawURLEncoding.EncodeToString([]byte("user@example.com"))
	if writeErr := os.WriteFile(filepath.Join(dir, "sa-"+enc+".json"), []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write sa file: %v", writeErr)
	}

	if writeErr := os.WriteFile(filepath.Join(dir, "keep-sa-"+enc+".json"), []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write keep-sa file: %v", writeErr)
	}

	if writeErr := os.WriteFile(filepath.Join(dir, "keep-sa-Other@Example.com.json"), []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write legacy keep-sa file: %v", writeErr)
	}

	emails, err := ListServiceAccountEmails()
	if err != nil {
		t.Fatalf("ListServiceAccountEmails: %v", err)
	}

	if !strings.Contains(strings.Join(emails, ","), "user@example.com") || !strings.Contains(strings.Join(emails, ","), "other@example.com") {
		t.Fatalf("unexpected emails: %#v", emails)
	}
}

func TestRemoveServiceAccountFiles_RemovesRawLegacyKeepPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	dir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	path := filepath.Join(dir, "keep-sa-User@Example.com.json")
	if writeErr := os.WriteFile(path, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write legacy keep-sa file: %v", writeErr)
	}

	removed, err := RemoveServiceAccountFiles("User@Example.com")
	if err != nil {
		t.Fatalf("RemoveServiceAccountFiles: %v", err)
	}
	if !removed {
		t.Fatalf("expected legacy keep-sa file to be removed")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected legacy keep-sa file removed, stat err: %v", statErr)
	}
}

func TestRemoveServiceAccountFiles_SkipsUnsafeRawLegacyKeepPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	dir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	victim := filepath.Join(dir, "keep-sa-victim@example.com.json")
	if writeErr := os.WriteFile(victim, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write victim keep-sa file: %v", writeErr)
	}

	if _, err := RemoveServiceAccountFiles("a/../victim@example.com"); err != nil {
		t.Fatalf("RemoveServiceAccountFiles: %v", err)
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Fatalf("expected victim file to remain, stat err: %v", statErr)
	}
}

func TestExistingServiceAccountPathExplicitDataSkipsLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_DATA_DIR", filepath.Join(home, "isolated-data"))

	legacyPath, err := ServiceAccountLegacyPath("user@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountLegacyPath: %v", err)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(legacyPath), 0o700); mkdirErr != nil {
		t.Fatalf("mkdir legacy service account: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(legacyPath, []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write legacy service account: %v", writeErr)
	}

	got, err := ExistingServiceAccountPath("user@example.com")
	if err != nil {
		t.Fatalf("ExistingServiceAccountPath: %v", err)
	}
	if got == legacyPath {
		t.Fatalf("expected explicit data dir to skip legacy path %q", legacyPath)
	}
}

func TestListServiceAccountEmailsExplicitDataSkipsLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("GOG_DATA_DIR", filepath.Join(home, "isolated-data"))

	legacyDir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	enc := base64.RawURLEncoding.EncodeToString([]byte("legacy@example.com"))
	if writeErr := os.WriteFile(filepath.Join(legacyDir, "sa-"+enc+".json"), []byte(`{"type":"service_account"}`), 0o600); writeErr != nil {
		t.Fatalf("write legacy service account: %v", writeErr)
	}

	emails, err := ListServiceAccountEmails()
	if err != nil {
		t.Fatalf("ListServiceAccountEmails: %v", err)
	}
	if len(emails) != 0 {
		t.Fatalf("expected no legacy service accounts with explicit data dir, got %#v", emails)
	}
}
