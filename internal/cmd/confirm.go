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
	return confirmDestructiveChecked(ctx, flags, action)
}

func confirmDestructiveChecked(ctx context.Context, flags *RootFlags, action string) error {
	if flags == nil || flags.Force {
		return nil
	}

	// Never prompt in non-interactive contexts.
	if flags.NoInput || !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // os file descriptor fits int on supported targets
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
	if ans == "y" || ans == sendAsYes {
		return nil
	}
	return &ExitError{Code: 1, Err: errors.New("cancelled")}
}

func flagsWithoutDryRun(flags *RootFlags) *RootFlags {
	if flags == nil {
		return nil
	}
	clone := *flags
	clone.DryRun = false
	return &clone
}

func dryRunAndConfirmDestructive(ctx context.Context, flags *RootFlags, op string, request any, action string) error {
	if err := dryRunExit(ctx, flags, op, request); err != nil {
		return err
	}
	return confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), action)
}
