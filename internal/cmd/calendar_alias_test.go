package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/config"
)

func TestCalendarAliasSetListUnset_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := newCalendarJSONOutputContext(t, os.Stdout, os.Stderr)

	// set
	_ = captureStdout(t, func() {
		if runErr := runKong(t, &CalendarAliasSetCmd{}, []string{"family", "3656f8abc123@group.calendar.google.com"}, ctx, &RootFlags{}); runErr != nil {
			t.Fatalf("set: %v", runErr)
		}
	})

	// list
	out := captureStdout(t, func() {
		if runErr := runKong(t, &CalendarAliasListCmd{}, []string{}, ctx, &RootFlags{}); runErr != nil {
			t.Fatalf("list: %v", runErr)
		}
	})
	var listResp struct {
		Aliases map[string]string `json:"aliases"`
	}
	if unmarshalErr := json.Unmarshal([]byte(out), &listResp); unmarshalErr != nil {
		t.Fatalf("list json: %v", unmarshalErr)
	}
	if listResp.Aliases["family"] != "3656f8abc123@group.calendar.google.com" {
		t.Fatalf("unexpected aliases: %#v", listResp.Aliases)
	}

	// unset
	_ = captureStdout(t, func() {
		if runErr := runKong(t, &CalendarAliasUnsetCmd{}, []string{"family"}, ctx, &RootFlags{}); runErr != nil {
			t.Fatalf("unset: %v", runErr)
		}
	})

	// Verify the alias was deleted
	_, ok, err := config.ResolveCalendarAlias("family")
	if err != nil {
		t.Fatalf("failed to resolve alias: %v", err)
	}
	if ok {
		t.Error("alias should have been deleted")
	}
}

func TestCalendarAliasSetCmd_JSON_UsesSnakeCaseCalendarID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := newCalendarJSONOutputContext(t, os.Stdout, os.Stderr)

	out := captureStdout(t, func() {
		if runErr := runKong(t, &CalendarAliasSetCmd{}, []string{"family", "family-cal@group.calendar.google.com"}, ctx, &RootFlags{}); runErr != nil {
			t.Fatalf("set: %v", runErr)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if got["calendar_id"] != "family-cal@group.calendar.google.com" {
		t.Fatalf("calendar_id = %v", got["calendar_id"])
	}
	if _, ok := got["calendarId"]; ok {
		t.Fatalf("unexpected camelCase key in output: %#v", got)
	}
}

func TestCalendarAliasSetCmd_Validation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := newCalendarOutputContext(t, os.Stdout, os.Stderr)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"whitespace in alias", []string{"my family", "cal@group.calendar.google.com"}, "whitespace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runKong(t, &CalendarAliasSetCmd{}, tt.args, ctx, &RootFlags{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestCalendarAliasUnsetCmd_NotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ctx := newCalendarOutputContext(t, os.Stdout, os.Stderr)

	err := runKong(t, &CalendarAliasUnsetCmd{}, []string{"nonexistent"}, ctx, &RootFlags{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "alias not found") {
		t.Errorf("expected 'alias not found' error, got: %v", err)
	}
}

func TestPrepareCalendarID_Integration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	// Set up alias
	if err := config.SetCalendarAlias("family", "family-cal@group.calendar.google.com"); err != nil {
		t.Fatalf("failed to set alias: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"alias resolved", "family", "family-cal@group.calendar.google.com", false},
		{"non-alias passthrough", "some-calendar@group.calendar.google.com", "some-calendar@group.calendar.google.com", false},
		{"primary passthrough", "primary", "primary", false},
		{"empty returns error", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareCalendarID(tt.input, false)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}

	t.Run("empty defaults to primary when requested", func(t *testing.T) {
		got, err := prepareCalendarID("", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != primaryCalendarID {
			t.Fatalf("expected %q, got %q", primaryCalendarID, got)
		}
	})
}
