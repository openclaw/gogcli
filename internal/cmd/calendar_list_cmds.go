package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarCalendarsCmd struct {
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *CalendarCalendarsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*calendar.CalendarListEntry, string, error) {
		call := svc.CalendarList.List().MaxResults(c.Max)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		r, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return r.Items, r.NextPageToken, nil
	}

	items, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"calendars":     items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}
	if len(items) == 0 {
		u.Err().Println("No calendars")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactCalendarRows(items),
		calendarListColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type CalendarSubscribeCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID to subscribe to (e.g., user@example.com or calendar ID)"`
	ColorID    string `name:"color-id" help:"Color ID (1-24, see 'calendar colors')"`
	Hidden     bool   `name:"hidden" help:"Hide from the calendar list UI"`
	Selected   bool   `name:"selected" help:"Show events in the calendar UI" default:"true" negatable:""`
}

func (c *CalendarSubscribeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}
	colorID, err := validateCalendarColorId(c.ColorID)
	if err != nil {
		return err
	}

	entry := &calendar.CalendarListEntry{
		Id:       calendarID,
		Hidden:   c.Hidden,
		Selected: c.Selected,
	}
	if colorID != "" {
		entry.ColorId = colorID
	}

	if dryRunErr := dryRunExit(ctx, flags, "calendar.subscribe", map[string]any{
		"calendar_id": calendarID,
		"entry":       entry,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}

	added, err := svc.CalendarList.Insert(entry).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"calendar": added})
	}
	u.Out().Linef("subscribed\t%s", added.Id)
	u.Out().Linef("name\t%s", added.Summary)
	u.Out().Linef("role\t%s", added.AccessRole)
	return nil
}

type CalendarAclCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page       string `name:"page" aliases:"cursor" help:"Page token"`
	All        bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty  bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *CalendarAclCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	if calendarID == "" {
		return usage("calendarId required")
	}

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	calendarID, err = resolveCalendarSelector(ctx, store, svc, calendarID, false)
	if err != nil {
		return err
	}

	fetch := func(pageToken string) ([]*calendar.AclRule, string, error) {
		call := svc.Acl.List(calendarID).MaxResults(c.Max)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		r, callErr := call.Do()
		if callErr != nil {
			return nil, "", callErr
		}
		return r.Items, r.NextPageToken, nil
	}

	items, nextPageToken, err := loadPagedItems(c.Page, c.All, fetch)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"rules":         items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}
	if len(items) == 0 {
		u.Err().Println("No ACL rules")
		return failEmptyExit(c.FailEmpty)
	}

	if err := outfmt.WriteTable(
		ctx,
		stdoutWriter(ctx),
		compactCalendarRows(items),
		calendarACLColumns(),
	); err != nil {
		return err
	}
	printNextPageHint(u, nextPageToken)
	return nil
}
