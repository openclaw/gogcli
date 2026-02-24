package cmd

import (
	"context"
	"errors"
	"testing"
)

func TestConfirmDestructiveForce(t *testing.T) {
	flags := &RootFlags{Force: true}
	if err := confirmDestructive(context.TODO(), flags, "delete"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestConfirmDestructiveNoInput(t *testing.T) {
	flags := &RootFlags{NoInput: true}
	err := confirmDestructive(context.TODO(), flags, "delete")
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %#v", err)
	}
}

func TestConfirmDestructiveDryRun(t *testing.T) {
	flags := &RootFlags{DryRun: true}
	out := captureStdout(t, func() {
		err := confirmDestructive(context.TODO(), flags, "delete something")
		if err == nil {
			t.Fatalf("expected early exit error")
		}
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected ExitError code 0, got %#v", err)
		}
	})
	if out == "" {
		t.Fatalf("expected output")
	}
}
