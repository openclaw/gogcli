package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/outfmt"
)

func TestDryRunExit_JSON_IgnoresResultsOnlyTransform(t *testing.T) {
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{ResultsOnly: true})

	out := captureStdout(t, func() {
		err := dryRunExit(ctx, &RootFlags{DryRun: true}, "send", map[string]any{"to": "a@example.com"})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected exit code 0, got: %v", err)
		}
	})

	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected json output")
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput=%q", err, out)
	}
	if got["dry_run"] != true {
		t.Fatalf("expected dry_run=true, got=%v", got["dry_run"])
	}
	if got["op"] != "send" {
		t.Fatalf("expected op=send, got=%v", got["op"])
	}
	if _, ok := got["request"]; !ok {
		t.Fatalf("expected request field, got=%v", got)
	}
}

func TestDryRunExit_JSON_IgnoresSelectTransform(t *testing.T) {
	ctx := context.Background()
	ctx = outfmt.WithMode(ctx, outfmt.Mode{JSON: true})
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{Select: []string{"request"}})

	out := captureStdout(t, func() {
		err := dryRunExit(ctx, &RootFlags{DryRun: true}, "drive.upload", map[string]any{"name": "x.txt"})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 0 {
			t.Fatalf("expected exit code 0, got: %v", err)
		}
	})

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput=%q", err, out)
	}
	if got["dry_run"] != true {
		t.Fatalf("expected dry_run=true, got=%v", got["dry_run"])
	}
	if got["op"] != "drive.upload" {
		t.Fatalf("expected op=drive.upload, got=%v", got["op"])
	}
	if _, ok := got["request"]; !ok {
		t.Fatalf("expected request field, got=%v", got)
	}
}
