package cmd

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

func TestVersionStringVariants(t *testing.T) {
	origVersion, origCommit, origDate := version, commit, date
	t.Cleanup(func() { version, commit, date = origVersion, origCommit, origDate })

	version, commit, date = "v1", "", ""
	if got := VersionString(); got != "v1" {
		t.Fatalf("unexpected: %q", got)
	}
	version, commit, date = "v1", "abc", ""
	if got := VersionString(); got != "v1 (abc)" {
		t.Fatalf("unexpected: %q", got)
	}
	version, commit, date = "v1", "", "2025-01-01"
	if got := VersionString(); got != "v1 (2025-01-01)" {
		t.Fatalf("unexpected: %q", got)
	}
	version, commit, date = "v1", "abc", "2025-01-01"
	if got := VersionString(); got != "v1 (abc 2025-01-01)" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestVersionCmd_JSON(t *testing.T) {
	origVersion, origCommit, origDate := version, commit, date
	t.Cleanup(func() { version, commit, date = origVersion, origCommit, origDate })
	version, commit, date = "v2", "c1", "d1"

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	jsonOut := captureStdout(t, func() {
		if err := runKong(t, &VersionCmd{}, []string{}, ctx, nil); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var parsed struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if parsed.Version != "v2" || parsed.Commit != "c1" || parsed.Date != "d1" {
		t.Fatalf("unexpected json: %#v", parsed)
	}
}
