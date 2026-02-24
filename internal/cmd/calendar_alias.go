package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/steipete/gogcli/internal/config"
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
	aliases, err := config.ListCalendarAliases()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"aliases": aliases})
	}
	if len(aliases) == 0 {
		u.Err().Println("No calendar aliases")
		return nil
	}
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ALIAS\tCALENDAR_ID")
	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%s\n", k, aliases[k])
	}
	return nil
}

type CalendarAliasSetCmd struct {
	Alias      string `arg:"" name:"alias" help:"Alias name (no spaces)"`
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID (e.g., abc123@group.calendar.google.com)"`
}

func (c *CalendarAliasSetCmd) Run(ctx context.Context) error {
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
	if err := config.SetCalendarAlias(alias, calendarID); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"alias":      strings.ToLower(alias),
			"calendarId": calendarID,
		})
	}
	u.Out().Printf("alias\t%s", strings.ToLower(alias))
	u.Out().Printf("calendarId\t%s", calendarID)
	return nil
}

type CalendarAliasUnsetCmd struct {
	Alias string `arg:"" name:"alias" help:"Alias name"`
}

func (c *CalendarAliasUnsetCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	alias := strings.TrimSpace(c.Alias)
	if alias == "" {
		return usage("empty alias")
	}
	deleted, err := config.DeleteCalendarAlias(alias)
	if err != nil {
		return err
	}
	if !deleted {
		return usage("alias not found")
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"deleted": true,
			"alias":   strings.ToLower(alias),
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("alias\t%s", strings.ToLower(alias))
	return nil
}
