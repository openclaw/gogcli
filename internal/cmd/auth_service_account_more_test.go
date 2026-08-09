package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/config"
)

func TestAuthServiceAccountSet_AndList_Text(t *testing.T) {
	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}, runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "Service account configured") {
		t.Fatalf("unexpected output: %q", out)
	}

	storedPath := defaultLayoutForTest(t, config.PathKindData).ServiceAccountPath("user@example.com")
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("expected stored key at %q: %v", storedPath, err)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "list"}, runtime); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})
	if !strings.Contains(listOut, "user@example.com") || !strings.Contains(listOut, "service_account") {
		t.Fatalf("unexpected list output: %q", listOut)
	}
}

func TestAuthServiceAccountCommandsUseInjectedLayout(t *testing.T) {
	ambientHome := t.TempDir()
	t.Setenv("HOME", ambientHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(ambientHome, "xdg-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(ambientHome, "xdg-data"))

	root := t.TempDir()
	layout := config.Layout{
		ConfigDir:      filepath.Join(root, "config"),
		DataDir:        filepath.Join(root, "data"),
		ExplicitConfig: true,
		ExplicitData:   true,
	}
	runtime := runtimeWithAuthStore(newMemSecretsStore())
	runtime.ServiceAccounts = config.NewServiceAccountStore(layout)

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}, runtime); err != nil {
				t.Fatalf("set: %v", err)
			}
		})
	})

	storedPath := layout.ServiceAccountPath("user@example.com")
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("expected injected service account path %q: %v", storedPath, err)
	}
	ambientLayout, err := config.NewSystemResolver("").Resolve(config.PathKindData)
	if err != nil {
		t.Fatalf("resolve ambient layout: %v", err)
	}
	if _, err := os.Stat(ambientLayout.ServiceAccountPath("user@example.com")); !os.IsNotExist(err) {
		t.Fatalf("ambient service account path was touched: %v", err)
	}

	for _, args := range [][]string{
		{"auth", "service-account", "status", "user@example.com"},
		{"auth", "list"},
	} {
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := executeWithRuntime(args, runtime); err != nil {
					t.Fatalf("%v: %v", args, err)
				}
			})
		})
		if !strings.Contains(out, "user@example.com") {
			t.Fatalf("%v output does not use injected service account: %q", args, out)
		}
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "service-account", "unset", "user@example.com", "--force"}, runtime); err != nil {
				t.Fatalf("unset: %v", err)
			}
		})
	})
	if _, err := os.Stat(storedPath); !os.IsNotExist(err) {
		t.Fatalf("injected service account path still exists: %v", err)
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

	storedPath := defaultLayoutForTest(t, config.PathKindData).ServiceAccountPath("stdin@example.com")
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

	storedPath := defaultLayoutForTest(t, config.PathKindData).ServiceAccountPath("env@example.com")
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

func TestAuthServiceAccountSet_InvalidJSONIsUsageError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "malformed", body: "nope", want: "invalid service account JSON"},
		{name: "wrong type", body: `{"type":"authorized_user"}`, want: "expected type=service_account"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keyPath := filepath.Join(t.TempDir(), "sa.json")
			if err := os.WriteFile(keyPath, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write key: %v", err)
			}
			err := Execute([]string{"auth", "service-account", "set", "bad@example.com", "--key", keyPath, "--dry-run"})
			if err == nil {
				t.Fatal("expected invalid JSON error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
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

	layout := defaultLayoutForTest(t, config.PathKindData)
	path := layout.ServiceAccountPath("user@example.com")
	if err := os.MkdirAll(layout.DataDir, 0o700); err != nil {
		t.Fatalf("ensure data dir: %v", err)
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
	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"svc@example.com"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	_ = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"auth", "service-account", "set", "user@example.com", "--key", keyPath}, runtime); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := executeWithRuntime([]string{"--account", "user@example.com", "auth", "status"}, runtime); err != nil {
				t.Fatalf("status: %v", err)
			}
		})
	})
	if !strings.Contains(out, "auth_preferred\tservice_account") {
		t.Fatalf("unexpected status output: %q", out)
	}
}
