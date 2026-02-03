package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type UsersASPsCmd struct {
	List   UsersASPsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List app-specific passwords"`
	Delete UsersASPsDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete an app-specific password"`
}

type UsersASPsListCmd struct {
	User string `arg:"" name:"user" help:"User email or ID"`
}

func (c *UsersASPsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	asps, err := svc.Asps.List(c.User).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list ASPs for %s: %w", c.User, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, asps)
	}

	if len(asps.Items) == 0 {
		u.Err().Println("No app-specific passwords found")
		return nil
	}

	tw, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintln(tw, "CODE ID\tNAME\tCREATED\tLAST USED")
	for _, asp := range asps.Items {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
			asp.CodeId,
			sanitizeTab(asp.Name),
			formatUnixSeconds(asp.CreationTime),
			formatUnixSeconds(asp.LastTimeUsed),
		)
	}

	return nil
}

type UsersASPsDeleteCmd struct {
	User   string `arg:"" name:"user" help:"User email or ID"`
	CodeID int64  `arg:"" name:"code-id" help:"ASP code ID to delete"`
}

func (c *UsersASPsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if err := svc.Asps.Delete(c.User, c.CodeID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete ASP %d for %s: %w", c.CodeID, c.User, err)
	}

	u.Out().Printf("Deleted app-specific password %d for: %s\n", c.CodeID, c.User)
	return nil
}

func formatUnixSeconds(ts int64) string {
	if ts <= 0 {
		return "never"
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}
