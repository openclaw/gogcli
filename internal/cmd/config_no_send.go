package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ConfigNoSendCmd struct {
	Set    ConfigNoSendSetCmd    `cmd:"" aliases:"add,enable" help:"Block Gmail send operations for an account"`
	Remove ConfigNoSendRemoveCmd `cmd:"" aliases:"rm,del,delete,unset,disable" help:"Remove an account no-send guard"`
	List   ConfigNoSendListCmd   `cmd:"" aliases:"ls" help:"List accounts with no-send guards"`
}

type ConfigNoSendSetCmd struct {
	Account string `arg:"" help:"Account email to guard"`
}

func (c *ConfigNoSendSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account := strings.TrimSpace(c.Account)
	if account == "" {
		return usage("missing account")
	}
	if err := dryRunExit(ctx, flags, "config.no-send.set", map[string]any{"account": account}); err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	if err := store.SetNoSendAccount(account, true); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"account": account,
			"noSend":  true,
			"saved":   true,
		})
	}
	fmt.Fprintf(stdoutWriter(ctx), "No-send enabled for %s\n", account)
	return nil
}

type ConfigNoSendRemoveCmd struct {
	Account string `arg:"" help:"Account email to unguard"`
}

func (c *ConfigNoSendRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	account := strings.TrimSpace(c.Account)
	if account == "" {
		return usage("missing account")
	}
	if err := dryRunExit(ctx, flags, "config.no-send.remove", map[string]any{"account": account}); err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	if err := store.SetNoSendAccount(account, false); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"account": account,
			"noSend":  false,
			"removed": true,
		})
	}
	fmt.Fprintf(stdoutWriter(ctx), "No-send removed for %s\n", account)
	return nil
}

type ConfigNoSendListCmd struct{}

func (c *ConfigNoSendListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	cfg, err := loadConfig(ctx)
	if err != nil {
		return err
	}
	accounts := config.NoSendAccountList(cfg)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"accounts": accounts})
	}
	if len(accounts) == 0 {
		if u != nil {
			u.Err().Println("No no-send accounts")
			return nil
		}
		fmt.Fprintln(stderrWriter(ctx), "No no-send accounts")
		return nil
	}
	for _, account := range accounts {
		fmt.Fprintln(stdoutWriter(ctx), account)
	}
	return nil
}
