package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

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
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Delegates.List("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"delegates": resp.Delegates})
	}

	if len(resp.Delegates) == 0 {
		u.Err().Println("No delegates")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "EMAIL\tSTATUS")
	for _, d := range resp.Delegates {
		fmt.Fprintf(tw, "%s\t%s\n",
			d.DelegateEmail,
			d.VerificationStatus)
	}
	_ = tw.Flush()
	return nil
}

type GmailDelegatesGetCmd struct {
	DelegateEmail string `arg:"" name:"delegateEmail" help:"Delegate email"`
}

func (c *GmailDelegatesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"delegate": delegate})
	}

	u.Out().Printf("delegate_email\t%s", delegate.DelegateEmail)
	u.Out().Printf("verification_status\t%s", delegate.VerificationStatus)
	return nil
}

type GmailDelegatesAddCmd struct {
	DelegateEmail string `arg:"" name:"delegateEmail" help:"Delegate email"`
}

func (c *GmailDelegatesAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	delegateEmail := strings.TrimSpace(c.DelegateEmail)
	if delegateEmail == "" {
		return usage("empty delegateEmail")
	}

	if err := dryRunExit(ctx, flags, "gmail.delegates.add", map[string]any{
		"delegate_email": delegateEmail,
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

	delegate := &gmail.Delegate{
		DelegateEmail: delegateEmail,
	}

	created, err := svc.Users.Settings.Delegates.Create("me", delegate).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"delegate": created})
	}

	u.Out().Println("Delegate added successfully")
	u.Out().Printf("delegate_email\t%s", created.DelegateEmail)
	u.Out().Printf("verification_status\t%s", created.VerificationStatus)
	u.Out().Println("\nThe delegate will receive an invitation email that they must accept.")
	return nil
}

type GmailDelegatesRemoveCmd struct {
	DelegateEmail string `arg:"" name:"delegateEmail" help:"Delegate email"`
}

func (c *GmailDelegatesRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	delegateEmail := strings.TrimSpace(c.DelegateEmail)
	if delegateEmail == "" {
		return usage("empty delegateEmail")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove gmail delegate %s", delegateEmail)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
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

	u.Out().Printf("Delegate %s removed successfully", delegateEmail)
	return nil
}
