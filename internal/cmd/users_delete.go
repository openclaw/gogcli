package cmd

import (
	"context"
	"fmt"

	"github.com/steipete/gogcli/internal/ui"
)

type UsersDeleteCmd struct {
	User string `arg:"" name:"user" help:"User email or ID to delete"`
}

func (c *UsersDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete user %s", c.User)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Users.Delete(c.User).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete user %s: %w", c.User, err)
	}

	u.Out().Printf("Deleted user: %s\n", c.User)
	return nil
}
