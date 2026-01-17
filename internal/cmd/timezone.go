package cmd

import (
	"fmt"
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

func resolveOutputLocation(timezone string, local bool) (*time.Location, error) {
	return resolveTimezone(timezone, local, timezoneWithFallback)
}

// getConfiguredTimezone returns the timezone from flag, env var, or config file.
// Returns nil if no timezone is explicitly configured. The special value "local"
// returns time.Local to explicitly use the local timezone.
func getConfiguredTimezone(timezone string) (*time.Location, error) {
	return resolveTimezone(timezone, false, timezoneExplicitOnly)
}

func resolveTimezone(timezone string, local bool, mode timezoneResolveMode) (*time.Location, error) {
	if local {
		return time.Local, nil
	}

	if loc, ok, err := parseTimezoneValue(flagTimezoneLabel, timezone, true); ok || err != nil {
		return loc, err
	}

	if loc, ok, err := parseTimezoneValue(envTimezoneLabel, os.Getenv("GOG_TIMEZONE"), false); ok || err != nil {
		return loc, err
	}

	if cfg, ok := readConfigOptional(); ok && cfg.DefaultTimezone != "" {
		loc, ok, err := parseTimezoneValue(configTimezoneLabel, cfg.DefaultTimezone, false)
		if ok {
			if err != nil {
				warnInvalidConfigTimezone(cfg.DefaultTimezone, mode)
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
	loc, err := time.LoadLocation(trimmed)
	if err != nil {
		return nil, true, fmt.Errorf("invalid %s %q: %w", label, trimmed, err)
	}
	return loc, true, nil
}

func readConfigOptional() (config.File, bool) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return config.File{}, false
	}
	return cfg, true
}

func warnInvalidConfigTimezone(value string, mode timezoneResolveMode) {
	if mode == timezoneWithFallback {
		fmt.Fprintf(os.Stderr, warnConfigFallback, configTimezoneLabel, value)
		return
	}
	fmt.Fprintf(os.Stderr, warnConfigIgnore, configTimezoneLabel, value)
}
