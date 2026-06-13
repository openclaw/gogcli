package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
)

func TestCalendarAliasSetListUnset_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	var output bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)

	// set
	if runErr := runKong(t, &CalendarAliasSetCmd{}, []string{"family", "3656f8abc123@group.calendar.google.com"}, ctx, &RootFlags{}); runErr != nil {
		t.Fatalf("set: %v", runErr)
	}

	// list
	output.Reset()
	if runErr := runKong(t, &CalendarAliasListCmd{}, []string{}, ctx, &RootFlags{}); runErr != nil {
		t.Fatalf("list: %v", runErr)
	}
	out := output.String()
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
	output.Reset()
	if runErr := runKong(t, &CalendarAliasUnsetCmd{}, []string{"family"}, ctx, &RootFlags{}); runErr != nil {
		t.Fatalf("unset: %v", runErr)
	}

	// Verify the alias was deleted
	_, ok, err := defaultConfigStoreForTest(t).ResolveCalendarAlias("family")
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

	var output bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &output, io.Discard)
	if runErr := runKong(t, &CalendarAliasSetCmd{}, []string{"family", "family-cal@group.calendar.google.com"}, ctx, &RootFlags{}); runErr != nil {
		t.Fatalf("set: %v", runErr)
	}
	out := output.String()

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

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

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

	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)

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
	store := defaultConfigStoreForTest(t)

	// Set up alias
	if err := defaultConfigStoreForTest(t).SetCalendarAlias("family", "family-cal@group.calendar.google.com"); err != nil {
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
			got, err := prepareCalendarID(store, tt.input, false)
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
		got, err := prepareCalendarID(store, "", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != primaryCalendarID {
			t.Fatalf("expected %q, got %q", primaryCalendarID, got)
		}
	})
}

func TestExecuteCalendarAliasCRUDUsesRuntimeConfigStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg-config"))

	ambientStore := defaultConfigStoreForTest(t)
	if err := ambientStore.SetCalendarAlias("family", "ambient@group.calendar.google.com"); err != nil {
		t.Fatalf("set ambient alias: %v", err)
	}
	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	runtime := &app.Runtime{Config: runtimeStore}

	setResult := executeWithTestRuntime(t, []string{
		"--json", "calendar", "alias", "set", "family", "runtime@group.calendar.google.com",
	}, runtime)
	if setResult.err != nil {
		t.Fatalf("set: %v", setResult.err)
	}
	if calendarID, ok, err := runtimeStore.ResolveCalendarAlias("family"); err != nil || !ok || calendarID != "runtime@group.calendar.google.com" {
		t.Fatalf("runtime alias = %q, ok=%v err=%v", calendarID, ok, err)
	}
	if calendarID, ok, err := ambientStore.ResolveCalendarAlias("family"); err != nil || !ok || calendarID != "ambient@group.calendar.google.com" {
		t.Fatalf("ambient alias = %q, ok=%v err=%v", calendarID, ok, err)
	}

	listResult := executeWithTestRuntime(t, []string{"--json", "calendar", "alias", "list"}, runtime)
	if listResult.err != nil {
		t.Fatalf("list: %v", listResult.err)
	}
	var listed struct {
		Aliases map[string]string `json:"aliases"`
	}
	if err := json.Unmarshal([]byte(listResult.stdout), &listed); err != nil {
		t.Fatalf("list JSON: %v", err)
	}
	if listed.Aliases["family"] != "runtime@group.calendar.google.com" {
		t.Fatalf("listed aliases = %#v", listed.Aliases)
	}

	unsetResult := executeWithTestRuntime(t, []string{"--json", "calendar", "alias", "unset", "family"}, runtime)
	if unsetResult.err != nil {
		t.Fatalf("unset: %v", unsetResult.err)
	}
	if _, ok, err := runtimeStore.ResolveCalendarAlias("family"); err != nil || ok {
		t.Fatalf("runtime alias after unset: ok=%v err=%v", ok, err)
	}
}

func TestExecuteCalendarAliasDryRunDoesNotMutateConfig(t *testing.T) {
	runtimeStore := config.NewConfigStore(config.Layout{ConfigDir: t.TempDir()})
	if err := runtimeStore.SetCalendarAlias("family", "original@group.calendar.google.com"); err != nil {
		t.Fatalf("set initial alias: %v", err)
	}
	runtime := &app.Runtime{Config: runtimeStore}

	setResult := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "calendar", "alias", "set", "family", "replacement@group.calendar.google.com",
	}, runtime)
	if setResult.err != nil {
		t.Fatalf("dry-run set: %v", setResult.err)
	}
	if calendarID, ok, err := runtimeStore.ResolveCalendarAlias("family"); err != nil || !ok || calendarID != "original@group.calendar.google.com" {
		t.Fatalf("alias after dry-run set = %q, ok=%v err=%v", calendarID, ok, err)
	}

	unsetResult := executeWithTestRuntime(t, []string{
		"--json", "--dry-run", "calendar", "alias", "unset", "family",
	}, runtime)
	if unsetResult.err != nil {
		t.Fatalf("dry-run unset: %v", unsetResult.err)
	}
	if calendarID, ok, err := runtimeStore.ResolveCalendarAlias("family"); err != nil || !ok || calendarID != "original@group.calendar.google.com" {
		t.Fatalf("alias after dry-run unset = %q, ok=%v err=%v", calendarID, ok, err)
	}
}
