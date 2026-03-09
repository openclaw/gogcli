package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestConfirmDestructive_Force(t *testing.T) {
	if err := confirmDestructive(context.Background(), &RootFlags{Force: true}, "do thing"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestConfirmDestructive_NoInput(t *testing.T) {
	err := confirmDestructive(context.Background(), &RootFlags{NoInput: true}, "nuke things")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "refusing to nuke things") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDryRunAndConfirmDestructive_DryRunSkipsPrompt(t *testing.T) {
	out := captureStdout(t, func() {
		err := dryRunAndConfirmDestructive(context.Background(), &RootFlags{DryRun: true, NoInput: true}, "drive.delete", map[string]any{"file_id": "f1"}, "delete file f1")
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected dry-run exit, got %v", err)
		}
	})
	if !strings.Contains(out, "drive.delete") {
		t.Fatalf("expected op in output, got %q", out)
	}
}

func TestFlagsWithoutDryRun(t *testing.T) {
	got := flagsWithoutDryRun(&RootFlags{DryRun: true, Force: true, NoInput: true})
	if got == nil || got.DryRun || !got.Force || !got.NoInput {
		t.Fatalf("unexpected flags clone: %#v", got)
	}
}
