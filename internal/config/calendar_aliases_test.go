package config

import (
	"os"
	"testing"
)

func TestCalendarAliasesCRUD(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	if err := store.SetCalendarAlias("family", "3656f8abc123@group.calendar.google.com"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	calID, ok, err := store.ResolveCalendarAlias("family")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if !ok || calID != "3656f8abc123@group.calendar.google.com" {
		t.Fatalf("unexpected alias resolve: ok=%v calID=%q", ok, calID)
	}

	aliases, err := store.ListCalendarAliases()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}

	if aliases["family"] != "3656f8abc123@group.calendar.google.com" {
		t.Fatalf("unexpected alias list: %#v", aliases)
	}

	deleted, err := store.DeleteCalendarAlias("family")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if !deleted {
		t.Fatalf("expected alias delete")
	}
}

func TestResolveCalendarID(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	// Empty stays empty; command parsing owns default-primary behavior.
	resolved, err := store.ResolveCalendarID("")
	if err != nil {
		t.Fatalf("resolve empty: %v", err)
	}

	if resolved != "" {
		t.Fatalf("expected empty for empty input, got %q", resolved)
	}

	// Non-alias returns unchanged
	resolved, err = store.ResolveCalendarID("some-calendar-id@group.calendar.google.com")
	if err != nil {
		t.Fatalf("resolve non-alias: %v", err)
	}

	if resolved != "some-calendar-id@group.calendar.google.com" {
		t.Fatalf("expected unchanged, got %q", resolved)
	}

	// Set alias and resolve
	if setErr := store.SetCalendarAlias("work", "work-calendar@group.calendar.google.com"); setErr != nil {
		t.Fatalf("set alias: %v", setErr)
	}

	resolved, err = store.ResolveCalendarID("work")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if resolved != "work-calendar@group.calendar.google.com" {
		t.Fatalf("expected resolved alias, got %q", resolved)
	}

	// Alias lookup is case-insensitive
	resolved, err = store.ResolveCalendarID("WORK")
	if err != nil {
		t.Fatalf("resolve uppercase alias: %v", err)
	}

	if resolved != "work-calendar@group.calendar.google.com" {
		t.Fatalf("expected resolved alias for uppercase, got %q", resolved)
	}
}

func TestCalendarAliasNormalization(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	// Set with mixed case and whitespace
	if err := store.SetCalendarAlias("  Family  ", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	// Resolve with different case
	calID, ok, err := store.ResolveCalendarAlias("FAMILY")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if !ok || calID != "family-cal@group.calendar.google.com" {
		t.Fatalf("unexpected alias resolve: ok=%v calID=%q", ok, calID)
	}

	// Delete with different case
	deleted, err := store.DeleteCalendarAlias("family")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if !deleted {
		t.Fatalf("expected alias delete")
	}
}

func TestSetCalendarAlias_Validation(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	tests := []struct {
		name       string
		alias      string
		calendarID string
	}{
		{name: "empty alias", alias: "", calendarID: "family@group.calendar.google.com"},
		{name: "alias with whitespace", alias: "my family", calendarID: "family@group.calendar.google.com"},
		{name: "empty calendar ID", alias: "family", calendarID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.SetCalendarAlias(tt.alias, tt.calendarID); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestCalendarAliasReadMissesDoNotCreateConfig(t *testing.T) {
	store := NewConfigStore(Layout{ConfigDir: t.TempDir()})

	if _, ok, err := store.ResolveCalendarAlias("missing"); err != nil || ok {
		t.Fatalf("resolve missing alias: ok=%v err=%v", ok, err)
	}

	aliases, err := store.ListCalendarAliases()
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
