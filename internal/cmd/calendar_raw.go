package cmd

import (
	"context"
	"net/url"
)

// CalendarRawCmd dumps the full Events.Get response as JSON, using the
// existing calendar-resolution helper so short names, "primary", and email
// aliases all work the same as they do for `calendar event`.
//
// REST reference: https://developers.google.com/calendar/api/v3/reference/events/get
// Go type: https://pkg.go.dev/google.golang.org/api/calendar/v3#Event
type CalendarRawCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID (e.g. 'primary', an email, or a calendar ID)"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
	Pretty     bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *CalendarRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	eventID := normalizeCalendarEventID(c.EventID)
	if eventID == "" {
		return usage("empty eventId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}

	calendarID, err := resolveCalendarSelector(ctx, store, svc, c.CalendarID, false)
	if err != nil {
		return err
	}

	client, err := calendarHTTPClient(ctx, account)
	if err != nil {
		return err
	}
	endpoint := "https://www.googleapis.com/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events/" + url.PathEscape(eventID)
	event, err := readRawObject(ctx, client, endpoint, nil, "event")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, event, c.Pretty)
}
