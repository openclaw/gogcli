package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	admin "google.golang.org/api/admin/directory/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AliasesCmd struct {
	List   AliasesListCmd   `cmd:"" name:"list" aliases:"ls" help:"List aliases"`
	Create AliasesCreateCmd `cmd:"" name:"create" aliases:"add" help:"Create alias"`
	Delete AliasesDeleteCmd `cmd:"" name:"delete" aliases:"rm" help:"Delete alias"`
}

type AliasesListCmd struct {
	User  string `name:"user" help:"List aliases for a user"`
	Group string `name:"group" help:"List aliases for a group"`
}

func (c *AliasesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	user, group, err := resolveAliasTarget(c.User, c.Group)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	var resp *admin.Aliases
	if user != "" {
		resp, err = svc.Users.Aliases.List(user).Context(ctx).Do()
	} else {
		resp, err = svc.Groups.Aliases.List(group).Context(ctx).Do()
	}
	if err != nil {
		return fmt.Errorf("list aliases: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Aliases) == 0 {
		u.Err().Println("No aliases found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ALIAS\tTYPE\tTARGET")
	for _, raw := range resp.Aliases {
		alias := fmt.Sprintf("%v", raw)
		if user != "" {
			fmt.Fprintf(w, "%s\tuser\t%s\n", sanitizeTab(alias), sanitizeTab(user))
		} else {
			fmt.Fprintf(w, "%s\tgroup\t%s\n", sanitizeTab(alias), sanitizeTab(group))
		}
	}

	return nil
}

type AliasesCreateCmd struct {
	Alias string `arg:"" name:"alias" help:"Alias to create"`
	User  string `name:"user" help:"Create alias for a user"`
	Group string `name:"group" help:"Create alias for a group"`
}

func (c *AliasesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	user, group, err := resolveAliasTarget(c.User, c.Group)
	if err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	req := &admin.Alias{Alias: c.Alias}
	if user != "" {
		_, err = svc.Users.Aliases.Insert(user, req).Context(ctx).Do()
	} else {
		_, err = svc.Groups.Aliases.Insert(group, req).Context(ctx).Do()
	}
	if err != nil {
		return fmt.Errorf("create alias %s: %w", c.Alias, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"alias": c.Alias})
	}

	if user != "" {
		u.Out().Printf("Created alias %s for user %s\n", c.Alias, user)
	} else {
		u.Out().Printf("Created alias %s for group %s\n", c.Alias, group)
	}
	return nil
}

type AliasesDeleteCmd struct {
	Alias string `arg:"" name:"alias" help:"Alias to delete"`
	User  string `name:"user" help:"Delete alias for a user"`
	Group string `name:"group" help:"Delete alias for a group"`
}

func (c *AliasesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	user, group, err := resolveAliasTarget(c.User, c.Group)
	if err != nil {
		return err
	}

	if err = confirmDestructive(ctx, flags, fmt.Sprintf("delete alias %s", c.Alias)); err != nil {
		return err
	}

	svc, err := newAdminDirectory(ctx, account)
	if err != nil {
		return err
	}

	if user != "" {
		err = svc.Users.Aliases.Delete(user, c.Alias).Context(ctx).Do()
	} else {
		err = svc.Groups.Aliases.Delete(group, c.Alias).Context(ctx).Do()
	}
	if err != nil {
		return fmt.Errorf("delete alias %s: %w", c.Alias, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"alias": c.Alias})
	}

	if user != "" {
		u.Out().Printf("Deleted alias %s for user %s\n", c.Alias, user)
	} else {
		u.Out().Printf("Deleted alias %s for group %s\n", c.Alias, group)
	}
	return nil
}

func resolveAliasTarget(user, group string) (string, string, error) {
	user = strings.TrimSpace(user)
	group = strings.TrimSpace(group)
	if user == "" && group == "" {
		return "", "", usage("provide --user or --group")
	}
	if user != "" && group != "" {
		return "", "", usage("provide only one of --user or --group")
	}
	return user, group, nil
}
