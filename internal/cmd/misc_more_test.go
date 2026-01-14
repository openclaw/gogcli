package cmd

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestCompletionCmdRun(t *testing.T) {
	out := captureStdout(t, func() {
		cmd := &CompletionCmd{Shell: "bash"}
		if err := cmd.Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "__complete") {
		t.Fatalf("expected __complete in output: %q", out)
	}
}

func TestVersionStringVariantsMore(t *testing.T) {
	origVersion := version
	origCommit := commit
	origDate := date
	t.Cleanup(func() {
		version = origVersion
		commit = origCommit
		date = origDate
	})

	version = "1.2.3"
	commit = ""
	date = ""
	if got := VersionString(); got != "1.2.3" {
		t.Fatalf("unexpected version: %q", got)
	}

	commit = "abc"
	if got := VersionString(); !strings.Contains(got, "abc") {
		t.Fatalf("expected commit in version, got %q", got)
	}

	date = "2026-01-09"
	if got := VersionString(); !strings.Contains(got, "2026-01-09") {
		t.Fatalf("expected date in version, got %q", got)
	}
}

func TestVersionCmdJSON(t *testing.T) {
	origVersion := version
	origCommit := commit
	origDate := date
	t.Cleanup(func() {
		version = origVersion
		commit = origCommit
		date = origDate
	})

	version = "1.2.3"
	commit = "abc"
	date = "2026-01-09"

	out := captureStdout(t, func() {
		ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})
		if err := (&VersionCmd{}).Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "\"version\"") || !strings.Contains(out, "\"commit\"") {
		t.Fatalf("unexpected json output: %q", out)
	}
}

func TestLoadTrackingConfigForAccount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	t.Setenv("GOG_KEYRING_BACKEND", "file")
	t.Setenv("GOG_KEYRING_PASSWORD", "testpass")

	flags := &RootFlags{Account: "a@b.com"}
	account, cfg, err := loadTrackingConfigForAccount(flags)
	if err != nil {
		t.Fatalf("loadTrackingConfigForAccount: %v", err)
	}
	if account != "a@b.com" || cfg == nil {
		t.Fatalf("unexpected result: %q %#v", account, cfg)
	}
}

func TestVersionCmdText(t *testing.T) {
	out := captureStdout(t, func() {
		ctx := outfmt.WithMode(context.Background(), outfmt.Mode{})
		if err := (&VersionCmd{}).Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected version output")
	}
}
