package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
)

func TestAuthTextOutputs(t *testing.T) {
	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)
	execute := func(args []string) error { return executeWithRuntime(args, runtime) }

	if err := store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{
		Services:     []string{"gmail"},
		RefreshToken: "rt",
		CreatedAt:    time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	listOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"auth", "list"}); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})
	if !strings.Contains(listOut, "a@b.com") {
		t.Fatalf("unexpected list output: %q", listOut)
	}

	keysOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"auth", "tokens", "list"}); err != nil {
				t.Fatalf("tokens list: %v", err)
			}
		})
	})
	if !strings.Contains(keysOut, "token:default:a@b.com") {
		t.Fatalf("unexpected keys output: %q", keysOut)
	}

	delOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"--force", "auth", "tokens", "delete", "a@b.com"}); err != nil {
				t.Fatalf("tokens delete: %v", err)
			}
		})
	})
	if !strings.Contains(delOut, "deleted") {
		t.Fatalf("unexpected delete output: %q", delOut)
	}

	// Re-add and remove via auth remove.
	if err := store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "rt"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	rmOut := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"--force", "auth", "remove", "a@b.com"}); err != nil {
				t.Fatalf("remove: %v", err)
			}
		})
	})
	if !strings.Contains(rmOut, "deleted") {
		t.Fatalf("unexpected remove output: %q", rmOut)
	}
}

func TestAuthListAndTokens_NoTokens_Text(t *testing.T) {
	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)

	errOut := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := executeWithRuntime([]string{"auth", "list"}, runtime); err != nil {
				t.Fatalf("list: %v", err)
			}
		})
	})
	if !strings.Contains(errOut, "No tokens stored") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}

	errOut = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := executeWithRuntime([]string{"auth", "tokens", "list"}, runtime); err != nil {
				t.Fatalf("tokens list: %v", err)
			}
		})
	})
	if !strings.Contains(errOut, "No tokens stored") {
		t.Fatalf("unexpected stderr: %q", errOut)
	}
}

func TestAuthList_Check_Text(t *testing.T) {
	store := newMemSecretsStore()

	runtime := &app.Runtime{
		Auth: app.AuthOperations{
			OpenSecretsStore: func() (secrets.Store, error) { return store, nil },
			CheckRefreshToken: func(_ context.Context, _ string, refreshToken string, _ []string, _ time.Duration) error {
				if refreshToken == "bad" {
					return errors.New("invalid_grant")
				}
				return nil
			},
		},
	}

	_ = store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{RefreshToken: "good"})
	_ = store.SetToken(config.DefaultClientName, "b@b.com", secrets.Token{RefreshToken: "bad"})

	result := executeWithTestRuntime(t, []string{"auth", "list", "--check"}, runtime)
	if result.err != nil {
		t.Fatalf("list --check: %v", result.err)
	}
	out := result.stdout
	if !strings.Contains(out, "a@b.com") || !strings.Contains(out, "\ttrue\t") {
		t.Fatalf("expected a@b.com valid in output: %q", out)
	}
	if !strings.Contains(out, "b@b.com") || !strings.Contains(out, "\tfalse\t") || !strings.Contains(out, "invalid_grant") {
		t.Fatalf("expected b@b.com invalid in output: %q", out)
	}
}

func TestAuthTokensExportImport_Text(t *testing.T) {
	store := newMemSecretsStore()
	runtime := runtimeWithAuthStore(store)
	runtime.Auth.EnsureKeychainAccess = func(context.Context) error { return nil }
	execute := func(args []string) error { return executeWithRuntime(args, runtime) }

	if err := store.SetToken(config.DefaultClientName, "a@b.com", secrets.Token{
		RefreshToken: "rt",
	}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "token.json")
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"auth", "tokens", "export", "a@b.com", "--out", outPath}); err != nil {
				t.Fatalf("export: %v", err)
			}
		})
	})
	if !strings.Contains(out, "exported") || !strings.Contains(out, outPath) {
		t.Fatalf("unexpected export output: %q", out)
	}

	if err := store.DeleteToken(config.DefaultClientName, "a@b.com"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	out = captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := execute([]string{"auth", "tokens", "import", outPath}); err != nil {
				t.Fatalf("import: %v", err)
			}
		})
	})
	if !strings.Contains(out, "imported") {
		t.Fatalf("unexpected import output: %q", out)
	}
}

func TestAuthCredentials_Text(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))
	useFileKeyringForAuthCredentials(t)

	in := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(in, []byte(`{"installed":{"client_id":"id","client_secret":"sec"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"auth", "credentials", in}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if !strings.Contains(out, "path") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAuthAdd_InvalidService(t *testing.T) {
	if err := Execute([]string{"auth", "add", "a@b.com", "--services", "nope"}); err == nil {
		t.Fatalf("expected error")
	}
}
