package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailForwardingCmd struct {
	List   GmailForwardingListCmd   `cmd:"" name:"list" aliases:"ls" help:"List all forwarding addresses"`
	Get    GmailForwardingGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a specific forwarding address"`
	Create GmailForwardingCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create/add a forwarding address"`
	Delete GmailForwardingDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a forwarding address"`
}

type GmailForwardingListCmd struct{}

func (c *GmailForwardingListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.ForwardingAddresses.List("me").Do()
	if err != nil {
		return err
	}
	rows := make([]gmailEmailStatusRow, 0, len(resp.ForwardingAddresses))
	for _, f := range resp.ForwardingAddresses {
		if f == nil {
			continue
		}
		rows = append(rows, gmailEmailStatusRow{
			Email:  f.ForwardingEmail,
			Status: f.VerificationStatus,
		})
	}
	return writeGmailEmailStatusList(ctx, "forwardingAddresses", resp.ForwardingAddresses, "No forwarding addresses", rows)
}

type GmailForwardingGetCmd struct {
	ForwardingEmail string `arg:"" name:"forwardingEmail" help:"Forwarding email"`
}

func (c *GmailForwardingGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	forwardingEmail := strings.TrimSpace(c.ForwardingEmail)
	if forwardingEmail == "" {
		return usage("empty forwardingEmail")
	}
	address, err := svc.Users.Settings.ForwardingAddresses.Get("me", forwardingEmail).Do()
	if err != nil {
		return err
	}
	return writeGmailEmailStatusItem(ctx, "forwardingAddress", address, "forwarding_email", gmailEmailStatusRow{
		Email:  address.ForwardingEmail,
		Status: address.VerificationStatus,
	})
}

type GmailForwardingCreateCmd struct {
	ForwardingEmail string `arg:"" name:"forwardingEmail" help:"Forwarding email"`
}

func (c *GmailForwardingCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	forwardingEmail := strings.TrimSpace(c.ForwardingEmail)
	if forwardingEmail == "" {
		return usage("empty forwardingEmail")
	}

	if err := dryRunExit(ctx, flags, "gmail.forwarding.create", map[string]any{
		"forwarding_email": forwardingEmail,
	}); err != nil {
		return err
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	address := &gmail.ForwardingAddress{
		ForwardingEmail: forwardingEmail,
	}

	created, err := svc.Users.Settings.ForwardingAddresses.Create("me", address).Do()
	if err != nil {
		return err
	}
	return writeGmailEmailStatusCreateResult(
		ctx,
		"forwardingAddress",
		created,
		"forwarding_email",
		gmailEmailStatusRow{Email: created.ForwardingEmail, Status: created.VerificationStatus},
		"Forwarding address created successfully",
		"",
		"A verification email has been sent to the forwarding address.",
		"The address cannot be used until the recipient confirms the verification link.",
	)
}

type GmailForwardingDeleteCmd struct {
	ForwardingEmail string `arg:"" name:"forwardingEmail" help:"Forwarding email"`
}

func (c *GmailForwardingDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	forwardingEmail := strings.TrimSpace(c.ForwardingEmail)
	if forwardingEmail == "" {
		return usage("empty forwardingEmail")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete gmail forwarding address %s", forwardingEmail)); confirmErr != nil {
		return confirmErr
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.ForwardingAddresses.Delete("me", forwardingEmail).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"success":         true,
			"forwardingEmail": forwardingEmail,
		})
	}

	ui.FromContext(ctx).Out().Printf("Forwarding address %s deleted successfully", forwardingEmail)
	return nil
}
