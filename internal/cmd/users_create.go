package cmd

import (
	"context"
	"fmt"
	"os"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersCreateCmd struct {
	Email          string `arg:"" name:"email" help:"Primary email address"`
	FirstName      string `name:"first-name" aliases:"given-name,fn" required:"" help:"First/given name"`
	LastName       string `name:"last-name" aliases:"family-name,ln" required:"" help:"Last/family name"`
	Password       string `name:"password" aliases:"pass" help:"Password (generated if not specified)"`
	OrgUnit        string `name:"org-unit" aliases:"ou" default:"/" help:"Organizational unit path"`
	ChangePassword bool   `name:"change-password" default:"true" help:"Require password change on first login"`
	Suspended      bool   `name:"suspended" help:"Create user in suspended state"`
	Archived       bool   `name:"archived" help:"Create user in archived state"`
	RecoveryEmail  string `name:"recovery-email" help:"Recovery email address"`
	RecoveryPhone  string `name:"recovery-phone" help:"Recovery phone number (E.164 format)"`
	HashFunction   string `name:"hash-function" help:"Password hash function if pre-hashed (MD5, SHA-1, crypt)"`
}

func (c *UsersCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	password := c.Password
	generated := false
	if password == "" {
		password, err = generatePassword(16)
		if err != nil {
			return fmt.Errorf("generate password: %w", err)
		}
		generated = true
	}

	user := &admin.User{
		PrimaryEmail: c.Email,
		Name: &admin.UserName{
			GivenName:  c.FirstName,
			FamilyName: c.LastName,
		},
		Password:                  password,
		ChangePasswordAtNextLogin: c.ChangePassword,
		OrgUnitPath:               c.OrgUnit,
		Suspended:                 c.Suspended,
		Archived:                  c.Archived,
	}

	if c.HashFunction != "" {
		var hash string
		hash, err = normalizeUserHashFunction(c.HashFunction)
		if err != nil {
			return err
		}
		user.HashFunction = hash
	}
	if c.RecoveryEmail != "" {
		user.RecoveryEmail = c.RecoveryEmail
	}
	if c.RecoveryPhone != "" {
		user.RecoveryPhone = c.RecoveryPhone
	}

	created, err := svc.Users.Insert(user).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"user": created,
		}
		if generated {
			result["generatedPassword"] = password
		}
		return outfmt.WriteJSON(os.Stdout, result)
	}

	u.Out().Printf("Created user: %s\n", created.PrimaryEmail)
	u.Out().Printf("User ID: %s\n", created.Id)
	if generated {
		u.Out().Printf("Generated password: %s\n", password)
	}
	if c.ChangePassword {
		u.Out().Println("User must change password on first login")
	}

	return nil
}
