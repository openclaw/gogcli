package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DomainsListCmd struct {
	ToDrive ToDriveFlags `embed:""`
}

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

	if len(resp.Domains) == 0 {
		u.Err().Println("No domains found")
		return nil
	}

	rows := make([][]string, 0, len(resp.Domains))
	for _, domain := range resp.Domains {
		if domain == nil {
			continue
		}
		rows = append(rows, toDriveRow(
			domain.DomainName,
			toDriveBool(domain.IsPrimary),
			toDriveBool(domain.Verified),
			formatUnixSeconds(domain.CreationTime),
		))
	}

	if ok, err := writeToDrive(ctx, flags, toDriveTitle("Domains", c.ToDrive), []string{"DOMAIN", "PRIMARY", "VERIFIED", "CREATED"}, rows, c.ToDrive); ok {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "DOMAIN\tPRIMARY\tVERIFIED\tCREATED")
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(row[0]),
			sanitizeTab(row[1]),
			sanitizeTab(row[2]),
			sanitizeTab(row[3]),
		)
	}

	return nil
}
