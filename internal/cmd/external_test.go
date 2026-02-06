package cmd

import (
	"errors"
	"os/exec"
	"testing"
)

// mockLookPath creates a mock LookPath function that returns success
// for binaries in the "exists" set.
func mockLookPath(exists map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := exists[name]; ok {
			return path, nil
		}
		return "", exec.ErrNotFound
	}
}

func TestFindExternalCommand_LongestFirstMatching(t *testing.T) {
	// This test documents the longest-first (greedy) matching behavior.
	// Why longest-first: More specific plugins should take precedence.
	// Example: gog-docs-headings should handle "docs headings" even if
	// gog-docs also exists.

	tests := []struct {
		name             string
		args             []string
		existingBinaries map[string]string
		wantPath         string
		wantArgs         []string
	}{
		{
			name: "exact match single arg",
			args: []string{"docs"},
			existingBinaries: map[string]string{
				"gog-docs": "/usr/bin/gog-docs",
			},
			wantPath: "/usr/bin/gog-docs",
			wantArgs: []string{},
		},
		{
			name: "exact match two args",
			args: []string{"docs", "headings"},
			existingBinaries: map[string]string{
				"gog-docs-headings": "/usr/bin/gog-docs-headings",
			},
			wantPath: "/usr/bin/gog-docs-headings",
			wantArgs: []string{},
		},
		{
			name: "longest match wins over shorter",
			// When both gog-docs and gog-docs-headings exist,
			// gog-docs-headings should win for "docs headings" args
			args: []string{"docs", "headings"},
			existingBinaries: map[string]string{
				"gog-docs":          "/usr/bin/gog-docs",
				"gog-docs-headings": "/usr/bin/gog-docs-headings",
			},
			wantPath: "/usr/bin/gog-docs-headings",
			wantArgs: []string{},
		},
		{
			name: "falls back to shorter when longer not found",
			// gog-docs-headings-list doesn't exist, so fall back to gog-docs-headings
			args: []string{"docs", "headings", "list"},
			existingBinaries: map[string]string{
				"gog-docs-headings": "/usr/bin/gog-docs-headings",
			},
			wantPath: "/usr/bin/gog-docs-headings",
			wantArgs: []string{"list"},
		},
		{
			name: "passes remaining args to plugin",
			args: []string{"docs", "headings", "--docid", "ABC123"},
			existingBinaries: map[string]string{
				"gog-docs-headings": "/usr/bin/gog-docs-headings",
			},
			wantPath: "/usr/bin/gog-docs-headings",
			wantArgs: []string{"--docid", "ABC123"},
		},
		{
			name: "three-level nesting",
			args: []string{"docs", "headings", "list", "--limit", "10"},
			existingBinaries: map[string]string{
				"gog-docs":               "/usr/bin/gog-docs",
				"gog-docs-headings":      "/usr/bin/gog-docs-headings",
				"gog-docs-headings-list": "/usr/bin/gog-docs-headings-list",
			},
			wantPath: "/usr/bin/gog-docs-headings-list",
			wantArgs: []string{"--limit", "10"},
		},
		{
			name:             "no match returns empty",
			args:             []string{"unknown", "command"},
			existingBinaries: map[string]string{},
			wantPath:         "",
			wantArgs:         nil,
		},
		{
			name:             "empty args returns empty",
			args:             []string{},
			existingBinaries: map[string]string{},
			wantPath:         "",
			wantArgs:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original lookPath
			origLookPath := lookPath
			defer func() { lookPath = origLookPath }()

			lookPath = mockLookPath(tt.existingBinaries)

			gotPath, gotArgs := findExternalCommandWithLookPath(tt.args)

			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}

			if !slicesEqual(gotArgs, tt.wantArgs) {
				t.Errorf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestTryExternalCommand_NotFound(t *testing.T) {
	// Save and restore original lookPath
	origLookPath := lookPath
	defer func() { lookPath = origLookPath }()

	lookPath = mockLookPath(map[string]string{})

	err := tryExternalCommand([]string{"nonexistent"})
	if !errors.Is(err, ErrExternalNotFound) {
		t.Errorf("err = %v, want ErrExternalNotFound", err)
	}
}

func TestTryExternalCommand_EmptyArgs(t *testing.T) {
	err := tryExternalCommand([]string{})
	if !errors.Is(err, ErrExternalNotFound) {
		t.Errorf("err = %v, want ErrExternalNotFound", err)
	}
}

// slicesEqual compares two string slices for equality.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestExternalPlugin_CommandName(t *testing.T) {
	tests := []struct {
		name        string
		subcommands []string
		want        string
	}{
		{
			name:        "single subcommand",
			subcommands: []string{"docs"},
			want:        "docs",
		},
		{
			name:        "two subcommands",
			subcommands: []string{"docs", "headings"},
			want:        "docs headings",
		},
		{
			name:        "three subcommands",
			subcommands: []string{"docs", "headings", "list"},
			want:        "docs headings list",
		},
		{
			name:        "empty subcommands",
			subcommands: []string{},
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ExternalPlugin{Subcommands: tt.subcommands}
			if got := p.CommandName(); got != tt.want {
				t.Errorf("CommandName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupPluginsByTopLevel(t *testing.T) {
	// Test that plugins are correctly grouped by their first subcommand.
	// This is used to display plugins under their parent command in help.

	plugins := []ExternalPlugin{
		{Name: "docs-headings", Subcommands: []string{"docs", "headings"}},
		{Name: "docs-bookmarks", Subcommands: []string{"docs", "bookmarks"}},
		{Name: "sheets-formulas", Subcommands: []string{"sheets", "formulas"}},
		{Name: "hello", Subcommands: []string{"hello"}},
	}

	groups := GroupPluginsByTopLevel(plugins)

	// Check docs group
	if len(groups["docs"]) != 2 {
		t.Errorf("docs group has %d plugins, want 2", len(groups["docs"]))
	}

	// Check sheets group
	if len(groups["sheets"]) != 1 {
		t.Errorf("sheets group has %d plugins, want 1", len(groups["sheets"]))
	}

	// Check hello group (single-level plugin)
	if len(groups["hello"]) != 1 {
		t.Errorf("hello group has %d plugins, want 1", len(groups["hello"]))
	}

	// Check total groups
	if len(groups) != 3 {
		t.Errorf("got %d groups, want 3", len(groups))
	}
}

func TestGroupPluginsByTopLevel_EmptySubcommands(t *testing.T) {
	// Plugins with empty subcommands should be skipped
	plugins := []ExternalPlugin{
		{Name: "", Subcommands: []string{}},
		{Name: "docs", Subcommands: []string{"docs"}},
	}

	groups := GroupPluginsByTopLevel(plugins)

	if len(groups) != 1 {
		t.Errorf("got %d groups, want 1", len(groups))
	}
	if len(groups["docs"]) != 1 {
		t.Errorf("docs group has %d plugins, want 1", len(groups["docs"]))
	}
}
