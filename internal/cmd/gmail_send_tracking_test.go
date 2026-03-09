package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

func TestResolveTrackingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	cmd := &GmailSendCmd{}
	cmd.BodyHTML = "<html></html>"

	// Multiple recipients without split should fail.
	if _, err := cmd.resolveTrackingConfig("a@b.com", []string{"a@b.com", "b@b.com"}, nil, nil, cmd.BodyHTML); err == nil {
		t.Fatalf("expected error for multiple recipients without split")
	}

	cmd.TrackSplit = true
	cmd.BodyHTML = ""
	if _, err := cmd.resolveTrackingConfig("a@b.com", []string{"a@b.com"}, nil, nil, cmd.BodyHTML); err == nil {
		t.Fatalf("expected error for missing body html")
	}

	cmd.BodyHTML = "<html></html>"
	if _, err := cmd.resolveTrackingConfig("a@b.com", []string{"a@b.com"}, nil, nil, cmd.BodyHTML); err == nil {
		t.Fatalf("expected error for unconfigured tracking")
	}

	key, err := tracking.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	cfg := &tracking.Config{
		Enabled:     true,
		WorkerURL:   "https://example.com",
		TrackingKey: key,
		AdminKey:    "admin",
	}
	err = tracking.SaveConfig("a@b.com", cfg)
	if err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got, err := cmd.resolveTrackingConfig("a@b.com", []string{"a@b.com"}, nil, nil, cmd.BodyHTML)
	if err != nil {
		t.Fatalf("resolveTrackingConfig: %v", err)
	}
	if got == nil || !got.IsConfigured() {
		t.Fatalf("expected configured tracking, got %#v", got)
	}
}

func TestFirstRecipient(t *testing.T) {
	if got := firstRecipient([]string{"a"}, []string{"b"}, []string{"c"}); got != "a" {
		t.Fatalf("unexpected first recipient: %q", got)
	}
	if got := firstRecipient(nil, []string{"b"}, nil); got != "b" {
		t.Fatalf("unexpected first recipient: %q", got)
	}
	if got := firstRecipient(nil, nil, []string{"c"}); got != "c" {
		t.Fatalf("unexpected first recipient: %q", got)
	}
}

func TestWriteSendResults_JSONMultiple(t *testing.T) {
	out := captureStdout(t, func() {
		u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if err != nil {
			t.Fatalf("ui.New: %v", err)
		}
		ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

		if err := writeSendResults(ctx, u, "from@example.com", []sendResult{
			{MessageID: "m1", ThreadID: "t1", To: "a@example.com"},
			{MessageID: "m2", ThreadID: "t2", To: "b@example.com"},
		}); err != nil {
			t.Fatalf("writeSendResults: %v", err)
		}
	})
	if !strings.Contains(out, "\"messages\"") {
		t.Fatalf("unexpected json output: %q", out)
	}
}
