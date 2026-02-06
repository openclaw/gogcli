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
