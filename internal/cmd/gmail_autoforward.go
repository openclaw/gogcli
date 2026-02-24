package cmd

import (
	"context"
	"errors"
	"os"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailAutoForwardCmd struct {
	Get    GmailAutoForwardGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get current auto-forwarding settings"`
	Update GmailAutoForwardUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update auto-forwarding settings"`
}

type GmailAutoForwardGetCmd struct{}

func (c *GmailAutoForwardGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	autoForward, err := svc.Users.Settings.GetAutoForwarding("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"autoForwarding": autoForward})
	}

	u.Out().Printf("enabled\t%t", autoForward.Enabled)
	if autoForward.EmailAddress != "" {
		u.Out().Printf("email_address\t%s", autoForward.EmailAddress)
	}
	if autoForward.Disposition != "" {
		u.Out().Printf("disposition\t%s", autoForward.Disposition)
	}
	return nil
}

type GmailAutoForwardUpdateCmd struct {
	Enable      bool   `name:"enable" help:"Enable auto-forwarding"`
	Disable     bool   `name:"disable" help:"Disable auto-forwarding"`
	Email       string `name:"email" help:"Email address to forward to (must be verified first)"`
	Disposition string `name:"disposition" help:"What to do with forwarded messages: leaveInInbox, archive, trash, markRead"`
}

func (c *GmailAutoForwardUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Enable && c.Disable {
		return errors.New("cannot specify both --enable and --disable")
	}

	updates := map[string]any{}
	if c.Enable {
		updates["enabled"] = true
	}
	if c.Disable {
		updates["enabled"] = false
	}
	if flagProvided(kctx, "email") {
		updates["email_address"] = c.Email
	}
	if flagProvided(kctx, "disposition") {
		// Validate disposition value
		validDispositions := map[string]bool{
			"leaveInInbox": true,
			"archive":      true,
			"trash":        true,
			"markRead":     true,
		}
		if !validDispositions[c.Disposition] {
			return errors.New("invalid disposition value; must be one of: leaveInInbox, archive, trash, markRead")
		}
		updates["disposition"] = c.Disposition
	}

	if err := dryRunExit(ctx, flags, "gmail.autoforward.update", map[string]any{
		"updates": updates,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Get current settings first
	current, err := svc.Users.Settings.GetAutoForwarding("me").Do()
	if err != nil {
		return err
	}

	// Build update request, preserving existing values if not specified
	autoForward := &gmail.AutoForwarding{
		Enabled:      current.Enabled,
		EmailAddress: current.EmailAddress,
		Disposition:  current.Disposition,
	}

	// Apply flags
	if c.Enable {
		autoForward.Enabled = true
	}
	if c.Disable {
		autoForward.Enabled = false
	}
	if flagProvided(kctx, "email") {
		autoForward.EmailAddress = c.Email
	}
	if flagProvided(kctx, "disposition") {
		autoForward.Disposition = c.Disposition
	}

	updated, err := svc.Users.Settings.UpdateAutoForwarding("me", autoForward).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"autoForwarding": updated})
	}

	u.Out().Println("Auto-forwarding settings updated successfully")
	u.Out().Printf("enabled\t%t", updated.Enabled)
	if updated.EmailAddress != "" {
		u.Out().Printf("email_address\t%s", updated.EmailAddress)
	}
	if updated.Disposition != "" {
		u.Out().Printf("disposition\t%s", updated.Disposition)
	}
	return nil
}
