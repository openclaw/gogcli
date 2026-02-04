package cmd

import (
	"context"
	"fmt"
	"os"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DomainsCreateCmd struct {
	Domain string `arg:"" name:"domain" help:"Domain name"`
}

func (c *DomainsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	req := &admin.Domains{DomainName: c.Domain}
	created, err := svc.Domains.Insert(adminCustomerID(), req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create domain %s: %w", c.Domain, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created domain: %s\n", created.DomainName)
	return nil
}
