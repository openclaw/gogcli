package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/calendar/v3"
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
	calendarID, err := prepareCalendarID(c.CalendarID, true)
	if err != nil {
		return err
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

	if dryRunErr := dryRunExit(ctx, flags, "calendar.out_of_office", map[string]any{
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
