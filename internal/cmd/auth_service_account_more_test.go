package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
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

func TestAuthServiceAccountSet_ReadsKeyFromStdin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	key := `{"type":"service_account","client_email":"svc-stdin@example.com","client_id":"stdin-123"}`
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			withStdin(t, key, func() {
				if err := Execute([]string{"auth", "service-account", "set", "stdin@example.com", "--key", "-"}); err != nil {
					t.Fatalf("Execute --key=-: %v", err)
				}
			})
		})
	})
	if !strings.Contains(out, "client_email\tsvc-stdin@example.com") {
		t.Fatalf("unexpected output: %q", out)
	}

	storedPath, err := config.ServiceAccountPath("stdin@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	if !strings.Contains(string(stored), "svc-stdin@example.com") {
		t.Fatalf("stored key did not come from stdin: %s", stored)
	}
}

func TestAuthServiceAccountSet_ReadsKeyFromEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("SA_JSON", `{"type":"service_account","client_email":"svc-env@example.com","client_id":"env-123"}`)

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "set", "env@example.com", "--key-env", "SA_JSON"}); err != nil {
				t.Fatalf("Execute --key-env: %v", err)
			}
		})
	})
	if !strings.Contains(out, "client_id\tenv-123") {
		t.Fatalf("unexpected output: %q", out)
	}

	storedPath, err := config.ServiceAccountPath("env@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	if !strings.Contains(string(stored), "svc-env@example.com") {
		t.Fatalf("stored key did not come from env: %s", stored)
	}
}

func TestAuthServiceAccountSet_RequiresOneKeySource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	t.Setenv("SA_JSON", `{"type":"service_account"}`)

	if err := Execute([]string{"auth", "service-account", "set", "missing@example.com"}); err == nil {
		t.Fatal("expected missing key source error")
	}
	if err := Execute([]string{"auth", "service-account", "set", "conflict@example.com", "--key", "-", "--key-env", "SA_JSON"}); err == nil {
		t.Fatal("expected conflicting key source error")
	}
}

func TestAuthServiceAccountStatus_MissingTextHasHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "status", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	for _, want := range []string{
		"email\tuser@example.com",
		"exists\tfalse",
		"stored\tfalse",
		"message\tno service account configured",
		"hint\tgog auth service-account set user@example.com --key <service-account.json>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestAuthServiceAccountStatus_ConfiguredTextShowsStored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	path, err := config.ServiceAccountPath("user@example.com")
	if err != nil {
		t.Fatalf("ServiceAccountPath: %v", err)
	}
	if _, err := config.EnsureDataDir(); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"service_account","client_email":"svc@example.com","client_id":"123"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "service-account", "status", "user@example.com"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	for _, want := range []string{
		"exists\ttrue",
		"stored\ttrue",
		"client_email\tsvc@example.com",
		"client_id\t123",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
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
