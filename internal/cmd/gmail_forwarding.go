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

type GmailForwardingCmd struct {
	List   GmailForwardingListCmd   `cmd:"" name:"list" aliases:"ls" help:"List all forwarding addresses"`
	Get    GmailForwardingGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get a specific forwarding address"`
	Create GmailForwardingCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create/add a forwarding address"`
	Delete GmailForwardingDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a forwarding address"`
}

type GmailForwardingListCmd struct{}

func (c *GmailForwardingListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.ForwardingAddresses.List("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"forwardingAddresses": resp.ForwardingAddresses})
	}

	if len(resp.ForwardingAddresses) == 0 {
		u.Err().Println("No forwarding addresses")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "EMAIL\tSTATUS")
	for _, f := range resp.ForwardingAddresses {
		fmt.Fprintf(tw, "%s\t%s\n",
			f.ForwardingEmail,
			f.VerificationStatus)
	}
	_ = tw.Flush()
	return nil
}

type GmailForwardingGetCmd struct {
	ForwardingEmail string `arg:"" name:"forwardingEmail" help:"Forwarding email"`
}

func (c *GmailForwardingGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"forwardingAddress": address})
	}

	u.Out().Printf("forwarding_email\t%s", address.ForwardingEmail)
	u.Out().Printf("verification_status\t%s", address.VerificationStatus)
	return nil
}

type GmailForwardingCreateCmd struct {
	ForwardingEmail string `arg:"" name:"forwardingEmail" help:"Forwarding email"`
}

func (c *GmailForwardingCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	forwardingEmail := strings.TrimSpace(c.ForwardingEmail)
	if forwardingEmail == "" {
		return usage("empty forwardingEmail")
	}

	if err := dryRunExit(ctx, flags, "gmail.forwarding.create", map[string]any{
		"forwarding_email": forwardingEmail,
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

	address := &gmail.ForwardingAddress{
		ForwardingEmail: forwardingEmail,
	}

	created, err := svc.Users.Settings.ForwardingAddresses.Create("me", address).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"forwardingAddress": created})
	}

	u.Out().Println("Forwarding address created successfully")
	u.Out().Printf("forwarding_email\t%s", created.ForwardingEmail)
	u.Out().Printf("verification_status\t%s", created.VerificationStatus)
	u.Out().Println("\nA verification email has been sent to the forwarding address.")
	u.Out().Println("The address cannot be used until the recipient confirms the verification link.")
	return nil
}

type GmailForwardingDeleteCmd struct {
	ForwardingEmail string `arg:"" name:"forwardingEmail" help:"Forwarding email"`
}

func (c *GmailForwardingDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	forwardingEmail := strings.TrimSpace(c.ForwardingEmail)
	if forwardingEmail == "" {
		return usage("empty forwardingEmail")
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete gmail forwarding address %s", forwardingEmail)); confirmErr != nil {
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

	u.Out().Printf("Forwarding address %s deleted successfully", forwardingEmail)
	return nil
}
