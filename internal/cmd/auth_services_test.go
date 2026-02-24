package cmd

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func TestAuthServices_JSON(t *testing.T) {
	u, uiErr := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if uiErr != nil {
		t.Fatalf("ui.New: %v", uiErr)
	}
	ctx := ui.WithUI(context.Background(), u)
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		cmd := &AuthServicesCmd{}
		if err := cmd.Run(ctx, &RootFlags{}); err != nil {
			t.Fatalf("run: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse json: %v", err)
	}
	raw, ok := parsed["services"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("missing services in output")
	}

	var docs map[string]any
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if svc, _ := item["service"].(string); svc == "docs" {
			docs = item
			break
		}
	}
	if docs == nil {
		t.Fatalf("missing docs service")
	}

	scopes, _ := docs["scopes"].([]any)
	if !containsString(scopes, "https://www.googleapis.com/auth/drive") {
		t.Fatalf("docs missing drive scope")
	}
	if !containsString(scopes, "https://www.googleapis.com/auth/documents") {
		t.Fatalf("docs missing documents scope")
	}
}

func containsString(items []any, want string) bool {
	for _, item := range items {
		if s, _ := item.(string); s == want {
			return true
		}
	}
	return false
}
