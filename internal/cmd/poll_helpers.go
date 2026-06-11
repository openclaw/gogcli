package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

const pollStateVersion = 1

type pollRuntime struct {
	now     func() time.Time
	runHook func(context.Context, string, any) error
	wait    func(context.Context, time.Duration) error
}

func defaultPollRuntime() pollRuntime {
	return pollRuntime{
		now:     time.Now,
		runHook: runJSONShellHook,
		wait:    waitForPollInterval,
	}
}

func (r pollRuntime) withDefaults() pollRuntime {
	defaults := defaultPollRuntime()
	if r.now == nil {
		r.now = defaults.now
	}
	if r.runHook == nil {
		r.runHook = defaults.runHook
	}
	if r.wait == nil {
		r.wait = defaults.wait
	}
	return r
}

func pollSignalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

func waitForPollInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runJSONShellHook(ctx context.Context, command string, payload any) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode poll hook payload: %w", err)
	}
	data = append(data, '\n')

	var cmd *exec.Cmd
	if os.PathSeparator == '\\' {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", command) //nolint:gosec // command is an explicit local operator-provided hook.
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command) //nolint:gosec // command is an explicit local operator-provided hook.
	}
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("shell hook failed: %w", err)
	}
	return nil
}

func expandPollStatePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", usage("missing --state-file")
	}
	path, err := config.ExpandPath(raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) == "" {
		return "", usage("empty --state-file")
	}
	return path, nil
}

func readPollState(path string, dst any) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // explicit user-provided state path.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read poll state %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return false, fmt.Errorf("decode poll state %s: %w", path, err)
	}
	return true, nil
}

func writePollState(path string, state any) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode poll state: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create poll state directory: %w", err)
	}
	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write poll state %s: %w", path, err)
	}
	return nil
}

func writePollJSON(ctx context.Context, payload any) error {
	var rendered bytes.Buffer
	if err := outfmt.WriteJSON(ctx, &rendered, payload); err != nil {
		return err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, rendered.Bytes()); err != nil {
		return fmt.Errorf("compact poll json: %w", err)
	}
	if err := compact.WriteByte('\n'); err != nil {
		return err
	}
	if _, err := stdoutWriter(ctx).Write(compact.Bytes()); err != nil {
		return fmt.Errorf("write poll output: %w", err)
	}
	return nil
}
