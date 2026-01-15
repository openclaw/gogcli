package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

type ConfigCmd struct {
	Get   ConfigGetCmd   `cmd:"" help:"Get a config value"`
	Set   ConfigSetCmd   `cmd:"" help:"Set a config value"`
	Unset ConfigUnsetCmd `cmd:"" help:"Unset a config value"`
	List  ConfigListCmd  `cmd:"" help:"List all config values"`
	Path  ConfigPathCmd  `cmd:"" help:"Print config file path"`
}

type ConfigGetCmd struct {
	Key string `arg:"" help:"Config key to get (timezone)"`
}

func (c *ConfigGetCmd) Run(ctx context.Context) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	var value string
	switch c.Key {
	case "timezone":
		value = cfg.DefaultTimezone
		if value == "" {
			value = "(not set, using local: " + time.Local.String() + ")"
		}
	case "keyring_backend":
		value = cfg.KeyringBackend
		if value == "" {
			value = "(not set, using auto)"
		}
	default:
		return fmt.Errorf("unknown config key: %s (valid keys: timezone, keyring_backend)", c.Key)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"key":   c.Key,
			"value": value,
		})
	}
	fmt.Fprintln(os.Stdout, value)
	return nil
}

type ConfigSetCmd struct {
	Key   string `arg:"" help:"Config key to set (timezone)"`
	Value string `arg:"" help:"Value to set"`
}

func (c *ConfigSetCmd) Run(ctx context.Context) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	switch c.Key {
	case "timezone":
		// Validate timezone
		if _, err := time.LoadLocation(c.Value); err != nil {
			return fmt.Errorf("invalid timezone %q: %w (use IANA timezone names like America/New_York, UTC, Europe/London)", c.Value, err)
		}
		cfg.DefaultTimezone = c.Value
	case "keyring_backend":
		cfg.KeyringBackend = c.Value
	default:
		return fmt.Errorf("unknown config key: %s (valid keys: timezone, keyring_backend)", c.Key)
	}

	if err := config.WriteConfig(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"key":   c.Key,
			"value": c.Value,
			"saved": true,
		})
	}
	fmt.Fprintf(os.Stdout, "Set %s = %s\n", c.Key, c.Value)
	return nil
}

type ConfigUnsetCmd struct {
	Key string `arg:"" help:"Config key to unset (timezone)"`
}

func (c *ConfigUnsetCmd) Run(ctx context.Context) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	switch c.Key {
	case "timezone":
		cfg.DefaultTimezone = ""
	case "keyring_backend":
		cfg.KeyringBackend = ""
	default:
		return fmt.Errorf("unknown config key: %s (valid keys: timezone, keyring_backend)", c.Key)
	}

	if err := config.WriteConfig(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"key":     c.Key,
			"removed": true,
		})
	}
	fmt.Fprintf(os.Stdout, "Unset %s\n", c.Key)
	return nil
}

type ConfigListCmd struct{}

func (c *ConfigListCmd) Run(ctx context.Context) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	path, _ := config.ConfigPath()

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"path":            path,
			"timezone":        cfg.DefaultTimezone,
			"keyring_backend": cfg.KeyringBackend,
		})
	}

	fmt.Fprintf(os.Stdout, "Config file: %s\n", path)
	fmt.Fprintf(os.Stdout, "timezone: %s\n", valueOrDefault(cfg.DefaultTimezone, "(not set)"))
	fmt.Fprintf(os.Stdout, "keyring_backend: %s\n", valueOrDefault(cfg.KeyringBackend, "(not set)"))
	return nil
}

type ConfigPathCmd struct{}

func (c *ConfigPathCmd) Run(ctx context.Context) error {
	path, err := config.ConfigPath()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"path": path,
		})
	}
	fmt.Fprintln(os.Stdout, path)
	return nil
}

func valueOrDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
