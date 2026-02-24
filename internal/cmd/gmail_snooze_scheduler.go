package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailSnoozeSchedulerCmd installs or removes a platform-appropriate scheduler
// (launchd on macOS, systemd timer on Linux) to periodically wake snoozed threads.
type GmailSnoozeSchedulerCmd struct {
	Interval  int    `name:"interval" default:"5" help:"Check interval in minutes"`
	Uninstall bool   `name:"uninstall" aliases:"remove" help:"Remove the scheduled task"`
	Platform  string `name:"platform" help:"Override platform: macos|linux (default: auto-detect)"`
}

const (
	launchdLabel = "com.gogcli.gmail-snooze-wake"
	systemdUnit  = "gogcli-gmail-snooze-wake"
)

func (c *GmailSnoozeSchedulerCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	platform := strings.ToLower(strings.TrimSpace(c.Platform))
	if platform == "" {
		switch runtime.GOOS {
		case "darwin":
			platform = "macos"
		case "linux":
			platform = "linux"
		default:
			return fmt.Errorf("scheduler not supported on %s; run 'gog gmail snooze wake' manually via cron", runtime.GOOS)
		}
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine binary path: %w", err)
	}
	// Resolve symlinks so the plist/unit always references the real binary.
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	intervalSecs := c.Interval * 60
	if intervalSecs <= 0 {
		intervalSecs = 300 // 5-minute default
	}

	switch platform {
	case "macos", "darwin":
		return c.runMacOS(ctx, flags, u, execPath, intervalSecs)
	case "linux":
		return c.runLinux(ctx, flags, u, execPath, intervalSecs)
	default:
		return fmt.Errorf("unknown platform %q: choose macos or linux", c.Platform)
	}
}

// runMacOS installs or removes the launchd plist.
func (c *GmailSnoozeSchedulerCmd) runMacOS(ctx context.Context, flags *RootFlags, u *ui.UI, execPath string, intervalSecs int) error {
	home := os.Getenv("HOME")
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")

	if c.Uninstall {
		if err := dryRunExit(ctx, flags, "gmail.snooze.scheduler.uninstall", map[string]any{
			"platform":  "macos",
			"plistPath": plistPath,
		}); err != nil {
			return err
		}
		// Unload — ignore errors (agent may not be loaded yet).
		_ = runCmd(ctx, "launchctl", "unload", plistPath)
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing plist: %w", err)
		}
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"uninstalled": true,
				"platform":    "macos",
				"plistPath":   plistPath,
			})
		}
		if u != nil {
			u.Out().Printf("Removed launchd agent: %s", plistPath)
		}
		return nil
	}

	if err := dryRunExit(ctx, flags, "gmail.snooze.scheduler.install", map[string]any{
		"platform":    "macos",
		"plistPath":   plistPath,
		"intervalSec": intervalSecs,
	}); err != nil {
		return err
	}

	plistContent := buildLaunchdPlist(execPath, intervalSecs)
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o700); err != nil {
		return fmt.Errorf("creating LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil { //nolint:gosec // plist must be world-readable for launchd
		return fmt.Errorf("writing plist: %w", err)
	}

	// Unload first in case it was previously loaded (ignore error).
	_ = runCmd(ctx, "launchctl", "unload", plistPath)
	if err := runCmd(ctx, "launchctl", "load", "-w", plistPath); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"installed":   true,
			"platform":    "macos",
			"plistPath":   plistPath,
			"intervalMin": c.Interval,
		})
	}
	if u != nil {
		u.Out().Printf("Installed launchd agent: %s (every %d min)", plistPath, c.Interval)
	}
	return nil
}

// runLinux installs or removes the systemd user service and timer.
func (c *GmailSnoozeSchedulerCmd) runLinux(ctx context.Context, flags *RootFlags, u *ui.UI, execPath string, intervalSecs int) error {
	home := os.Getenv("HOME")
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	serviceFile := filepath.Join(unitDir, systemdUnit+".service")
	timerFile := filepath.Join(unitDir, systemdUnit+".timer")

	if c.Uninstall {
		if err := dryRunExit(ctx, flags, "gmail.snooze.scheduler.uninstall", map[string]any{
			"platform":    "linux",
			"serviceFile": serviceFile,
			"timerFile":   timerFile,
		}); err != nil {
			return err
		}
		_ = runCmd(ctx, "systemctl", "--user", "disable", "--now", systemdUnit+".timer")
		for _, f := range []string{serviceFile, timerFile} {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing %s: %w", f, err)
			}
		}
		_ = runCmd(ctx, "systemctl", "--user", "daemon-reload")
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
				"uninstalled": true,
				"platform":    "linux",
				"serviceFile": serviceFile,
				"timerFile":   timerFile,
			})
		}
		if u != nil {
			u.Out().Printf("Removed systemd user units: %s, %s", serviceFile, timerFile)
		}
		return nil
	}

	if err := dryRunExit(ctx, flags, "gmail.snooze.scheduler.install", map[string]any{
		"platform":    "linux",
		"serviceFile": serviceFile,
		"timerFile":   timerFile,
		"intervalSec": intervalSecs,
	}); err != nil {
		return err
	}

	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		return fmt.Errorf("creating systemd user unit directory: %w", err)
	}

	serviceContent := buildSystemdService(execPath)
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0o644); err != nil { //nolint:gosec // systemd unit must be world-readable
		return fmt.Errorf("writing service unit: %w", err)
	}

	timerContent := buildSystemdTimer(systemdUnit, intervalSecs)
	if err := os.WriteFile(timerFile, []byte(timerContent), 0o644); err != nil { //nolint:gosec // systemd unit must be world-readable
		return fmt.Errorf("writing timer unit: %w", err)
	}

	if err := runCmd(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w", err)
	}
	if err := runCmd(ctx, "systemctl", "--user", "enable", "--now", systemdUnit+".timer"); err != nil {
		return fmt.Errorf("systemctl enable timer: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"installed":   true,
			"platform":    "linux",
			"serviceFile": serviceFile,
			"timerFile":   timerFile,
			"intervalMin": c.Interval,
		})
	}
	if u != nil {
		u.Out().Printf("Installed systemd user timer: %s (every %d min)", timerFile, c.Interval)
	}
	return nil
}

// buildLaunchdPlist returns the XML content for a launchd plist that runs
// `gog gmail snooze wake --all` every intervalSecs seconds.
func buildLaunchdPlist(execPath string, intervalSecs int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>gmail</string>
        <string>snooze</string>
        <string>wake</string>
        <string>--all</string>
    </array>
    <key>StartInterval</key>
    <integer>%d</integer>
    <key>RunAtLoad</key>
    <false/>
    <key>StandardOutPath</key>
    <string>/tmp/gogcli-gmail-snooze-wake.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/gogcli-gmail-snooze-wake.log</string>
</dict>
</plist>
`, launchdLabel, execPath, intervalSecs)
}

// buildSystemdService returns the content for a systemd service unit that runs
// `gog gmail snooze wake --all`.
func buildSystemdService(execPath string) string {
	return fmt.Sprintf(`[Unit]
Description=gogcli Gmail snooze wake
After=network.target

[Service]
Type=oneshot
ExecStart=%s gmail snooze wake --all
`, execPath)
}

// buildSystemdTimer returns the content for a systemd timer unit that activates
// the service every intervalSecs seconds.
func buildSystemdTimer(unitName string, intervalSecs int) string {
	return fmt.Sprintf(`[Unit]
Description=gogcli Gmail snooze wake timer

[Timer]
OnBootSec=60
OnUnitActiveSec=%d
Unit=%s.service

[Install]
WantedBy=timers.target
`, intervalSecs, unitName)
}

// runCmd executes an external command with a context, passing through its stderr to os.Stderr.
func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
