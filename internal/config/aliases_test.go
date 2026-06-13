package config

import (
	"os"
	"testing"
)

func TestAccountAliasesCRUD(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	if err := store.SetAccountAlias("work", "Work@Example.com"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	email, ok, err := store.ResolveAccountAlias("work")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if !ok || email != "work@example.com" {
		t.Fatalf("unexpected alias resolve: ok=%v email=%q", ok, email)
	}

	aliases, err := store.ListAccountAliases()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}

	if aliases["work"] != "work@example.com" {
		t.Fatalf("unexpected alias list: %#v", aliases)
	}

	deleted, err := store.DeleteAccountAlias("work")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if !deleted {
		t.Fatalf("expected alias delete")
	}
}

func TestDeleteMissingAccountAliasDoesNotCreateConfig(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	deleted, err := store.DeleteAccountAlias("missing")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if deleted {
		t.Fatalf("expected no delete")
	}

	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("expected no config file, stat err=%v", err)
	}
}

func TestAccountAliasReadMissesDoNotCreateConfig(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	if _, ok, err := store.ResolveAccountAlias("missing"); err != nil || ok {
		t.Fatalf("resolve missing alias: ok=%v err=%v", ok, err)
	}

	aliases, err := store.ListAccountAliases()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}

	if len(aliases) != 0 {
		t.Fatalf("aliases = %#v, want empty", aliases)
	}

	if _, err := os.Stat(store.Path()); !os.IsNotExist(err) {
		t.Fatalf("expected no config file, stat err=%v", err)
	}
}
