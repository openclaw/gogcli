package cmd

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
