package cmd

import (
	"context"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarFreeBusyCmd struct {
	CalendarIDs string   `arg:"" optional:"" name:"calendarIds" help:"Comma-separated calendar IDs, names, or indices from 'calendar calendars'"`
	Cal         []string `name:"cal" help:"Calendar ID, name, or index (can be repeated)"`
	All         bool     `name:"all" help:"Query all calendars"`
	From        string   `name:"from" help:"Start time (RFC3339 with timezone, date, or relative: now, today, tomorrow, monday)"`
	To          string   `name:"to" help:"End time (RFC3339 with timezone, date, or relative: now, today, tomorrow, monday)"`
}

func (c *CalendarFreeBusyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if strings.TrimSpace(c.From) == "" || strings.TrimSpace(c.To) == "" {
		return usage("required: --from and --to")
	}

	_, svc, err := requireCalendarService(ctx, flags)
	if err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}

	calendarIDs, err := resolveSelectedCalendarIDs(ctx, store, svc, c.Cal, c.CalendarIDs, c.All, true)
	if err != nil {
		return err
	}

	from, to, err := c.resolveFreeBusyRange(ctx, svc)
	if err != nil {
		return err
	}

	req := &calendar.FreeBusyRequest{
		TimeMin: from,
		TimeMax: to,
		Items:   make([]*calendar.FreeBusyRequestItem, 0, len(calendarIDs)),
	}
	for _, id := range calendarIDs {
		req.Items = append(req.Items, &calendar.FreeBusyRequestItem{Id: id})
	}

	resp, err := svc.Freebusy.Query(req).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"calendars": resp.Calendars})
	}

	if len(resp.Calendars) == 0 {
		u.Err().Println("No free/busy data")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), calendarFreeBusyRows(resp.Calendars), calendarFreeBusyColumns())
}

func (c *CalendarFreeBusyCmd) resolveFreeBusyRange(ctx context.Context, svc *calendar.Service) (string, string, error) {
	if isRFC3339Instant(c.From) && isRFC3339Instant(c.To) {
		return c.From, c.To, nil
	}

	timeRange, err := ResolveTimeRange(ctx, svc, TimeRangeFlags{
		From: c.From,
		To:   c.To,
	})
	if err != nil {
		return "", "", err
	}
	from, to := timeRange.FormatRFC3339()
	return from, to, nil
}

func isRFC3339Instant(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	return err == nil
}
