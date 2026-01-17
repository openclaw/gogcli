package cmd

import (
	"context"
	"fmt"
	"os"

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

type ConfigGetCmd struct {
	Key string `arg:"" help:"Config key to get (timezone)"`
}

func (c *ConfigGetCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	key, err := config.ParseKey(c.Key)
	if err != nil {
		return err
	}
	spec, err := config.KeySpecFor(key)
	if err != nil {
		return err
	}
	value := config.GetValue(cfg, key)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, outfmt.KeyValuePayload(key.String(), value))
	}
	fmt.Fprintln(os.Stdout, formatConfigValue(value, spec.EmptyHint))
	return nil
}

type ConfigKeysCmd struct{}

func (c *ConfigKeysCmd) Run(ctx context.Context) error {
	keys := config.KeyNames()
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, outfmt.KeysPayload(keys))
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

	key, err := config.ParseKey(c.Key)
	if err != nil {
		return err
	}

	if err := config.SetValue(&cfg, key, c.Value); err != nil {
		return err
	}

	if err := config.WriteConfig(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := outfmt.KeyValuePayload(key.String(), c.Value)
		payload["saved"] = true
		return outfmt.WriteJSON(os.Stdout, payload)
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

	key, err := config.ParseKey(c.Key)
	if err != nil {
		return err
	}

	if err := config.UnsetValue(&cfg, key); err != nil {
		return err
	}

	if err := config.WriteConfig(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := outfmt.KeyValuePayload(key.String(), "")
		payload["removed"] = true
		return outfmt.WriteJSON(os.Stdout, payload)
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
	keys := config.KeyList()

	if outfmt.IsJSON(ctx) {
		payload := outfmt.PathPayload(path)
		for _, key := range keys {
			payload[key.String()] = config.GetValue(cfg, key)
		}
		return outfmt.WriteJSON(os.Stdout, payload)
	}

	fmt.Fprintf(os.Stdout, "Config file: %s\n", path)
	for _, key := range keys {
		value := config.GetValue(cfg, key)
		fmt.Fprintf(os.Stdout, "%s: %s\n", key, formatConfigValue(value, func() string { return "(not set)" }))
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
		return outfmt.WriteJSON(os.Stdout, outfmt.PathPayload(path))
	}
	fmt.Fprintln(os.Stdout, path)
	return nil
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

func loadConfig() (config.File, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		return config.File{}, err
	}
	return cfg, nil
}
