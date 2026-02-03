package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersGetCmd struct {
	User       string `arg:"" name:"user" help:"User email or ID"`
	Projection string `name:"projection" default:"full" enum:"basic,full,custom" help:"Data projection"`
}

func (c *UsersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	user, err := svc.Users.Get(c.User).Projection(c.Projection).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get user %s: %w", c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, user)
	}

	name := ""
	if user.Name != nil {
		name = strings.TrimSpace(strings.Join([]string{user.Name.GivenName, user.Name.FamilyName}, " "))
	}

	u.Out().Printf("Email:              %s\n", user.PrimaryEmail)
	u.Out().Printf("Name:               %s\n", name)
	u.Out().Printf("ID:                 %s\n", user.Id)
	u.Out().Printf("Is Admin:           %v\n", user.IsAdmin)
	u.Out().Printf("Is Delegated Admin: %v\n", user.IsDelegatedAdmin)
	u.Out().Printf("Suspended:          %v\n", user.Suspended)
	if user.SuspensionReason != "" {
		u.Out().Printf("Suspension Reason:  %s\n", user.SuspensionReason)
	}
	u.Out().Printf("Archived:           %v\n", user.Archived)
	u.Out().Printf("Org Unit:           %s\n", user.OrgUnitPath)
	u.Out().Printf("Creation Time:      %s\n", user.CreationTime)
	u.Out().Printf("Last Login:         %s\n", user.LastLoginTime)
	u.Out().Printf("Agreed to Terms:    %v\n", user.AgreedToTerms)
	u.Out().Printf("Change Password:    %v\n", user.ChangePasswordAtNextLogin)
	u.Out().Printf("2SV Enrolled:       %v\n", user.IsEnrolledIn2Sv)
	u.Out().Printf("2SV Enforced:       %v\n", user.IsEnforcedIn2Sv)

	if user.RecoveryEmail != "" {
		u.Out().Printf("Recovery Email:     %s\n", user.RecoveryEmail)
	}
	if user.RecoveryPhone != "" {
		u.Out().Printf("Recovery Phone:     %s\n", user.RecoveryPhone)
	}

	if len(user.Aliases) > 0 {
		u.Out().Printf("Aliases:            %v\n", user.Aliases)
	}
	if len(user.NonEditableAliases) > 0 {
		u.Out().Printf("Non-Editable:       %v\n", user.NonEditableAliases)
	}

	return nil
}
