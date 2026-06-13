package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthAliasCmd struct {
	List  AuthAliasListCmd  `cmd:"" name:"list" help:"List account aliases"`
	Set   AuthAliasSetCmd   `cmd:"" name:"set" help:"Set an account alias"`
	Unset AuthAliasUnsetCmd `cmd:"" name:"unset" help:"Remove an account alias"`
}

type AuthAliasListCmd struct{}

func (c *AuthAliasListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	aliases, err := store.ListAccountAliases()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"aliases": aliases})
	}
	if len(aliases) == 0 {
		u.Err().Println("No account aliases")
		return nil
	}
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ALIAS\tEMAIL")
	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%s\n", k, aliases[k])
	}
	return nil
}

type AuthAliasSetCmd struct {
	Alias string `arg:"" name:"alias" help:"Alias name (no spaces)"`
	Email string `arg:"" name:"email" help:"Account email"`
}

func (c *AuthAliasSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	alias := strings.TrimSpace(c.Alias)
	if alias == "" {
		return usage("empty alias")
	}
	if strings.Contains(alias, "@") {
		return usage("alias must not contain '@'")
	}
	if shouldAutoSelectAccount(alias) {
		return usage("alias name is reserved")
	}
	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}
	if err := dryRunExit(ctx, flags, "auth.alias.set", map[string]any{
		"alias": alias,
		"email": strings.ToLower(email),
	}); err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	if err := store.SetAccountAlias(alias, email); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"alias": alias,
			"email": strings.ToLower(email),
		})
	}
	u.Out().Linef("alias\t%s", alias)
	u.Out().Linef("email\t%s", strings.ToLower(email))
	return nil
}

type AuthAliasUnsetCmd struct {
	Alias string `arg:"" name:"alias" help:"Alias name"`
}

func (c *AuthAliasUnsetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	alias := strings.TrimSpace(c.Alias)
	if alias == "" {
		return usage("empty alias")
	}
	if err := dryRunExit(ctx, flags, "auth.alias.unset", map[string]any{
		"alias": alias,
	}); err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	deleted, err := store.DeleteAccountAlias(alias)
	if err != nil {
		return err
	}
	if !deleted {
		return usage("alias not found")
	}
	return writeResult(ctx, u,
		kv("deleted", true),
		kv("alias", alias),
	)
}
