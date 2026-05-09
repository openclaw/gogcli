package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/ui"
)

func TestWarnTasksDueTime(t *testing.T) {
	var stderr bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &bytes.Buffer{}, Stderr: &stderr, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}

	warnTasksDueTime(u, "2025-01-01")
	if stderr.Len() != 0 {
		t.Fatalf("date-only due should not warn, got %q", stderr.String())
	}

	warnTasksDueTime(u, "2025-01-01T10:00:00Z")
	if !strings.Contains(stderr.String(), "Google Tasks treats due dates as date-only") {
		t.Fatalf("expected datetime warning, got %q", stderr.String())
	}

	stderr.Reset()
	warnTasksDueTime(u, "2025-01-01 10:00")
	if !strings.Contains(stderr.String(), "Google Tasks treats due dates as date-only") {
		t.Fatalf("expected time warning, got %q", stderr.String())
	}
}
