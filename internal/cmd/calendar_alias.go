package cmd

import (
	"context"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarAliasCmd struct {
	List  CalendarAliasListCmd  `cmd:"" name:"list" help:"List calendar aliases"`
	Set   CalendarAliasSetCmd   `cmd:"" name:"set" help:"Set a calendar alias"`
	Unset CalendarAliasUnsetCmd `cmd:"" name:"unset" help:"Remove a calendar alias"`
}

type CalendarAliasListCmd struct{}

func (c *CalendarAliasListCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	aliases, err := store.ListCalendarAliases()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"aliases": aliases})
	}
	if len(aliases) == 0 {
		u.Err().Println("No calendar aliases")
		return nil
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), calendarAliasRows(aliases), calendarAliasColumns())
}

type CalendarAliasSetCmd struct {
	Alias      string `arg:"" name:"alias" help:"Alias name (no spaces)"`
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID (e.g., abc123@group.calendar.google.com)"`
}

func (c *CalendarAliasSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	alias := strings.TrimSpace(c.Alias)
	if alias == "" {
		return usage("empty alias")
	}
	if strings.ContainsAny(alias, " \t\n") {
		return usage("alias must not contain whitespace")
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("empty calendar ID")
	}
	if dryRunErr := dryRunExit(ctx, flags, "calendar.alias.set", map[string]any{
		"alias":       strings.ToLower(alias),
		"calendar_id": calendarID,
	}); dryRunErr != nil {
		return dryRunErr
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	if err := store.SetCalendarAlias(alias, calendarID); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"alias":       strings.ToLower(alias),
			"calendar_id": calendarID,
		})
	}
	u.Out().Linef("alias\t%s", strings.ToLower(alias))
	u.Out().Linef("calendar_id\t%s", calendarID)
	return nil
}

type CalendarAliasUnsetCmd struct {
	Alias string `arg:"" name:"alias" help:"Alias name"`
}

func (c *CalendarAliasUnsetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	alias := strings.TrimSpace(c.Alias)
	if alias == "" {
		return usage("empty alias")
	}
	if dryRunErr := dryRunExit(ctx, flags, "calendar.alias.unset", map[string]any{
		"alias": strings.ToLower(alias),
	}); dryRunErr != nil {
		return dryRunErr
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	deleted, err := store.DeleteCalendarAlias(alias)
	if err != nil {
		return err
	}
	if !deleted {
		return usage("alias not found")
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"deleted": true,
			"alias":   strings.ToLower(alias),
		})
	}
	u.Out().Linef("deleted\ttrue")
	u.Out().Linef("alias\t%s", strings.ToLower(alias))
	return nil
}
