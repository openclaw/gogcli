package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarRespondCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
	Status     string `name:"status" help:"Response status (accepted, declined, tentative, needsAction)"`
	Comment    string `name:"comment" help:"Optional comment/note to include with response"`
}

func (c *CalendarRespondCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	calendarID, err := prepareCalendarID(c.CalendarID, false)
	if err != nil {
		return err
	}
	eventID := normalizeCalendarEventID(c.EventID)
	if eventID == "" {
		return usage("empty eventId")
	}

	status := strings.TrimSpace(c.Status)
	if status == "" {
		return usage("required: --status")
	}
	validStatuses := []string{"accepted", "declined", "tentative", "needsAction"}
	isValid := false
	for _, v := range validStatuses {
		if status == v {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid status %q; must be one of: %s", status, strings.Join(validStatuses, ", "))
	}

	if dryRunErr := dryRunExit(ctx, flags, "calendar.respond", map[string]any{
		"calendar_id": calendarID,
		"event_id":    eventID,
		"status":      status,
		"comment":     strings.TrimSpace(c.Comment),
	}); dryRunErr != nil {
		return dryRunErr
	}

	mutation, err := newCalendarMutationContext(ctx, flags, calendarID)
	if err != nil {
		return err
	}

	event, err := mutation.svc.Events.Get(mutation.calendarID, eventID).Do()
	if err != nil {
		return err
	}

	if len(event.Attendees) == 0 {
		return errors.New("event has no attendees")
	}

	var selfAttendee *int
	for i, a := range event.Attendees {
		if a.Self {
			selfAttendee = &i
			break
		}
	}

	if selfAttendee == nil {
		return errors.New("you are not an attendee of this event")
	}

	if event.Attendees[*selfAttendee].Organizer {
		return errors.New("cannot respond to your own event (you are the organizer)")
	}

	event.Attendees[*selfAttendee].ResponseStatus = status
	if strings.TrimSpace(c.Comment) != "" {
		event.Attendees[*selfAttendee].Comment = strings.TrimSpace(c.Comment)
	}

	// Only patch the Attendees field to avoid issues with reminders validation
	patch := &calendar.Event{
		Attendees: event.Attendees,
	}
	updated, err := mutation.patchEvent(ctx, eventID, patch, "")
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return mutation.writeEvent(ctx, updated)
	}

	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("summary\t%s", orEmpty(updated.Summary, "(no title)"))
	u.Out().Printf("response_status\t%s", status)
	if strings.TrimSpace(c.Comment) != "" {
		u.Out().Printf("comment\t%s", strings.TrimSpace(c.Comment))
	}
	if updated.HtmlLink != "" {
		u.Out().Printf("link\t%s", updated.HtmlLink)
	}
	return nil
}
