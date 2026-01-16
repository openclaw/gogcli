package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestAuthAliasSetListUnset_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	u, err := ui.New(ui.Options{Stdout: os.Stdout, Stderr: os.Stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})

	// set
	_ = captureStdout(t, func() {
		if err := runKong(t, &AuthAliasSetCmd{}, []string{"work", "alias@example.com"}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("set: %v", err)
		}
	})

	// list
	out := captureStdout(t, func() {
		if err := runKong(t, &AuthAliasListCmd{}, []string{}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	var listResp struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal([]byte(out), &listResp); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if listResp.Aliases["work"] != "alias@example.com" {
		t.Fatalf("unexpected aliases: %#v", listResp.Aliases)
	}

	// unset
	_ = captureStdout(t, func() {
		if err := runKong(t, &AuthAliasUnsetCmd{}, []string{"work"}, ctx, &RootFlags{}); err != nil {
			t.Fatalf("unset: %v", err)
		}
	})
}
