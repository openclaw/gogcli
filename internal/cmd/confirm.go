package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/steipete/gogcli/internal/input"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func confirmDestructive(ctx context.Context, flags *RootFlags, action string) error {
	if flags != nil && flags.DryRun {
		// Dry-run exits successfully after printing the intended action.
		if outfmt.IsJSON(ctx) {
			_ = outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"dry_run": true,
				"action":  action,
			})
		} else if outfmt.IsPlain(ctx) {
			fmt.Fprintf(os.Stdout, "dry_run\ttrue\n")
			fmt.Fprintf(os.Stdout, "action\t%s\n", action)
		} else if u := ui.FromContext(ctx); u != nil {
			u.Out().Printf("Dry run: would %s", action)
		} else {
			fmt.Printf("Dry run: would %s\n", action)
		}
		return &ExitError{Code: 0, Err: nil}
	}
	if flags.Force {
		return nil
	}

	// Never prompt in non-interactive contexts.
	if flags.NoInput || !term.IsTerminal(int(os.Stdin.Fd())) {
		return usagef("refusing to %s without --force (non-interactive)", action)
	}

	prompt := fmt.Sprintf("Proceed to %s? [y/N]: ", action)
	line, readErr := input.PromptLine(ctx, prompt)
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) {
		if errors.Is(readErr, io.EOF) {
			return &ExitError{Code: 1, Err: errors.New("cancelled")}
		}
		return fmt.Errorf("read confirmation: %w", readErr)
	}
	ans := strings.TrimSpace(strings.ToLower(line))
	if ans == "y" || ans == "yes" {
		return nil
	}
	return &ExitError{Code: 1, Err: errors.New("cancelled")}
}
