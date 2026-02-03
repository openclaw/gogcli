package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DomainsGetCmd struct {
	Domain string `arg:"" name:"domain" help:"Domain name"`
}

func (c *DomainsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	domain, err := svc.Domains.Get(adminCustomerID, c.Domain).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get domain %s: %w", c.Domain, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, domain)
	}

	u.Out().Printf("Domain:    %s\n", domain.DomainName)
	u.Out().Printf("Primary:   %v\n", domain.IsPrimary)
	u.Out().Printf("Verified:  %v\n", domain.Verified)
	u.Out().Printf("Created:   %s\n", formatUnixSeconds(domain.CreationTime))
	if len(domain.DomainAliases) > 0 {
		u.Out().Printf("Aliases:   %d\n", len(domain.DomainAliases))
		for _, alias := range domain.DomainAliases {
			if alias == nil {
				continue
			}
			u.Out().Printf("  - %s\n", alias.DomainAliasName)
		}
	}
	return nil
}
