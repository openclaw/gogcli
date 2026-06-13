package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBodyInput_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.txt")
	if err := os.WriteFile(path, []byte("Line 1\nLine 2\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := resolveBodyInput(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), "", path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "Line 1\nLine 2\n" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestResolveBodyInput_Stdin(t *testing.T) {
	ctx := newCmdRuntimeIOContext(t, strings.NewReader("stdin body"), io.Discard, io.Discard)
	got, err := resolveBodyInput(ctx, "", "-")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "stdin body" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestResolveBodyInput_Conflict(t *testing.T) {
	_, err := resolveBodyInput(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), "body", "/tmp/body.txt")
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "--body") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveComposeBodyInputs_RejectsDoubleStdin(t *testing.T) {
	ctx := newCmdRuntimeIOContext(t, strings.NewReader("body"), io.Discard, io.Discard)
	_, _, err := resolveComposeBodyInputs(ctx, "", "-", "", "-")
	if err == nil || !strings.Contains(err.Error(), "use stdin for only one") {
		t.Fatalf("unexpected error: %v", err)
	}
}
