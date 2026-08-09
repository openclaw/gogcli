package cmd

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

func TestGmailURLCmd_JSON(t *testing.T) {
	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	cmd := GmailURLCmd{ThreadIDs: []string{"t1"}}
	out := captureStdout(t, func() {
		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("GmailURLCmd: %v", err)
		}
	})
	var payload struct {
		URLs []map[string]string `json:"urls"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(payload.URLs) != 1 || payload.URLs[0]["id"] != "t1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestGmailURLCmd_Text(t *testing.T) {
	cmd := GmailURLCmd{ThreadIDs: []string{"t1"}}
	out := captureStdout(t, func() {
		u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
		if err != nil {
			t.Fatalf("ui.New: %v", err)
		}
		ctx := ui.WithUI(context.Background(), u)

		if err := cmd.Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
			t.Fatalf("GmailURLCmd: %v", err)
		}
	})
	if !strings.Contains(out, "t1") || !strings.Contains(out, "mail.google.com") {
		t.Fatalf("unexpected output: %q", out)
	}
}
