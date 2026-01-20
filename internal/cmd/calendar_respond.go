package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	eventID := strings.TrimSpace(c.EventID)
	if calendarID == "" {
		return usage("empty calendarId")
	}
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

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	event, err := svc.Events.Get(calendarID, eventID).Do()
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

	updated, err := svc.Events.Patch(calendarID, eventID, event).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
		return outfmt.WriteJSON(os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(updated, tz, loc)})
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
