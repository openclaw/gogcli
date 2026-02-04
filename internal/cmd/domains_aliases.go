package cmd

import (
	"context"
	"fmt"
	"os"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DomainsAliasesListCmd struct{}

func (c *DomainsAliasesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.DomainAliases.List(adminCustomerID()).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list domain aliases: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.DomainAliases) == 0 {
		u.Err().Println("No domain aliases found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ALIAS\tPARENT DOMAIN\tVERIFIED\tCREATED")
	for _, alias := range resp.DomainAliases {
		if alias == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\n",
			sanitizeTab(alias.DomainAliasName),
			sanitizeTab(alias.ParentDomainName),
			alias.Verified,
			formatUnixSeconds(alias.CreationTime),
		)
	}
	return nil
}

type DomainsAliasesCreateCmd struct {
	Alias  string `arg:"" name:"alias" help:"Domain alias to create"`
	Parent string `name:"parent" required:"" help:"Parent domain"`
}

func (c *DomainsAliasesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	req := &admin.DomainAlias{
		DomainAliasName:  c.Alias,
		ParentDomainName: c.Parent,
	}
	created, err := svc.DomainAliases.Insert(adminCustomerID(), req).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create domain alias %s: %w", c.Alias, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created domain alias: %s (parent: %s)\n", created.DomainAliasName, created.ParentDomainName)
	return nil
}

type DomainsAliasesDeleteCmd struct {
	Alias string `arg:"" name:"alias" help:"Domain alias to delete"`
}

func (c *DomainsAliasesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete domain alias %s", c.Alias)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.DomainAliases.Delete(adminCustomerID(), c.Alias).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete domain alias %s: %w", c.Alias, err)
	}

	u.Out().Printf("Deleted domain alias: %s\n", c.Alias)
	return nil
}
