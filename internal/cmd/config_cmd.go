package cmd

import (
	"context"
	"fmt"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
)

type ConfigCmd struct {
	Get    ConfigGetCmd    `cmd:"" aliases:"show" help:"Get a config value"`
	Keys   ConfigKeysCmd   `cmd:"" aliases:"list-keys,names" help:"List available config keys"`
	Set    ConfigSetCmd    `cmd:"" aliases:"add,update" help:"Set a config value"`
	Unset  ConfigUnsetCmd  `cmd:"" aliases:"rm,del,remove" help:"Unset a config value"`
	List   ConfigListCmd   `cmd:"" aliases:"ls,all" help:"List all config values"`
	Path   ConfigPathCmd   `cmd:"" aliases:"where" help:"Print config file path"`
	NoSend ConfigNoSendCmd `cmd:"" name:"no-send" aliases:"nosend" help:"Manage per-account Gmail no-send guards"`
}

type ConfigGetCmd struct {
	Key string `arg:"" help:"Config key to get (timezone)"`
}

func (c *ConfigGetCmd) Run(ctx context.Context) error {
	cfg, err := loadConfig(ctx)
	if err != nil {
		return err
	}

	key, err := config.ParseKey(c.Key)
	if err != nil {
		return usage(err.Error())
	}
	spec, err := config.KeySpecFor(key)
	if err != nil {
		return usage(err.Error())
	}
	value := config.GetValue(cfg, key)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), outfmt.KeyValuePayload(key.String(), value))
	}
	fmt.Fprintln(stdoutWriter(ctx), formatConfigValue(value, spec.EmptyHint))
	return nil
}

type ConfigKeysCmd struct{}

func (c *ConfigKeysCmd) Run(ctx context.Context) error {
	keys := config.KeyNames()
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), outfmt.KeysPayload(keys))
	}
	for _, key := range keys {
		fmt.Fprintln(stdoutWriter(ctx), key)
	}
	return nil
}

type ConfigSetCmd struct {
	Key   string `arg:"" help:"Config key to set (timezone)"`
	Value string `arg:"" help:"Value to set"`
}

func (c *ConfigSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}

	cfg, err := store.Read()
	if err != nil {
		return err
	}

	key, err := config.ParseKey(c.Key)
	if err != nil {
		return usage(err.Error())
	}

	if err := config.SetValue(&cfg, key, c.Value); err != nil {
		return usage(err.Error())
	}

	if err := dryRunExit(ctx, flags, "config.set", map[string]any{
		"key":   key.String(),
		"value": c.Value,
	}); err != nil {
		return err
	}

	if err := store.Write(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := outfmt.KeyValuePayload(key.String(), c.Value)
		payload["saved"] = true
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	fmt.Fprintf(stdoutWriter(ctx), "Set %s = %s\n", c.Key, c.Value)
	return nil
}

type ConfigUnsetCmd struct {
	Key string `arg:"" help:"Config key to unset (timezone)"`
}

func (c *ConfigUnsetCmd) Run(ctx context.Context, flags *RootFlags) error {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}

	cfg, err := store.Read()
	if err != nil {
		return err
	}

	key, err := config.ParseKey(c.Key)
	if err != nil {
		return usage(err.Error())
	}

	if err := config.UnsetValue(&cfg, key); err != nil {
		return usage(err.Error())
	}

	if err := dryRunExit(ctx, flags, "config.unset", map[string]any{
		"key": key.String(),
	}); err != nil {
		return err
	}

	if err := store.Write(cfg); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		payload := outfmt.KeyValuePayload(key.String(), "")
		payload["removed"] = true
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	fmt.Fprintf(stdoutWriter(ctx), "Unset %s\n", c.Key)
	return nil
}

type ConfigListCmd struct{}

func (c *ConfigListCmd) Run(ctx context.Context) error {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}

	cfg, err := store.Read()
	if err != nil {
		return err
	}

	path := store.Path()
	keys := config.KeyList()

	if outfmt.IsJSON(ctx) {
		payload := outfmt.PathPayload(path)
		for _, key := range keys {
			payload[key.String()] = config.GetValue(cfg, key)
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	fmt.Fprintf(stdoutWriter(ctx), "Config file: %s\n", path)
	for _, key := range keys {
		value := config.GetValue(cfg, key)
		fmt.Fprintf(stdoutWriter(ctx), "%s: %s\n", key, formatConfigValue(value, func() string { return "(not set)" }))
	}
	return nil
}

type ConfigPathCmd struct{}

func (c *ConfigPathCmd) Run(ctx context.Context) error {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	path := store.Path()

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), outfmt.PathPayload(path))
	}
	fmt.Fprintln(stdoutWriter(ctx), path)
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

func loadConfig(ctx context.Context) (config.File, error) {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return config.File{}, err
	}
	return store.Read()
}

func commandConfigStore(ctx context.Context) (*config.ConfigStore, error) {
	if runtime, ok := app.FromContext(ctx); ok {
		if err := configureRuntimeConfig(runtime); err != nil {
			return nil, err
		}
		return runtime.Config, nil
	}
	return nil, errRuntimeRequired
}
