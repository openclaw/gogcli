package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthAliasSetListUnset_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := newCmdJSONOutputContext(t, os.Stdout, os.Stderr)

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
