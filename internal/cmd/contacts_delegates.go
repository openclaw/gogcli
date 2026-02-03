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

// ContactsDelegatesCmd manages delegate access using Gmail delegation APIs.
type ContactsDelegatesCmd struct {
	List   ContactsDelegatesListCmd   `cmd:"" name:"list" help:"List delegates (Gmail delegation)"`
	Add    ContactsDelegatesAddCmd    `cmd:"" name:"add" help:"Add a delegate (Gmail delegation)"`
	Remove ContactsDelegatesRemoveCmd `cmd:"" name:"remove" help:"Remove a delegate (Gmail delegation)"`
}

type ContactsDelegatesListCmd struct {
	User string `name:"user" help:"User email to list delegates for"`
}

func (c *ContactsDelegatesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Delegates.List("me").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list delegates: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"delegates": resp.Delegates})
	}

	if len(resp.Delegates) == 0 {
		u.Err().Println("No delegates")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "EMAIL\tSTATUS")
	for _, d := range resp.Delegates {
		fmt.Fprintf(tw, "%s\t%s\n", d.DelegateEmail, d.VerificationStatus)
	}
	_ = tw.Flush()
	return nil
}

type ContactsDelegatesAddCmd struct {
	User     string `name:"user" help:"User email to add delegate for"`
	Delegate string `name:"delegate" help:"Delegate email" required:""`
}

func (c *ContactsDelegatesAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	delegate := strings.TrimSpace(c.Delegate)
	if delegate == "" {
		return usage("--delegate is required")
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.Delegates.Create("me", &gmail.Delegate{DelegateEmail: delegate}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("add delegate: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	u.Out().Printf("Added delegate: %s\n", resp.DelegateEmail)
	return nil
}

type ContactsDelegatesRemoveCmd struct {
	User     string `name:"user" help:"User email to remove delegate for"`
	Delegate string `name:"delegate" help:"Delegate email" required:""`
}

func (c *ContactsDelegatesRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	delegate := strings.TrimSpace(c.Delegate)
	if delegate == "" {
		return usage("--delegate is required")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove delegate %s", delegate)); err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Users.Settings.Delegates.Delete("me", delegate).Context(ctx).Do(); err != nil {
		return fmt.Errorf("remove delegate: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"delegate": delegate, "removed": true})
	}

	u.Out().Printf("Removed delegate: %s\n", delegate)
	return nil
}
