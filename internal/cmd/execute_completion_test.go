package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestExecute_Completion_Bash(t *testing.T) {
	orig := os.Stdout
	f, createErr := os.CreateTemp(t.TempDir(), "completion-*.txt")
	if createErr != nil {
		t.Fatalf("CreateTemp: %v", createErr)
	}
	os.Stdout = f
	_ = captureStderr(t, func() {
		if execErr := Execute([]string{"completion", "bash"}); execErr != nil {
			t.Fatalf("Execute: %v", execErr)
		}
	})
	_ = f.Close()
	os.Stdout = orig

	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	out := string(b)
	if !strings.Contains(out, "__complete") || !strings.Contains(out, "complete -F _gog_complete gog") {
		excerpt := out
		if len(excerpt) > 200 {
			excerpt = excerpt[:200]
		}
		t.Fatalf("unexpected out=%q", excerpt)
	}
}
