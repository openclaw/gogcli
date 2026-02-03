package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DomainsListCmd struct{}

func (c *DomainsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Domains.List(adminCustomerID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Domains) == 0 {
		u.Err().Println("No domains found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "DOMAIN\tPRIMARY\tVERIFIED\tCREATED")
	for _, domain := range resp.Domains {
		if domain == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%t\t%t\t%s\n",
			sanitizeTab(domain.DomainName),
			domain.IsPrimary,
			domain.Verified,
			formatUnixSeconds(domain.CreationTime),
		)
	}

	return nil
}
