package cmd

import (
	"encoding/json"
	"testing"
)

func TestSplitCommandPath_SplitsWhitespaceWithinArgs(t *testing.T) {
	got := splitCommandPath([]string{" drive ls ", "  "})
	want := []string{"drive", "ls"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected token at %d: got=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestExecute_Schema_QuotedCommandPathToken(t *testing.T) {
	out := captureStdout(t, func() {
		_ = captureStderr(t, func() {
			if err := Execute([]string{"schema", "drive ls"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})

	var doc struct {
		Command struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"command"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v out=%q", err, out)
	}
	if doc.Command.Name != "ls" {
		t.Fatalf("expected command name ls, got %q", doc.Command.Name)
	}
	if doc.Command.Path == "" {
		t.Fatalf("expected non-empty command path")
	}
}
