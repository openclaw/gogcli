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

type GmailDelegatesCmd struct {
	List   GmailDelegatesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List all delegates"`
	Get    GmailDelegatesGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a specific delegate's information"`
	Add    GmailDelegatesAddCmd    `cmd:"" name:"add" aliases:"create,new" help:"Add a delegate"`
	Remove GmailDelegatesRemoveCmd `cmd:"" name:"remove" aliases:"delete,rm,del" help:"Remove a delegate"`
}

type GmailDelegatesListCmd struct{}

func (c *GmailDelegatesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Delegates.List("me").Do()
	if err != nil {
		return err
	}
	rows := make([]gmailEmailStatusRow, 0, len(resp.Delegates))
	for _, d := range resp.Delegates {
		if d == nil {
			continue
		}
		rows = append(rows, gmailEmailStatusRow{
			Email:  d.DelegateEmail,
			Status: d.VerificationStatus,
		})
	}
	return writeGmailEmailStatusList(ctx, "delegates", resp.Delegates, "No delegates", rows)
}

type GmailDelegatesGetCmd struct {
	DelegateEmail string `arg:"" name:"delegateEmail" help:"Delegate email"`
}

func (c *GmailDelegatesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	delegateEmail := strings.TrimSpace(c.DelegateEmail)
	if delegateEmail == "" {
		return usage("empty delegateEmail")
	}
	delegate, err := svc.Users.Settings.Delegates.Get("me", delegateEmail).Do()
	if err != nil {
		return err
	}
	return writeGmailEmailStatusItem(ctx, "delegate", delegate, "delegate_email", gmailEmailStatusRow{
		Email:  delegate.DelegateEmail,
		Status: delegate.VerificationStatus,
	})
}

type GmailDelegatesAddCmd struct {
	DelegateEmail string `arg:"" name:"delegateEmail" help:"Delegate email"`
}

func (c *GmailDelegatesAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	delegateEmail := strings.TrimSpace(c.DelegateEmail)
	if delegateEmail == "" {
		return usage("empty delegateEmail")
	}

	if err := dryRunExit(ctx, flags, "gmail.delegates.add", map[string]any{
		"delegate_email": delegateEmail,
	}); err != nil {
		return err
	}
	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("add gmail delegate %s (grants mailbox read access)", delegateEmail)); confirmErr != nil {
		return confirmErr
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	delegate := &gmail.Delegate{
		DelegateEmail: delegateEmail,
	}

	created, err := svc.Users.Settings.Delegates.Create("me", delegate).Do()
	if err != nil {
		return err
	}
	return writeGmailEmailStatusCreateResult(
		ctx,
		"delegate",
		created,
		"delegate_email",
		gmailEmailStatusRow{Email: created.DelegateEmail, Status: created.VerificationStatus},
		"Delegate added successfully",
		"",
		"The delegate will receive an invitation email that they must accept.",
	)
}

type GmailDelegatesRemoveCmd struct {
	DelegateEmail string `arg:"" name:"delegateEmail" help:"Delegate email"`
}

func (c *GmailDelegatesRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	delegateEmail := strings.TrimSpace(c.DelegateEmail)
	if delegateEmail == "" {
		return usage("empty delegateEmail")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove gmail delegate %s", delegateEmail)); confirmErr != nil {
		return confirmErr
	}

	svc, err := loadGmailSettingsService(ctx, flags)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.Delegates.Delete("me", delegateEmail).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"success":       true,
			"delegateEmail": delegateEmail,
		})
	}

	ui.FromContext(ctx).Out().Printf("Delegate %s removed successfully", delegateEmail)
	return nil
}
