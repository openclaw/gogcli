package config

import (
	"path/filepath"
	"testing"
)

func TestAccountAliasesCRUD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	if err := SetAccountAlias("work", "Work@Example.com"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	email, ok, err := ResolveAccountAlias("work")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if !ok || email != "work@example.com" {
		t.Fatalf("unexpected alias resolve: ok=%v email=%q", ok, email)
	}

	aliases, err := ListAccountAliases()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}

	if aliases["work"] != "work@example.com" {
		t.Fatalf("unexpected alias list: %#v", aliases)
	}

	deleted, err := DeleteAccountAlias("work")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if !deleted {
		t.Fatalf("expected alias delete")
	}
}
