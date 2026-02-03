package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type OrgunitsGetCmd struct {
	Path string `arg:"" name:"path" help:"Org unit path or ID"`
}

func (c *OrgunitsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	ou, err := svc.Orgunits.Get(adminCustomerID, c.Path).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get org unit %s: %w", c.Path, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, ou)
	}

	u.Out().Printf("Name:        %s\n", ou.Name)
	u.Out().Printf("Path:        %s\n", ou.OrgUnitPath)
	u.Out().Printf("ID:          %s\n", ou.OrgUnitId)
	if ou.ParentOrgUnitPath != "" {
		u.Out().Printf("Parent Path: %s\n", ou.ParentOrgUnitPath)
	}
	if ou.ParentOrgUnitId != "" {
		u.Out().Printf("Parent ID:   %s\n", ou.ParentOrgUnitId)
	}
	if ou.Description != "" {
		u.Out().Printf("Description: %s\n", ou.Description)
	}
	return nil
}
