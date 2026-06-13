package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
)

type timezoneResolveMode int

const (
	timezoneExplicitOnly timezoneResolveMode = iota
	timezoneWithFallback
)

const (
	flagTimezoneLabel   = "timezone"
	envTimezoneLabel    = "GOG_TIMEZONE"
	configTimezoneLabel = "default_timezone"
	warnConfigFallback  = "warning: invalid %s in config %q, using local timezone\n"
	warnConfigIgnore    = "warning: invalid %s in config %q, ignoring\n"
)

func resolveOutputLocation(ctx context.Context, timezone string, local bool, diagnostics io.Writer) (*time.Location, error) {
	return resolveTimezone(ctx, timezone, local, timezoneWithFallback, diagnostics)
}

// getConfiguredTimezone returns the timezone from flag, env var, or config file.
// Returns nil if no timezone is explicitly configured. The special value "local"
// returns time.Local to explicitly use the local timezone.
func getConfiguredTimezone(ctx context.Context, timezone string, diagnostics io.Writer) (*time.Location, error) {
	return resolveTimezone(ctx, timezone, false, timezoneExplicitOnly, diagnostics)
}

func resolveTimezone(ctx context.Context, timezone string, local bool, mode timezoneResolveMode, diagnostics io.Writer) (*time.Location, error) {
	if local {
		return time.Local, nil
	}

	if loc, ok, err := parseTimezoneValue(flagTimezoneLabel, timezone, true); ok || err != nil {
		return loc, err
	}

	if loc, ok, err := parseTimezoneValue(envTimezoneLabel, os.Getenv("GOG_TIMEZONE"), false); ok || err != nil {
		return loc, err
	}

	if cfg, ok := readConfigOptional(ctx); ok && cfg.DefaultTimezone != "" {
		loc, ok, err := parseTimezoneValue(configTimezoneLabel, cfg.DefaultTimezone, false)
		if ok {
			if err != nil {
				warnInvalidConfigTimezone(diagnostics, cfg.DefaultTimezone, mode)
			} else {
				return loc, nil
			}
		}
	}

	if mode == timezoneWithFallback {
		return time.Local, nil
	}

	// No explicit timezone configured; nil signals caller to use its own fallback.
	return nil, nil //nolint:nilnil // intentional: nil means no config, let caller decide fallback
}

func parseTimezoneValue(label, value string, allowLocal bool) (*time.Location, bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false, nil
	}
	if allowLocal && strings.EqualFold(trimmed, "local") {
		return time.Local, true, nil
	}
	loc, err := loadTimezoneLocation(trimmed)
	if err != nil {
		return nil, true, fmt.Errorf("invalid %s %q: %w", label, trimmed, err)
	}
	return loc, true, nil
}

func loadTimezoneLocation(timezone string) (*time.Location, error) {
	return time.LoadLocation(strings.TrimSpace(timezone))
}

func tryLoadTimezoneLocation(timezone string) (*time.Location, bool) {
	loc, err := loadTimezoneLocation(timezone)
	if err != nil {
		return nil, false
	}
	return loc, true
}

func readConfigOptional(ctx context.Context) (config.File, bool) {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return config.File{}, false
	}
	cfg, err := store.Read()
	if err != nil {
		return config.File{}, false
	}
	return cfg, true
}

func warnInvalidConfigTimezone(diagnostics io.Writer, value string, mode timezoneResolveMode) {
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	if mode == timezoneWithFallback {
		fmt.Fprintf(diagnostics, warnConfigFallback, configTimezoneLabel, value)
		return
	}
	fmt.Fprintf(diagnostics, warnConfigIgnore, configTimezoneLabel, value)
}
