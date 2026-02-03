package cmd

import (
	"context"
	"fmt"
	"os"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersPasswordCmd struct {
	User           string `arg:"" name:"user" help:"User email or ID"`
	Password       string `name:"password" aliases:"pass" help:"New password (generated if not specified)"`
	ChangePassword bool   `name:"change-password" default:"true" help:"Require password change on next login"`
	HashFunction   string `name:"hash-function" help:"Password hash function if pre-hashed (MD5, SHA-1, crypt)"`
}

func (c *UsersPasswordCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		Password:                  password,
		ChangePasswordAtNextLogin: c.ChangePassword,
	}
	user.ForceSendFields = append(user.ForceSendFields, "ChangePasswordAtNextLogin")
	if c.HashFunction != "" {
		hash, err := normalizeUserHashFunction(c.HashFunction)
		if err != nil {
			return err
		}
		user.HashFunction = hash
	}

	updated, err := svc.Users.Update(c.User, user).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("reset password for %s: %w", c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"user": updated.PrimaryEmail,
		}
		if generated {
			result["generatedPassword"] = password
		}
		return outfmt.WriteJSON(os.Stdout, result)
	}

	u.Out().Printf("Password reset for: %s\n", updated.PrimaryEmail)
	if generated {
		u.Out().Printf("New password: %s\n", password)
	}
	if c.ChangePassword {
		u.Out().Println("User must change password on next login")
	}

	return nil
}
