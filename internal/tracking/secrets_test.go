package tracking

import (
	"path/filepath"
	"testing"

	"github.com/degree-analytics/ratatosk/internal/secrets"
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
