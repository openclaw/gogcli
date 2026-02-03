package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersTurnOff2SVCmd struct {
	User string `arg:"" name:"user" help:"User email or ID"`
}

func (c *UsersTurnOff2SVCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("turn off 2-step verification for %s", c.User)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.TwoStepVerification.TurnOff(c.User).Context(ctx).Do(); err != nil {
		return fmt.Errorf("turn off 2SV for %s: %w", c.User, err)
	}

	u.Out().Printf("Turned off 2-step verification for: %s\n", c.User)
	return nil
}

type UsersBackupCodesCmd struct {
	List     UsersBackupCodesListCmd     `cmd:"" name:"list" aliases:"show" help:"List backup codes"`
	Generate UsersBackupCodesGenerateCmd `cmd:"" name:"generate" aliases:"create" help:"Generate new backup codes"`
	Delete   UsersBackupCodesDeleteCmd   `cmd:"" name:"delete" aliases:"rm" help:"Delete all backup codes"`
}

type UsersBackupCodesListCmd struct {
	User string `arg:"" name:"user" help:"User email or ID"`
}

func (c *UsersBackupCodesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	codes, err := svc.VerificationCodes.List(c.User).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list backup codes for %s: %w", c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, codes)
	}

	if len(codes.Items) == 0 {
		u.Err().Println("No backup codes found")
		return nil
	}

	u.Out().Printf("Backup codes for %s:\n", c.User)
	for _, code := range codes.Items {
		u.Out().Printf("  %s\n", code.VerificationCode)
	}

	return nil
}

type UsersBackupCodesGenerateCmd struct {
	User string `arg:"" name:"user" help:"User email or ID"`
}

func (c *UsersBackupCodesGenerateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.VerificationCodes.Generate(c.User).Context(ctx).Do(); err != nil {
		return fmt.Errorf("generate backup codes for %s: %w", c.User, err)
	}

	u.Out().Printf("Generated new backup codes for: %s\n", c.User)
	u.Out().Println("Use 'gog users backupcodes list' to view the new codes")

	return nil
}

type UsersBackupCodesDeleteCmd struct {
	User string `arg:"" name:"user" help:"User email or ID"`
}

func (c *UsersBackupCodesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete all backup codes for %s", c.User)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.VerificationCodes.Invalidate(c.User).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete backup codes for %s: %w", c.User, err)
	}

	u.Out().Printf("Deleted all backup codes for: %s\n", c.User)
	return nil
}
