package cmd

import (
	"context"
	"os"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarOOOCmd struct {
	CalendarID     string `arg:"" name:"calendarId" help:"Calendar ID (default: primary)" default:"primary"`
	Summary        string `name:"summary" help:"Out of office title" default:"Out of office"`
	From           string `name:"from" required:"" help:"Start date or datetime (RFC3339 or YYYY-MM-DD)"`
	To             string `name:"to" required:"" help:"End date or datetime (RFC3339 or YYYY-MM-DD)"`
	AutoDecline    string `name:"auto-decline" help:"Auto-decline mode: none, all, new" default:"all"`
	DeclineMessage string `name:"decline-message" help:"Message for declined invitations" default:"I am out of office and will respond when I return."`
	AllDay         bool   `name:"all-day" help:"Create as all-day event"`
}

func (c *CalendarOOOCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	autoDeclineMode, err := validateAutoDeclineMode(c.AutoDecline)
	if err != nil {
		return err
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	event := &calendar.Event{
		Summary:      strings.TrimSpace(c.Summary),
		Start:        buildEventDateTime(c.From, c.AllDay),
		End:          buildEventDateTime(c.To, c.AllDay),
		EventType:    eventTypeOutOfOffice,
		Transparency: "opaque",
		OutOfOfficeProperties: &calendar.EventOutOfOfficeProperties{
			AutoDeclineMode: autoDeclineMode,
			DeclineMessage:  strings.TrimSpace(c.DeclineMessage),
		},
	}

	created, err := svc.Events.Insert(c.CalendarID, event).Do()
	if err != nil {
		return err
	}

	tz, loc, _ := getCalendarLocation(ctx, svc, c.CalendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(created, tz, loc)})
	}
	printCalendarEventWithTimezone(u, created, tz, loc)
	return nil
}
