package config

import (
	"path/filepath"
	"testing"
)

func TestCalendarAliasesCRUD(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	if err := SetCalendarAlias("family", "3656f8abc123@group.calendar.google.com"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	calID, ok, err := ResolveCalendarAlias("family")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if !ok || calID != "3656f8abc123@group.calendar.google.com" {
		t.Fatalf("unexpected alias resolve: ok=%v calID=%q", ok, calID)
	}

	aliases, err := ListCalendarAliases()
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}

	if aliases["family"] != "3656f8abc123@group.calendar.google.com" {
		t.Fatalf("unexpected alias list: %#v", aliases)
	}

	deleted, err := DeleteCalendarAlias("family")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if !deleted {
		t.Fatalf("expected alias delete")
	}
}

func TestResolveCalendarID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// Empty stays empty; command parsing owns default-primary behavior.
	resolved, err := ResolveCalendarID("")
	if err != nil {
		t.Fatalf("resolve empty: %v", err)
	}

	if resolved != "" {
		t.Fatalf("expected empty for empty input, got %q", resolved)
	}

	// Non-alias returns unchanged
	resolved, err = ResolveCalendarID("some-calendar-id@group.calendar.google.com")
	if err != nil {
		t.Fatalf("resolve non-alias: %v", err)
	}

	if resolved != "some-calendar-id@group.calendar.google.com" {
		t.Fatalf("expected unchanged, got %q", resolved)
	}

	// Set alias and resolve
	if setErr := SetCalendarAlias("work", "work-calendar@group.calendar.google.com"); setErr != nil {
		t.Fatalf("set alias: %v", setErr)
	}

	resolved, err = ResolveCalendarID("work")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if resolved != "work-calendar@group.calendar.google.com" {
		t.Fatalf("expected resolved alias, got %q", resolved)
	}

	// Alias lookup is case-insensitive
	resolved, err = ResolveCalendarID("WORK")
	if err != nil {
		t.Fatalf("resolve uppercase alias: %v", err)
	}

	if resolved != "work-calendar@group.calendar.google.com" {
		t.Fatalf("expected resolved alias for uppercase, got %q", resolved)
	}
}

func TestCalendarAliasNormalization(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// Set with mixed case and whitespace
	if err := SetCalendarAlias("  Family  ", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	// Resolve with different case
	calID, ok, err := ResolveCalendarAlias("FAMILY")
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}

	if !ok || calID != "family-cal@group.calendar.google.com" {
		t.Fatalf("unexpected alias resolve: ok=%v calID=%q", ok, calID)
	}

	// Delete with different case
	deleted, err := DeleteCalendarAlias("family")
	if err != nil {
		t.Fatalf("delete alias: %v", err)
	}

	if !deleted {
		t.Fatalf("expected alias delete")
	}
}

func TestSetCalendarAlias_Validation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

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
			if err := SetCalendarAlias(tt.alias, tt.calendarID); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}
