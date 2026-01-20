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

type CalendarFocusTimeCmd struct {
	CalendarID     string   `arg:"" name:"calendarId" help:"Calendar ID (default: primary)" default:"primary"`
	Summary        string   `name:"summary" help:"Focus time title" default:"Focus Time"`
	From           string   `name:"from" required:"" help:"Start time (RFC3339)"`
	To             string   `name:"to" required:"" help:"End time (RFC3339)"`
	AutoDecline    string   `name:"auto-decline" help:"Auto-decline mode: none, all, new" default:"all"`
	DeclineMessage string   `name:"decline-message" help:"Message for declined invitations"`
	ChatStatus     string   `name:"chat-status" help:"Chat status: available, doNotDisturb" default:"doNotDisturb"`
	Recurrence     []string `name:"rrule" help:"Recurrence rules. Can be repeated."`
}

func (c *CalendarFocusTimeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	autoDeclineMode, err := validateAutoDeclineMode(c.AutoDecline)
	if err != nil {
		return err
	}

	chatStatus, err := validateChatStatus(c.ChatStatus)
	if err != nil {
		return err
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	event := &calendar.Event{
		Summary:      strings.TrimSpace(c.Summary),
		Start:        &calendar.EventDateTime{DateTime: strings.TrimSpace(c.From)},
		End:          &calendar.EventDateTime{DateTime: strings.TrimSpace(c.To)},
		EventType:    eventTypeFocusTime,
		Transparency: "opaque",
		FocusTimeProperties: &calendar.EventFocusTimeProperties{
			AutoDeclineMode: autoDeclineMode,
			DeclineMessage:  strings.TrimSpace(c.DeclineMessage),
			ChatStatus:      chatStatus,
		},
		Recurrence: buildRecurrence(c.Recurrence),
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

func validateAutoDeclineMode(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "none":
		return "declineNone", nil
	case "all":
		return "declineAllConflictingInvitations", nil
	case "new":
		return "declineOnlyNewConflictingInvitations", nil
	default:
		return "", fmt.Errorf("invalid auto-decline mode: %q (must be none, all, or new)", s)
	}
}

func validateChatStatus(s string) (string, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "available":
		return "available", nil
	case "donotdisturb", "dnd":
		return "doNotDisturb", nil
	default:
		return "", fmt.Errorf("invalid chat status: %q (must be available or doNotDisturb)", s)
	}
}
