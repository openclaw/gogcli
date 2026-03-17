package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
)

type ConfigNoSendCmd struct {
	Set    ConfigNoSendSetCmd    `cmd:"" aliases:"add,block" help:"Block send for an account"`
	Remove ConfigNoSendRemoveCmd `cmd:"" aliases:"rm,unblock,allow" help:"Allow send for an account"`
	List   ConfigNoSendListCmd   `cmd:"" aliases:"ls" help:"List accounts with send blocked"`
}

type ConfigNoSendSetCmd struct {
	Account string `arg:"" help:"Account email to block send for"`
}

func (c *ConfigNoSendSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := dryRunExit(ctx, flags, "config.no-send.set", map[string]any{
		"account": c.Account,
	}); err != nil {
		return err
	}

	if err := config.UpdateConfig(func(cfg *config.File) error {
		return config.SetNoSendAccount(cfg, c.Account, true)
	}); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"account": c.Account,
			"noSend":  true,
		})
	}
	fmt.Fprintf(os.Stdout, "Send blocked for %s\n", c.Account)
	return nil
}

type ConfigNoSendRemoveCmd struct {
	Account string `arg:"" help:"Account email to allow send for"`
}

func (c *ConfigNoSendRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	if err := dryRunExit(ctx, flags, "config.no-send.remove", map[string]any{
		"account": c.Account,
	}); err != nil {
		return err
	}

	if err := config.UpdateConfig(func(cfg *config.File) error {
		return config.SetNoSendAccount(cfg, c.Account, false)
	}); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"account": c.Account,
			"noSend":  false,
		})
	}
	fmt.Fprintf(os.Stdout, "Send allowed for %s\n", c.Account)
	return nil
}

type ConfigNoSendListCmd struct{}

func (c *ConfigNoSendListCmd) Run(ctx context.Context) error {
	cfg, err := config.ReadConfig()
	if err != nil {
		return err
	}

	accounts := config.ListNoSendAccounts(cfg)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"noSendAccounts": accounts,
		})
	}

	if len(accounts) == 0 {
		fmt.Fprintln(os.Stdout, "No accounts have send blocked")
		return nil
	}
	for _, acct := range accounts {
		fmt.Fprintln(os.Stdout, acct)
	}
	return nil
}
