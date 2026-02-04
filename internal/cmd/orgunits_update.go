package cmd

import (
	"context"
	"fmt"
	"os"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type OrgunitsUpdateCmd struct {
	Path        string  `arg:"" name:"path" help:"Org unit path or ID"`
	Name        *string `name:"name" help:"New org unit name"`
	Parent      *string `name:"parent" help:"New parent org unit path"`
	Description *string `name:"description" help:"Description"`
}

func (c *OrgunitsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	org := &admin.OrgUnit{}
	hasUpdates := false

	if c.Name != nil {
		org.Name = *c.Name
		hasUpdates = true
	}
	if c.Parent != nil {
		org.ParentOrgUnitPath = *c.Parent
		hasUpdates = true
	}
	if c.Description != nil {
		org.Description = *c.Description
		if *c.Description == "" {
			org.ForceSendFields = append(org.ForceSendFields, "Description")
		}
		hasUpdates = true
	}

	if !hasUpdates {
		return usage("no updates specified")
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	updated, err := svc.Orgunits.Update(adminCustomerID(), c.Path, org).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update org unit %s: %w", c.Path, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated org unit: %s (%s)\n", updated.Name, updated.OrgUnitPath)
	return nil
}
