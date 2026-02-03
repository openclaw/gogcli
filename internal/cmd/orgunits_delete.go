package cmd

import (
	"context"
	"fmt"

	"github.com/steipete/gogcli/internal/ui"
)

type OrgunitsDeleteCmd struct {
	Path string `arg:"" name:"path" help:"Org unit path or ID"`
}

func (c *OrgunitsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete org unit %s", c.Path)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Orgunits.Delete(adminCustomerID, c.Path).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete org unit %s: %w", c.Path, err)
	}

	u.Out().Printf("Deleted org unit: %s\n", c.Path)
	return nil
}
