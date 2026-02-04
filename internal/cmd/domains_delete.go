package cmd

import (
	"context"
	"fmt"

	"github.com/steipete/gogcli/internal/ui"
)

type DomainsDeleteCmd struct {
	Domain string `arg:"" name:"domain" help:"Domain name"`
}

func (c *DomainsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete domain %s", c.Domain)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Domains.Delete(adminCustomerID(), c.Domain).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete domain %s: %w", c.Domain, err)
	}

	u.Out().Printf("Deleted domain: %s\n", c.Domain)
	return nil
}
