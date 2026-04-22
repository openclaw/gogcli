package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestVersionString_WithFork asserts VersionString includes a `[fork: ...]`
// suffix when the `fork` ldflag variable is populated. This is the downstream
// fork-maintainer scaffold's hook for tagging patched binaries.
func TestVersionString_WithFork(t *testing.T) {
	origFork := fork
	t.Cleanup(func() { fork = origFork })

	fork = "prateek/gogcli@abc123"
	got := VersionString()
	if !strings.Contains(got, "[fork: prateek/gogcli@abc123]") {
		t.Fatalf("expected fork suffix in %q", got)
	}
}

// TestVersionString_NoForkMatchesUpstream asserts an empty `fork` produces
// output identical to upstream's VersionString (no `[fork: ...]` suffix).
func TestVersionString_NoFork(t *testing.T) {
	origFork := fork
	t.Cleanup(func() { fork = origFork })

	fork = ""
	got := VersionString()
	if strings.Contains(got, "[fork:") {
		t.Fatalf("unexpected fork suffix in %q", got)
	}
}

// TestVersionJSONField asserts the JSON payload exposes `fork` only when set,
// matching upstream's schema when absent.
func TestVersionJSONField(t *testing.T) {
	origFork := fork
	t.Cleanup(func() { fork = origFork })

	// With fork set, the JSON path in VersionCmd.Run should emit a "fork" key.
	fork = "prateek/gogcli@abc123"
	payload := map[string]any{
		"version": strings.TrimSpace(version),
		"commit":  strings.TrimSpace(commit),
		"date":    strings.TrimSpace(date),
	}
	if f := strings.TrimSpace(fork); f != "" {
		payload["fork"] = f
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	if !strings.Contains(string(b), `"fork":"prateek/gogcli@abc123"`) {
		t.Fatalf("expected fork field in %s", string(b))
	}

	// Without fork set, no "fork" key.
	fork = ""
	payload = map[string]any{
		"version": strings.TrimSpace(version),
		"commit":  strings.TrimSpace(commit),
		"date":    strings.TrimSpace(date),
	}
	if f := strings.TrimSpace(fork); f != "" {
		payload["fork"] = f
	}
	b, err = json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	if strings.Contains(string(b), `"fork"`) {
		t.Fatalf("unexpected fork field in %s", string(b))
	}
}
