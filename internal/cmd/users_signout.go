package cmd

import (
	"context"
	"fmt"

	"github.com/steipete/gogcli/internal/ui"
)

type UsersSignoutCmd struct {
	User string `arg:"" name:"user" help:"User email or ID to sign out"`
}

func (c *UsersSignoutCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Users.SignOut(c.User).Context(ctx).Do(); err != nil {
		return fmt.Errorf("sign out user %s: %w", c.User, err)
	}

	u.Out().Printf("Signed out user from all sessions: %s\n", c.User)
	return nil
}
