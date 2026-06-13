package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	proposeTimeAPILimitation   = "The Google Calendar API has no endpoint for proposing new meeting times. This is a known limitation since 2018."
	proposeTimeIssueTrackerURL = "https://issuetracker.google.com/issues/170465098"
	proposeTimeUpvoteAction    = "Open the issue tracker link above in a new browser tab and click the '+1' button to upvote. More votes = higher priority for Google to fix."
)

// CalendarProposeTimeCmd generates a browser URL for proposing a new meeting time.
// This is a workaround for a Google Calendar API limitation (since 2018).
type CalendarProposeTimeCmd struct {
	CalendarID  string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID     string `arg:"" name:"eventId" help:"Event ID"`
	Open        bool   `name:"open" help:"Open the URL in browser automatically"`
	Decline     bool   `name:"decline" help:"Also decline the event (notifies organizer)"`
	Comment     string `name:"comment" help:"Comment to include with decline (implies --decline)"`
	openBrowser func(context.Context, string) error
}

func (c *CalendarProposeTimeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	calendarID, err := prepareCalendarID(store, c.CalendarID, false)
	if err != nil {
		return err
	}
	eventID := normalizeCalendarEventID(c.EventID)
	if eventID == "" {
		return usage("empty eventId")
	}

	// Handle --comment implies --decline
	decline := c.Decline || strings.TrimSpace(c.Comment) != ""

	// Generate the propose-time URL
	// Format: base64(eventId + " " + calendarId)
	payload := eventID + " " + calendarID
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	proposeURL := "https://calendar.google.com/calendar/u/0/r/proposetime/" + encoded

	// Avoid touching auth/keyring and avoid mutating the event in dry-run mode.
	if dryRunErr := dryRunExit(ctx, flags, "calendar.propose-time", map[string]any{
		"calendar_id": calendarID,
		"event_id":    eventID,
		"propose_url": proposeURL,
		"open":        c.Open,
		"decline":     decline,
		"comment":     strings.TrimSpace(c.Comment),
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}

	calendarID, err = resolveCalendarID(ctx, svc, calendarID)
	if err != nil {
		return err
	}

	// Recompute URL in case the user provided a calendar name instead of an ID.
	payload = eventID + " " + calendarID
	encoded = base64.StdEncoding.EncodeToString([]byte(payload))
	proposeURL = "https://calendar.google.com/calendar/u/0/r/proposetime/" + encoded

	// Fetch event to display info and verify it exists
	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return fmt.Errorf("failed to get event: %w", err)
	}

	// If declining, update the event response
	if decline {
		if len(event.Attendees) == 0 {
			return usage("event has no attendees, cannot decline")
		}

		var selfIdx *int
		for i, a := range event.Attendees {
			if a.Self {
				selfIdx = &i
				break
			}
		}
		if selfIdx == nil {
			return usage("you are not an attendee of this event")
		}
		if event.Attendees[*selfIdx].Organizer {
			return usage("cannot decline your own event (you are the organizer)")
		}

		event.Attendees[*selfIdx].ResponseStatus = "declined"
		if strings.TrimSpace(c.Comment) != "" {
			event.Attendees[*selfIdx].Comment = strings.TrimSpace(c.Comment)
		}

		// Create a minimal patch with only attendees to avoid side effects
		patchEvent := &calendar.Event{
			Attendees: event.Attendees,
		}

		if _, err := svc.Events.Patch(calendarID, eventID, patchEvent).SendUpdates("all").Do(); err != nil {
			return fmt.Errorf("failed to decline event: %w", err)
		}
	}

	// JSON output
	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"event_id":          eventID,
			"calendar_id":       calendarID,
			"summary":           event.Summary,
			"propose_url":       proposeURL,
			"api_limitation":    proposeTimeAPILimitation,
			"issue_tracker_url": proposeTimeIssueTrackerURL,
			"upvote_action":     proposeTimeUpvoteAction,
		}
		if event.Start != nil {
			if event.Start.DateTime != "" {
				result["current_start"] = event.Start.DateTime
			} else {
				result["current_start"] = event.Start.Date
			}
		}
		if event.End != nil {
			if event.End.DateTime != "" {
				result["current_end"] = event.End.DateTime
			} else {
				result["current_end"] = event.End.Date
			}
		}
		if decline {
			result["declined"] = true
			if strings.TrimSpace(c.Comment) != "" {
				result["comment"] = strings.TrimSpace(c.Comment)
			}
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), result)
	}

	// Text output
	u.Out().Linef("# API Limitation: %s", proposeTimeAPILimitation)
	u.Out().Linef("# Issue tracker: %s", proposeTimeIssueTrackerURL)
	u.Out().Linef("# Action: %s", proposeTimeUpvoteAction)
	u.Out().Linef("")
	u.Out().Linef("event\t%s", orEmpty(event.Summary, "(no title)"))
	if event.Start != nil {
		start := event.Start.DateTime
		if start == "" {
			start = event.Start.Date
		}
		end := ""
		if event.End != nil {
			end = event.End.DateTime
			if end == "" {
				end = event.End.Date
			}
		}
		u.Out().Linef("current\t%s - %s", start, end)
	}
	u.Out().Linef("propose_url\t%s", proposeURL)

	if decline {
		u.Out().Linef("")
		u.Out().Linef("declined\tyes")
		if strings.TrimSpace(c.Comment) != "" {
			u.Out().Linef("comment\t%s", strings.TrimSpace(c.Comment))
		}
	} else {
		u.Out().Linef("")
		u.Out().Linef("Tip: To notify the organizer, decline with a comment:")
		u.Out().Linef("  gog calendar propose-time %s %s --decline --comment \"Can we do 5pm instead?\"", calendarID, eventID)
	}

	// Open browser if requested
	if c.Open {
		u.Out().Linef("")
		u.Out().Linef("Opening browser...")
		openBrowser := c.openBrowser
		if openBrowser == nil {
			openBrowser = openProposeTimeBrowser
		}
		if err := openBrowser(ctx, proposeURL); err != nil {
			u.Err().Linef("Failed to open browser: %v", err)
			u.Err().Linef("Please open the propose_url manually.")
		}
	}

	return nil
}

// openProposeTimeBrowser opens the URL in the default browser.
func openProposeTimeBrowser(ctx context.Context, url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url) //nolint:gosec // executable is fixed; arg is generated propose URL
	case literalWindows:
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // executable is fixed; arg is generated propose URL
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url) //nolint:gosec // executable is fixed; arg is generated propose URL
	}
	return cmd.Start()
}
