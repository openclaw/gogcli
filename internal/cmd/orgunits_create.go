package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type OrgunitsCreateCmd struct {
	Name        string `arg:"" name:"name" help:"Org unit name"`
	Parent      string `name:"parent" help:"Parent org unit path"`
	Description string `name:"description" help:"Description"`
}

func (c *OrgunitsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	parent := strings.TrimSpace(c.Parent)
	if parent == "" {
		parent = "/"
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	org := &admin.OrgUnit{
		Name:              c.Name,
		ParentOrgUnitPath: parent,
		Description:       c.Description,
	}

	created, err := svc.Orgunits.Insert(adminCustomerID, org).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create org unit: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created org unit: %s (%s)\n", created.Name, created.OrgUnitPath)
	return nil
}
