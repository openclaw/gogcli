package cmd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestAgentExitCodes_JSON(t *testing.T) {
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{JSON: true})

	out := captureStdout(t, func() {
		if err := (&AgentExitCodesCmd{}).Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	var doc struct {
		ExitCodes map[string]int `json:"exit_codes"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v (out=%q)", err, out)
	}
	if doc.ExitCodes["empty_results"] != emptyResultsExitCode {
		t.Fatalf("expected empty_results=%d, got %d", emptyResultsExitCode, doc.ExitCodes["empty_results"])
	}
	if doc.ExitCodes["auth_required"] != exitCodeAuthRequired {
		t.Fatalf("expected auth_required=%d, got %d", exitCodeAuthRequired, doc.ExitCodes["auth_required"])
	}
}
