package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/degree-analytics/ratatosk/internal/config"
	"github.com/degree-analytics/ratatosk/internal/secrets"
)

func TestAuthServiceAccountSet_AndList_Text(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Service account configured") {
		t.Fatalf("unexpected output: %q", out)
	}

	storedPath, err := config.ServiceAccountPath("user@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("expected stored key at %q: %v", storedPath, err)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})
	if !strings.Contains(listOut, "user@example.com") || !strings.Contains(listOut, "service_account") {
		t.Fatalf("unexpected list output: %q", listOut)
	}
}

func TestAuthStatus_ShowsServiceAccountPreferred(t *testing.T) {
	origOpen := openSecretsStore
	t.Cleanup(func() { openSecretsStore = origOpen })

	store := newMemSecretsStore()
	openSecretsStore = func() (secrets.Store, error) { return store, nil }

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"--account", "user@example.com", "auth", "status"}); err != nil {
				t.Fatalf("status: %v", err)
			}
		})
	})
	if !strings.Contains(out, "auth_preferred\tservice_account") {
		t.Fatalf("unexpected status output: %q", out)
	}
}
