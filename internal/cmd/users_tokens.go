package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersTokensCmd struct {
	List   UsersTokensListCmd   `cmd:"" name:"list" aliases:"ls" help:"List user tokens"`
	Delete UsersTokensDeleteCmd `cmd:"" name:"delete" aliases:"rm,revoke" help:"Revoke a token"`
}

type UsersTokensListCmd struct {
	User string `arg:"" name:"user" help:"User email or ID"`
}

func (c *UsersTokensListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	tokens, err := svc.Tokens.List(c.User).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list tokens for %s: %w", c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, tokens)
	}

	if len(tokens.Items) == 0 {
		u.Err().Println("No tokens found")
		return nil
	}

	tw, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(tw, "CLIENT ID\tDISPLAY TEXT\tSCOPES\tANONYMOUS")
	for _, token := range tokens.Items {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%v\n",
			sanitizeTab(token.ClientId),
			sanitizeTab(token.DisplayText),
			len(token.Scopes),
			token.Anonymous,
		)
	}

	return nil
}

type UsersTokensDeleteCmd struct {
	User     string `arg:"" name:"user" help:"User email or ID"`
	ClientID string `arg:"" name:"client-id" help:"OAuth client ID to revoke"`
}

func (c *UsersTokensDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Tokens.Delete(c.User, c.ClientID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete token %s for %s: %w", c.ClientID, c.User, err)
	}

	u.Out().Printf("Revoked token %s for: %s\n", c.ClientID, c.User)
	return nil
}
