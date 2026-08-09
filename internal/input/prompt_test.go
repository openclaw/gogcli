package input

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/ui"
)

func TestPromptLineFrom(t *testing.T) {
	var stderr bytes.Buffer

	u, err := ui.New(ui.Options{Stdout: &stderr, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	line, err := PromptLineFrom(ctx, "Prompt: ", strings.NewReader("hello\n"))
	if err != nil {
		t.Fatalf("PromptLineFrom: %v", err)
	}

	if line != "hello" {
		t.Fatalf("unexpected line: %q", line)
	}

	if !strings.Contains(stderr.String(), "Prompt: ") {
		t.Fatalf("expected prompt in stderr: %q", stderr.String())
	}
}

func TestPromptLine(t *testing.T) {
	orig := os.Stdin

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() {
		_ = r.Close()
		os.Stdin = orig
	}()
	os.Stdin = r

	_, writeErr := w.WriteString("world\n")
	if writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	_ = w.Close()

	line, err := PromptLine(context.Background(), "Prompt: ")
	if err != nil {
		t.Fatalf("PromptLine: %v", err)
	}

	if line != "world" {
		t.Fatalf("unexpected line: %q", line)
	}
}
