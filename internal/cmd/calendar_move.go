package cmd

import (
	"context"
	"strings"
)

type CalendarMoveCmd struct {
	CalendarID            string `arg:"" name:"calendarId" help:"Source calendar ID"`
	EventID               string `arg:"" name:"eventId" help:"Event ID"`
	DestinationCalendarID string `arg:"" name:"destinationCalendarId" help:"Destination calendar ID that becomes the event organizer"`
	SendUpdates           string `name:"send-updates" help:"Notification mode: all, externalOnly, none (default: none)"`
}

func (c *CalendarMoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	calendarID, err := prepareCalendarID(c.CalendarID, false)
	if err != nil {
		return err
	}
	eventID := normalizeCalendarEventID(c.EventID)
	if eventID == "" {
		return usage("empty eventId")
	}
	destinationCalendarID, err := prepareCalendarID(c.DestinationCalendarID, false)
	if err != nil {
		return err
	}
	if strings.EqualFold(calendarID, destinationCalendarID) {
		return usage("destination calendar must differ from source calendar")
	}
	sendUpdates, err := validateSendUpdates(c.SendUpdates)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "calendar.move", map[string]any{
		"calendar_id":             calendarID,
		"event_id":                eventID,
		"destination_calendar_id": destinationCalendarID,
		"send_updates":            sendUpdates,
	}); dryRunErr != nil {
		return dryRunErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
	if err != nil {
		return err
	}
	destinationCalendarID, err = resolveCalendarID(ctx, mutation.svc, destinationCalendarID)
	if err != nil {
		return err
	}
	if strings.EqualFold(mutation.calendarID, destinationCalendarID) {
		return usage("destination calendar must differ from source calendar")
	}

	moved, err := mutation.moveEvent(ctx, eventID, destinationCalendarID, sendUpdates)
	if err != nil {
		return err
	}
	mutation.calendarID = destinationCalendarID
	return mutation.writeEvent(ctx, moved)
}
