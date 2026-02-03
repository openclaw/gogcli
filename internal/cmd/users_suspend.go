package cmd

import (
	"context"
	"fmt"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/ui"
)

type UsersSuspendCmd struct {
	User string `arg:"" name:"user" help:"User email or ID to suspend"`
}

func (c *UsersSuspendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	user := &admin.User{Suspended: true}
	user.ForceSendFields = append(user.ForceSendFields, "Suspended")

	if _, err := svc.Users.Update(c.User, user).Context(ctx).Do(); err != nil {
		return fmt.Errorf("suspend user %s: %w", c.User, err)
	}

	u.Out().Printf("Suspended user: %s\n", c.User)
	return nil
}

type UsersUnsuspendCmd struct {
	User string `arg:"" name:"user" help:"User email or ID to unsuspend"`
}

func (c *UsersUnsuspendCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	user := &admin.User{Suspended: false}
	user.ForceSendFields = append(user.ForceSendFields, "Suspended")

	if _, err := svc.Users.Update(c.User, user).Context(ctx).Do(); err != nil {
		return fmt.Errorf("unsuspend user %s: %w", c.User, err)
	}

	u.Out().Printf("Unsuspended user: %s\n", c.User)
	return nil
}
