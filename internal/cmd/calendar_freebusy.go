package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarFreeBusyCmd struct {
	CalendarIDs string   `arg:"" optional:"" name:"calendarIds" help:"Comma-separated calendar IDs, names, or indices from 'calendar calendars'"`
	Cal         []string `name:"cal" help:"Calendar ID, name, or index (can be repeated)"`
	All         bool     `name:"all" help:"Query all calendars"`
	From        string   `name:"from" help:"Start time (RFC3339, required)"`
	To          string   `name:"to" help:"End time (RFC3339, required)"`
}

func (c *CalendarFreeBusyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.From) == "" || strings.TrimSpace(c.To) == "" {
		return usage("required: --from and --to")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	calendarIDs, err := resolveSelectedCalendarIDs(ctx, svc, c.Cal, c.CalendarIDs, c.All, true)
	if err != nil {
		return err
	}

	req := &calendar.FreeBusyRequest{
		TimeMin: c.From,
		TimeMax: c.To,
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"calendars": resp.Calendars})
	}

	if len(resp.Calendars) == 0 {
		u.Err().Println("No free/busy data")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "CALENDAR\tSTART\tEND")
	for id, data := range resp.Calendars {
		for _, b := range data.Busy {
			fmt.Fprintf(w, "%s\t%s\t%s\n", id, b.Start, b.End)
		}
	}
	return nil
}
