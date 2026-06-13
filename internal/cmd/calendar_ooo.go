package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/calendar/v3"
)

type CalendarOOOCmd struct {
	CalendarID     string `arg:"" name:"calendarId" help:"Calendar ID (default: primary)" default:"primary"`
	Summary        string `name:"summary" help:"Out of office title" default:"Out of office"`
	From           string `name:"from" required:"" help:"Start datetime (RFC3339; date-only is not supported by Google Calendar API)"`
	To             string `name:"to" required:"" help:"End datetime (RFC3339; date-only is not supported by Google Calendar API)"`
	AutoDecline    string `name:"auto-decline" help:"Auto-decline mode: none, all, new" default:"all"`
	DeclineMessage string `name:"decline-message" help:"Message for declined invitations" default:"I am out of office and will respond when I return."`
	AllDay         bool   `name:"all-day" help:"Unsupported for out-of-office events; Google Calendar API rejects all-day OOO"`
}

func (c *CalendarOOOCmd) Run(ctx context.Context, flags *RootFlags) error {
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	calendarID, err := prepareCalendarID(store, c.CalendarID, true)
	if err != nil {
		return err
	}
	if c.AllDay {
		return usage("out-of-office events cannot be all-day; provide RFC3339 datetime --from/--to without --all-day")
	}
	if !strings.Contains(c.From, "T") || !strings.Contains(c.To, "T") {
		return usage("out-of-office requires RFC3339 datetime --from/--to; date-only out-of-office events are not supported by Google Calendar API")
	}
	autoDeclineMode, err := validateAutoDeclineMode(c.AutoDecline)
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

	if dryRunErr := dryRunExit(ctx, flags, "calendar.out-of-office", map[string]any{
		"calendar_id": calendarID,
		"event":       event,
	}); dryRunErr != nil {
		return dryRunErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
	if err != nil {
		return err
	}

	created, err := mutation.insertEvent(ctx, event, calendarInsertOptions{})
	if err != nil {
		return err
	}
	return mutation.writeEvent(ctx, created)
}
