package cmd

import (
	"context"
	"fmt"
	"os"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersUpdateCmd struct {
	User           string  `arg:"" name:"user" help:"User email or ID"`
	FirstName      *string `name:"first-name" aliases:"given-name,fn" help:"First/given name"`
	LastName       *string `name:"last-name" aliases:"family-name,ln" help:"Last/family name"`
	PrimaryEmail   *string `name:"primary-email" help:"Change primary email address"`
	OrgUnit        *string `name:"org-unit" aliases:"ou" help:"Organizational unit path"`
	Suspended      *bool   `name:"suspended" help:"Suspended state"`
	Archived       *bool   `name:"archived" help:"Archived state"`
	RecoveryEmail  *string `name:"recovery-email" help:"Recovery email"`
	RecoveryPhone  *string `name:"recovery-phone" help:"Recovery phone"`
	ChangePassword *bool   `name:"change-password" help:"Require password change on next login"`
	Admin          *bool   `name:"admin" help:"Super admin status (use with caution)"`
}

func (c *UsersUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	user := &admin.User{}
	hasFieldUpdates := false

	if c.FirstName != nil || c.LastName != nil {
		user.Name = &admin.UserName{}
		if c.FirstName != nil {
			user.Name.GivenName = *c.FirstName
			hasFieldUpdates = true
		}
		if c.LastName != nil {
			user.Name.FamilyName = *c.LastName
			hasFieldUpdates = true
		}
	}
	if c.PrimaryEmail != nil {
		user.PrimaryEmail = *c.PrimaryEmail
		hasFieldUpdates = true
	}
	if c.OrgUnit != nil {
		user.OrgUnitPath = *c.OrgUnit
		hasFieldUpdates = true
	}
	if c.Suspended != nil {
		user.Suspended = *c.Suspended
		user.ForceSendFields = append(user.ForceSendFields, "Suspended")
		hasFieldUpdates = true
	}
	if c.Archived != nil {
		user.Archived = *c.Archived
		user.ForceSendFields = append(user.ForceSendFields, "Archived")
		hasFieldUpdates = true
	}
	if c.RecoveryEmail != nil {
		user.RecoveryEmail = *c.RecoveryEmail
		if *c.RecoveryEmail == "" {
			user.ForceSendFields = append(user.ForceSendFields, "RecoveryEmail")
		}
		hasFieldUpdates = true
	}
	if c.RecoveryPhone != nil {
		user.RecoveryPhone = *c.RecoveryPhone
		if *c.RecoveryPhone == "" {
			user.ForceSendFields = append(user.ForceSendFields, "RecoveryPhone")
		}
		hasFieldUpdates = true
	}
	if c.ChangePassword != nil {
		user.ChangePasswordAtNextLogin = *c.ChangePassword
		user.ForceSendFields = append(user.ForceSendFields, "ChangePasswordAtNextLogin")
		hasFieldUpdates = true
	}

	if c.Admin == nil && !hasFieldUpdates {
		return usage("no updates specified")
	}

	if c.Admin != nil {
		if err = svc.Users.MakeAdmin(c.User, &admin.UserMakeAdmin{Status: *c.Admin}).Context(ctx).Do(); err != nil {
			return fmt.Errorf("update admin status for %s: %w", c.User, err)
		}
		if !hasFieldUpdates {
			u.Out().Printf("Updated admin status for: %s\n", c.User)
			return nil
		}
	}

	updated, err := svc.Users.Update(c.User, user).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("update user %s: %w", c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	u.Out().Printf("Updated user: %s\n", updated.PrimaryEmail)
	return nil
}
