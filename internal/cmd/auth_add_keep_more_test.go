package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestAuthAddCmd_JSON_More(t *testing.T) {
	store := newMemSecretsStore()
	operations := app.AuthOperations{
		OpenSecretsStore: func() (secrets.Store, error) { return store, nil },
		AuthorizeGoogle: func(_ context.Context, opts googleauth.AuthorizeOptions) (string, error) {
			if len(opts.Services) == 0 {
				t.Fatalf("expected services")
			}
			return "rt", nil
		},
		FetchAuthorizedIdentity: func(context.Context, string, string, []string, time.Duration) (googleauth.Identity, error) {
			return googleauth.Identity{Email: "a@b.com"}, nil
		},
		EnsureKeychainAccess: func(context.Context) error { return nil },
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := withAuthOperations(
		outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true}),
		operations,
	)

	cmd := &AuthAddCmd{Email: "a@b.com", ServicesCSV: "gmail,drive"}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"stored\"") {
		t.Fatalf("unexpected output: %q", out)
	}
	if _, err := store.GetToken(config.DefaultClientName, "a@b.com"); err != nil {
		t.Fatalf("expected token stored: %v", err)
	}
}

func TestAuthKeepCmd_JSON_More(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	payload := map[string]any{
		"type":         "service_account",
		"client_email": "svc@example.com",
		"private_key":  "key",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if writeErr := os.WriteFile(keyPath, data, 0o600); writeErr != nil {
		t.Fatalf("write key: %v", writeErr)
	}

	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	ctx = withTestRuntime(ctx, func(*app.Runtime) {})

	cmd := &AuthKeepCmd{Email: "user@example.com", Key: keyPath}
	out := captureStdout(t, func() {
		if runErr := cmd.Run(ctx, &RootFlags{}); runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
	})
	if !strings.Contains(out, "\"stored\"") || !strings.Contains(out, "\"path\"") {
		t.Fatalf("unexpected output: %q", out)
	}
}
