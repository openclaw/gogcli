package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// externalCommandPrefix is the prefix for external plugin binaries.
// Uses "gog-" to match the CLI binary name (following git/cargo convention).
const externalCommandPrefix = "gog-"

// ErrExternalNotFound indicates no external command was found for the given args.
var ErrExternalNotFound = errors.New("external command not found")

// tryExternalCommand attempts to find and execute an external plugin binary.
//
// Design: Post-parse fallback (Option B)
// This function is called AFTER Kong parsing fails with "unknown command".
// Why: Built-in commands always take precedence over external plugins.
// This prevents accidental or malicious shadowing of core functionality
// and matches the git/cargo convention.
//
// Algorithm: Longest-first (greedy) matching
// For args ["docs", "headings", "list"], tries in order:
//  1. gog-docs-headings-list (most specific)
//  2. gog-docs-headings
//  3. gog-docs (least specific)
//
// Why longest-first: More specific plugins should win over generic ones.
// Example: gog-docs-headings handles "headings" specifically, while gog-docs
// might handle generic docs operations. The specific plugin takes precedence.
//
// Returns:
//   - nil if no external command found (caller should return original error)
//   - error from exec if external command found but execution failed
//   - does not return if exec succeeds (replaces current process)
func tryExternalCommand(args []string) error {
	if len(args) == 0 {
		return ErrExternalNotFound
	}

	path, remainingArgs := findExternalCommand(args)
	if path == "" {
		return ErrExternalNotFound
	}

	return execExternal(path, remainingArgs)
}

// findExternalCommand searches PATH for a matching external plugin binary.
// Uses longest-first matching: tries most specific binary name first.
// Returns the path to the binary and remaining arguments to pass to it.
func findExternalCommand(args []string) (binaryPath string, remainingArgs []string) {
	// Longest-first: start with all args, progressively try fewer
	// Why: More specific plugins take precedence (e.g., gog-docs-headings over gog-docs)
	for i := len(args); i > 0; i-- {
		binaryName := externalCommandPrefix + strings.Join(args[:i], "-")
		if path, err := exec.LookPath(binaryName); err == nil {
			return path, args[i:]
		}
	}
	return "", nil
}

// execExternal replaces the current process with the external command.
// Uses syscall.Exec for true process replacement (no child process).
func execExternal(binaryPath string, args []string) error {
	// Build argv: binary name followed by remaining arguments
	argv := append([]string{binaryPath}, args...)

	// Use syscall.Exec to replace current process
	// This is the standard pattern for CLI plugin dispatch (git, cargo)
	return syscall.Exec(binaryPath, argv, os.Environ())
}

// LookPath is a variable to allow mocking in tests.
// In production, this is exec.LookPath.
var lookPath = exec.LookPath

// findExternalCommandWithLookPath is like findExternalCommand but uses
// the package-level lookPath variable for testability.
func findExternalCommandWithLookPath(args []string) (binaryPath string, remainingArgs []string) {
	for i := len(args); i > 0; i-- {
		binaryName := externalCommandPrefix + strings.Join(args[:i], "-")
		if path, err := lookPath(binaryName); err == nil {
			return path, args[i:]
		}
	}
	return "", nil
}

// helpOnelinerTimeout is the maximum time to wait for --help-oneliner response.
// Short timeout to keep help responsive; plugins that don't respond are skipped.
const helpOnelinerTimeout = 100 * time.Millisecond

// ExternalPlugin represents a discovered external plugin binary.
type ExternalPlugin struct {
	// Name is the plugin binary name without prefix (e.g., "docs-headings")
	Name string
	// Path is the full path to the binary
	Path string
	// Subcommands is the command hierarchy (e.g., ["docs", "headings"])
	Subcommands []string
	// Oneliner is the short description from --help-oneliner (may be empty)
	Oneliner string
}

// CommandName returns the user-facing command (e.g., "docs headings")
func (p *ExternalPlugin) CommandName() string {
	return strings.Join(p.Subcommands, " ")
}

// DiscoverExternalPlugins scans PATH for all gog-* binaries.
// Returns deduplicated list sorted by command name.
//
// Discovery happens lazily (only when help is requested) to avoid
// performance impact on normal command execution.
func DiscoverExternalPlugins() []ExternalPlugin {
	seen := make(map[string]bool)
	var plugins []ExternalPlugin

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range pathDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Skip unreadable directories
		}

		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasPrefix(name, externalCommandPrefix) {
				continue
			}
			if entry.IsDir() {
				continue
			}

			// Check if executable (skip if not)
			fullPath := filepath.Join(dir, name)
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Mode()&0111 == 0 {
				continue // Not executable
			}

			// Extract plugin name (remove prefix)
			pluginName := strings.TrimPrefix(name, externalCommandPrefix)
			if pluginName == "" {
				continue
			}

			// Deduplicate: first occurrence in PATH wins
			// Why: Matches exec.LookPath behavior and user expectations
			if seen[pluginName] {
				continue
			}
			seen[pluginName] = true

			plugins = append(plugins, ExternalPlugin{
				Name:        pluginName,
				Path:        fullPath,
				Subcommands: strings.Split(pluginName, "-"),
			})
		}
	}

	// Sort by command name for consistent display
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})

	return plugins
}

// FetchOneliners queries each plugin for its --help-oneliner description.
// Uses short timeout to keep help responsive; unresponsive plugins get empty description.
//
// Protocol: Plugins should respond to --help-oneliner with a single line (≤80 chars)
// describing what they do. Exit code 0 indicates success.
func FetchOneliners(plugins []ExternalPlugin) []ExternalPlugin {
	result := make([]ExternalPlugin, len(plugins))
	copy(result, plugins)

	for i := range result {
		result[i].Oneliner = fetchOneliner(result[i].Path)
	}

	return result
}

// fetchOneliner invokes a plugin with --help-oneliner and returns the response.
// Returns empty string on timeout, error, or non-zero exit code.
func fetchOneliner(binaryPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), helpOnelinerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--help-oneliner")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // Ignore stderr

	if err := cmd.Run(); err != nil {
		return "" // Timeout, not found, or non-zero exit
	}

	// Take first line only, trim whitespace
	oneliner := strings.TrimSpace(stdout.String())
	if idx := strings.IndexByte(oneliner, '\n'); idx >= 0 {
		oneliner = oneliner[:idx]
	}

	// Truncate to 80 chars for display
	if len(oneliner) > 80 {
		oneliner = oneliner[:77] + "..."
	}

	return oneliner
}

// GroupPluginsByTopLevel groups plugins by their first subcommand.
// Used to display plugins under their parent command in help output.
// Example: gog-docs-headings and gog-docs-bookmarks both under "docs"
func GroupPluginsByTopLevel(plugins []ExternalPlugin) map[string][]ExternalPlugin {
	groups := make(map[string][]ExternalPlugin)
	for _, p := range plugins {
		if len(p.Subcommands) == 0 {
			continue
		}
		topLevel := p.Subcommands[0]
		groups[topLevel] = append(groups[topLevel], p)
	}
	return groups
}
