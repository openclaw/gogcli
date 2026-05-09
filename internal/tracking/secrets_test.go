package tracking

import (
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/internal/secrets"
)

func setupTrackingKeyringEnv(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")
}

func TestSaveAndLoadSecrets(t *testing.T) {
	setupTrackingKeyringEnv(t)

	if err := SaveSecrets("a@b.com", "track", "admin"); err != nil {
		t.Fatalf("SaveSecrets: %v", err)
	}

	track, admin, err := LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "track" || admin != "admin" {
		t.Fatalf("unexpected secrets: %q %q", track, admin)
	}
}

func TestSaveAndLoadTrackingKeys(t *testing.T) {
	setupTrackingKeyringEnv(t)

	keys := map[int]string{
		1: "track-v1",
		2: "track-v2",
	}
	if err := SaveTrackingKeys("a@b.com", keys, 2, "admin"); err != nil {
		t.Fatalf("SaveTrackingKeys: %v", err)
	}

	loaded, currentVersion, err := LoadTrackingKeys("a@b.com", []int{1, 2}, 2)
	if err != nil {
		t.Fatalf("LoadTrackingKeys: %v", err)
	}

	if currentVersion != 2 {
		t.Fatalf("current version = %d, want 2", currentVersion)
	}

	if loaded[1] != "track-v1" || loaded[2] != "track-v2" {
		t.Fatalf("unexpected tracking keys: %#v", loaded)
	}

	track, admin, err := LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "track-v2" || admin != "admin" {
		t.Fatalf("unexpected current secrets: %q %q", track, admin)
	}
}

func TestLoadSecrets_LegacyFallback(t *testing.T) {
	setupTrackingKeyringEnv(t)

	if err := secrets.SetSecret(legacyTrackingKeySecretKey, []byte("legacy-track")); err != nil {
		t.Fatalf("SetSecret legacy: %v", err)
	}

	if err := secrets.SetSecret(legacyAdminKeySecretKey, []byte("legacy-admin")); err != nil {
		t.Fatalf("SetSecret legacy admin: %v", err)
	}

	track, admin, err := LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "legacy-track" || admin != "legacy-admin" {
		t.Fatalf("unexpected legacy secrets: %q %q", track, admin)
	}
}

func TestScopedSecretKey(t *testing.T) {
	if got := scopedSecretKey(" A@B.com ", "tracking_key"); got != "tracking/A@B.com/tracking_key" {
		t.Fatalf("unexpected scoped key: %q", got)
	}
}
