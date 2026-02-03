package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// confirmDestructive additional coverage tests
// -----------------------------------------------------------------------------

func TestConfirmDestructive_ForceBypassesAll(t *testing.T) {
	// Force flag should bypass all checks, including NoInput
	flags := &RootFlags{Force: true, NoInput: true}
	if err := confirmDestructive(context.Background(), flags, "delete everything"); err != nil {
		t.Fatalf("expected nil with Force=true, got %v", err)
	}
}

func TestConfirmDestructive_NoInputReturnusageError(t *testing.T) {
	flags := &RootFlags{NoInput: true}
	err := confirmDestructive(context.Background(), flags, "wipe data")
	if err == nil {
		t.Fatalf("expected error for NoInput without Force")
	}

	// Should be an ExitError with code 2 (usage error)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 2 {
		t.Fatalf("expected exit code 2, got %d", exitErr.Code)
	}

	// Should contain the action in the message
	if !strings.Contains(err.Error(), "wipe data") {
		t.Fatalf("error should mention the action: %v", err)
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("error should mention refusing: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error should mention --force: %v", err)
	}
}

func TestConfirmDestructive_ActionInMessage(t *testing.T) {
	tests := []struct {
		action string
	}{
		{"delete user account"},
		{"remove all files"},
		{"destroy database"},
		{"reset configuration"},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			flags := &RootFlags{NoInput: true}
			err := confirmDestructive(context.Background(), flags, tc.action)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.action) {
				t.Fatalf("error message should contain action %q: %v", tc.action, err)
			}
		})
	}
}

func TestConfirmDestructive_ExitErrorProperties(t *testing.T) {
	flags := &RootFlags{NoInput: true}
	err := confirmDestructive(context.Background(), flags, "test action")

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}

	// Verify ExitError properly wraps and exposes the underlying error
	if exitErr.Err == nil {
		t.Fatal("ExitError.Err should not be nil")
	}

	// Error() should return the wrapped error message
	errStr := err.Error()
	if errStr == "" {
		t.Fatal("Error() should return non-empty string")
	}
}

func TestConfirmDestructive_ContextPassedThrough(t *testing.T) {
	// Test that context is passed through (though currently not used when Force=true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// With Force=true, should still succeed even with cancelled context
	flags := &RootFlags{Force: true}
	if err := confirmDestructive(ctx, flags, "action"); err != nil {
		t.Fatalf("Force=true should succeed regardless of context: %v", err)
	}
}

func TestConfirmDestructive_DefaultFlags(t *testing.T) {
	// Default flags (no Force, no NoInput) in non-terminal environment
	// This tests the term.IsTerminal check which will return false in tests
	flags := &RootFlags{}
	err := confirmDestructive(context.Background(), flags, "test")

	// In a test environment, stdin is not a terminal, so it should fail
	if err == nil {
		// If it doesn't fail, it means the stdin somehow is a terminal (unlikely in tests)
		// This is acceptable as the behavior depends on the environment
		t.Skip("stdin appears to be a terminal in this test environment")
	}

	// Should fail with usage error indicating non-interactive mode
	if !strings.Contains(err.Error(), "non-interactive") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected non-interactive error with --force hint: %v", err)
	}
}

// -----------------------------------------------------------------------------
// ExitError tests
// -----------------------------------------------------------------------------

func TestExitError_Error(t *testing.T) {
	err := &ExitError{Code: 1, Err: errors.New("something went wrong")}
	if err.Error() != "something went wrong" {
		t.Fatalf("unexpected error string: %s", err.Error())
	}
}

func TestExitError_NilErr(t *testing.T) {
	// ExitError with nil Err
	err := &ExitError{Code: 1, Err: nil}
	// Error() method should handle nil gracefully
	result := err.Error()
	if result != "" {
		t.Fatalf("expected empty string for nil Err, got %q", result)
	}
}

func TestExitError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	exitErr := &ExitError{Code: 1, Err: innerErr}

	// errors.Unwrap should return the inner error
	unwrapped := errors.Unwrap(exitErr)
	if unwrapped != innerErr {
		t.Fatalf("Unwrap should return inner error, got %v", unwrapped)
	}

	// errors.Is should work with the inner error
	if !errors.Is(exitErr, innerErr) {
		t.Fatal("errors.Is should find inner error")
	}
}

func TestExitError_ExitCodes(t *testing.T) {
	tests := []struct {
		code int
		name string
	}{
		{0, "success (unusual)"},
		{1, "general error"},
		{2, "usage error"},
		{126, "command not executable"},
		{127, "command not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := &ExitError{Code: tc.code, Err: errors.New("test")}
			if err.Code != tc.code {
				t.Fatalf("expected code %d, got %d", tc.code, err.Code)
			}
		})
	}
}
