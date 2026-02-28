package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestSelectedCommandTokens(t *testing.T) {
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}

	kctx, err := parser.Parse([]string{"config", "set", "foo", "bar"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := selectedCommandTokens(kctx)
	want := []string{"config", "set"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected tokens: got=%v want=%v", got, want)
	}
}

func TestExecute_ReadOnlyBlocksMutatingCommand(t *testing.T) {
	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"--read-only", "config", "set", "foo", "bar"})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), "read-only mode blocks mutating command") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})
	if !strings.Contains(errText, "read-only mode blocks mutating command") {
		t.Fatalf("expected read-only error on stderr, got: %q", errText)
	}
}

func TestExecute_ReadOnlyAllowsReadCommand(t *testing.T) {
	_ = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if err := Execute([]string{"--read-only", "version"}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
}

func TestExecute_ReadOnlyFromEnvBlocksMutatingCommand(t *testing.T) {
	t.Setenv("GOG_READ_ONLY", "true")

	errText := captureStderr(t, func() {
		_ = captureStdout(t, func() {
			err := Execute([]string{"config", "set", "foo", "bar"})
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), "read-only mode blocks mutating command") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	})
	if !strings.Contains(errText, "read-only mode blocks mutating command") {
		t.Fatalf("expected read-only error on stderr, got: %q", errText)
	}
}
