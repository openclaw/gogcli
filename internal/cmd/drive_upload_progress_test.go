package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestDriveUploadReader_ReportsProgressOnStderr(t *testing.T) {
	var stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)
	data := bytes.Repeat([]byte("x"), driveUploadProgressMinBytes)

	wrapped := driveUploadReader(ctx, bytes.NewReader(data), driveUploadOptions{size: int64(len(data))})
	if _, err := io.ReadAll(wrapped); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "upload:") || !strings.Contains(out, "100%") {
		t.Fatalf("expected upload progress, got %q", out)
	}
	if count := strings.Count(out, "100%"); count != 1 {
		t.Fatalf("expected one final progress line, got %d in %q", count, out)
	}
}

func TestDriveUploadReader_SkipsProgressForJSON(t *testing.T) {
	var stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := outfmt.WithMode(ui.WithUI(context.Background(), u), outfmt.Mode{JSON: true})
	data := bytes.Repeat([]byte("x"), driveUploadProgressMinBytes)

	wrapped := driveUploadReader(ctx, bytes.NewReader(data), driveUploadOptions{size: int64(len(data))})
	if _, err := io.ReadAll(wrapped); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no JSON-mode progress, got %q", stderr.String())
	}
}
