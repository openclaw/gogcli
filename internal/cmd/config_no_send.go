package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
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
	if err := config.SetNoSendAccount(account, true); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"account": account,
			"noSend":  true,
			"saved":   true,
		})
	}
	fmt.Fprintf(os.Stdout, "No-send enabled for %s\n", account)
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
	if err := config.SetNoSendAccount(account, false); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"account": account,
			"noSend":  false,
			"removed": true,
		})
	}
	fmt.Fprintf(os.Stdout, "No-send removed for %s\n", account)
	return nil
}

type ConfigNoSendListCmd struct{}

func (c *ConfigNoSendListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	accounts := config.NoSendAccountList(cfg)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"accounts": accounts})
	}
	if len(accounts) == 0 {
		if u != nil {
			u.Err().Println("No no-send accounts")
			return nil
		}
		fmt.Fprintln(os.Stderr, "No no-send accounts")
		return nil
	}
	for _, account := range accounts {
		fmt.Fprintln(os.Stdout, account)
	}
	return nil
}
