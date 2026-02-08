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
)

func confirmDestructive(ctx context.Context, flags *RootFlags, action string) error {
	if err := dryRunExit(ctx, flags, action, nil); err != nil {
		return err
	}
	if flags == nil || flags.Force {
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
