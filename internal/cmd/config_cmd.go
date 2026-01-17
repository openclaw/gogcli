package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

type ConfigCmd struct {
	Get   ConfigGetCmd   `cmd:"" help:"Get a config value"`
	Keys  ConfigKeysCmd  `cmd:"" help:"List available config keys"`
	Set   ConfigSetCmd   `cmd:"" help:"Set a config value"`
	Unset ConfigUnsetCmd `cmd:"" help:"Unset a config value"`
	List  ConfigListCmd  `cmd:"" help:"List all config values"`
	Path  ConfigPathCmd  `cmd:"" help:"Print config file path"`
}

type configKeySpec struct {
	Get       func(config.File) string
	Set       func(*config.File, string) error
	Unset     func(*config.File)
	EmptyHint func() string
}

var configKeyOrder = []string{"timezone", "keyring_backend"}

var configKeySpecs = map[string]configKeySpec{
	"timezone": {
		Get: func(cfg config.File) string {
			return cfg.DefaultTimezone
		},
		Set: func(cfg *config.File, value string) error {
			if _, err := time.LoadLocation(value); err != nil {
				return fmt.Errorf("invalid timezone %q: %w (use IANA timezone names like America/New_York, UTC, Europe/London)", value, err)
			}
			cfg.DefaultTimezone = value
			return nil
		},
		Unset: func(cfg *config.File) {
			cfg.DefaultTimezone = ""
		},
		EmptyHint: func() string {
			return "(not set, using local: " + time.Local.String() + ")"
		},
	},
	"keyring_backend": {
		Get: func(cfg config.File) string {
			return cfg.KeyringBackend
		},
		Set: func(cfg *config.File, value string) error {
			cfg.KeyringBackend = value
			return nil
		},
		Unset: func(cfg *config.File) {
			cfg.KeyringBackend = ""
		},
		EmptyHint: func() string {
			return "(not set, using auto)"
		},
	},
}

type ConfigGetCmd struct {
	Key string `arg:"" help:"Config key to get (timezone)"`
}

func (c *ConfigGetCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	spec, err := configKeySpecFor(c.Key)
	if err != nil {
		return err
	}
	values := configKeyValues(cfg)
	value := values[c.Key]

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, configJSONPayload("", values, c.Key))
	}
	fmt.Fprintln(os.Stdout, formatConfigValue(value, spec.EmptyHint))
	return nil
}

type ConfigKeysCmd struct{}

func (c *ConfigKeysCmd) Run(ctx context.Context) error {
	keys := configKeyNames()
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"keys": keys,
		})
	}
	for _, key := range keys {
		fmt.Fprintln(os.Stdout, key)
	}
	return nil
}

type ConfigSetCmd struct {
	Key   string `arg:"" help:"Config key to set (timezone)"`
	Value string `arg:"" help:"Value to set"`
}

func (c *ConfigSetCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	spec, err := configKeySpecFor(c.Key)
	if err != nil {
		return err
	}

	if err := spec.Set(&cfg, c.Value); err != nil {
		return err
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
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	spec, err := configKeySpecFor(c.Key)
	if err != nil {
		return err
	}

	if spec.Unset == nil {
		return fmt.Errorf("config key %s cannot be unset", c.Key)
	}
	spec.Unset(&cfg)

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
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	path, _ := config.ConfigPath()

	if outfmt.IsJSON(ctx) {
		values := configKeyValues(cfg)
		return outfmt.WriteJSON(os.Stdout, configJSONPayload(path, values, ""))
	}

	fmt.Fprintf(os.Stdout, "Config file: %s\n", path)
	for _, key := range configKeyOrder {
		spec, err := configKeySpecFor(key)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "%s: %s\n", key, formatConfigValue(spec.Get(cfg), func() string { return "(not set)" }))
	}
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

func configKeySpecFor(key string) (configKeySpec, error) {
	spec, ok := configKeySpecs[key]
	if !ok {
		return configKeySpec{}, fmt.Errorf("unknown config key: %s (valid keys: %s)", key, strings.Join(configKeyNames(), ", "))
	}
	return spec, nil
}

func configKeyNames() []string {
	names := make([]string, 0, len(configKeySpecs))
	for name := range configKeySpecs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func formatConfigValue(value string, emptyHint func() string) string {
	if value != "" {
		return value
	}
	if emptyHint != nil {
		return emptyHint()
	}
	return "(not set)"
}

func configKeyValues(cfg config.File) map[string]string {
	values := make(map[string]string, len(configKeySpecs))
	for name, spec := range configKeySpecs {
		values[name] = spec.Get(cfg)
	}
	return values
}

func configJSONPayload(path string, values map[string]string, key string) map[string]any {
	if key != "" {
		return map[string]any{
			"key":   key,
			"value": values[key],
		}
	}

	payload := map[string]any{
		"path": path,
	}
	for _, name := range configKeyOrder {
		if value, ok := values[name]; ok {
			payload[name] = value
		}
	}
	return payload
}

func loadConfig() (config.File, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return config.File{}, err
	}
	return cfg, nil
}
