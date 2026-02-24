package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestAuthKeepCmd_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := AuthKeepCmd{Email: "a@b.com", Key: keyPath}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("AuthKeepCmd: %v", err)
		}
	})
	var payload struct {
		Stored bool   `json:"stored"`
		Email  string `json:"email"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !payload.Stored || payload.Email != "a@b.com" || payload.Path == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if _, err := os.Stat(payload.Path); err != nil {
		t.Fatalf("missing stored key: %v", err)
	}
}

func TestAuthManageCmd(t *testing.T) {
	orig := startManageServer
	t.Cleanup(func() { startManageServer = orig })

	var captured googleauth.ManageServerOptions
	startManageServer = func(_ context.Context, opts googleauth.ManageServerOptions) error {
		captured = opts
		return nil
	}

	cmd := AuthManageCmd{ServicesCSV: "gmail,calendar", ForceConsent: true}
	if err := cmd.Run(context.Background(), &RootFlags{}); err != nil {
		t.Fatalf("AuthManageCmd: %v", err)
	}
	if !captured.ForceConsent || len(captured.Services) != 2 {
		t.Fatalf("unexpected manage options: %#v", captured)
	}
}

func TestAuthServicesCmd_Markdown(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := AuthServicesCmd{Markdown: true}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("AuthServicesCmd: %v", err)
		}
	})
	if !strings.Contains(out, "|") {
		t.Fatalf("expected markdown output, got: %q", out)
	}
}

func TestAuthServicesCmd_JSON(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := AuthServicesCmd{}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("AuthServicesCmd: %v", err)
		}
	})
	if !strings.Contains(out, "\"services\"") {
		t.Fatalf("unexpected json output: %q", out)
	}
}

func TestAuthServicesCmd_Table(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	cmd := AuthServicesCmd{}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("AuthServicesCmd: %v", err)
		}
	})
	if !strings.Contains(out, "SERVICE") {
		t.Fatalf("unexpected table output: %q", out)
	}
}

func TestAuthKeepCmd_Text(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	out := captureStdout(t, func() {
		u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if err != nil {
			t.Fatalf("ui.New: %v", err)
		}
		ctx := ui.WithUI(context.Background(), u)

		cmd := AuthKeepCmd{Email: "a@b.com", Key: keyPath}
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("AuthKeepCmd: %v", err)
		}
	})
	if !strings.Contains(out, "Keep service account configured") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAuthStatusCmd_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	if _, err := config.ConfigPath(); err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}

	cmd := AuthStatusCmd{}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("AuthStatusCmd: %v", err)
		}
	})
	if !strings.Contains(out, "\"keyring\"") || !strings.Contains(out, "\"config\"") {
		t.Fatalf("unexpected status output: %q", out)
	}
}
