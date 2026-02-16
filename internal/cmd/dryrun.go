package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// dryRunExit prints the intended operation and exits successfully (exit code 0).
// Call this from mutating commands early to avoid touching auth/keyring or making API calls.
func dryRunExit(ctx context.Context, flags *RootFlags, op string, request any) error {
	if flags == nil || !flags.DryRun {
		return nil
	}

	if outfmt.IsJSON(ctx) {
		jsonCtx := outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{})
		_ = outfmt.WriteJSON(jsonCtx, os.Stdout, map[string]any{
			"dry_run": true,
			"op":      op,
			"request": request,
		})
		return &ExitError{Code: 0, Err: nil}
	}

	if outfmt.IsPlain(ctx) {
		fmt.Fprintf(os.Stdout, "dry_run\ttrue\n")
		fmt.Fprintf(os.Stdout, "op\t%s\n", op)
		if request != nil {
			if b, err := json.Marshal(request); err == nil {
				fmt.Fprintf(os.Stdout, "request_json\t%s\n", string(b))
			}
		}
		return &ExitError{Code: 0, Err: nil}
	}

	if u := ui.FromContext(ctx); u != nil {
		u.Out().Printf("Dry run: would %s", op)
		if request != nil {
			if b, err := json.MarshalIndent(request, "", "  "); err == nil {
				u.Out().Println(string(b))
			}
		}
		return &ExitError{Code: 0, Err: nil}
	}

	fmt.Printf("Dry run: would %s\n", op)
	return &ExitError{Code: 0, Err: nil}
}
