package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	proposeTimeErrorMessage    = "Google Calendar API does not support proposing new times programmatically (since 2018)."
	proposeTimeIssueTrackerURL = "https://issuetracker.google.com/issues/170465098"
)

// CalendarProposeTimeCmd generates a browser URL for proposing a new meeting time.
// This is a workaround for a Google Calendar API limitation (since 2018).
type CalendarProposeTimeCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
	Open       bool   `name:"open" help:"Open the URL in browser automatically"`
	Decline    bool   `name:"decline" help:"Also decline the event (notifies organizer)"`
	Comment    string `name:"comment" help:"Comment to include with decline (implies --decline)"`
}

func (c *CalendarProposeTimeCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// Fetch event to display info and verify it exists
	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return fmt.Errorf("failed to get event: %w", err)
	}

	// Generate the propose-time URL
	// Format: base64(eventId + " " + calendarId)
	payload := eventID + " " + calendarID
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	proposeURL := "https://calendar.google.com/calendar/u/0/r/proposetime/" + encoded

	// Handle --comment implies --decline
	decline := c.Decline || strings.TrimSpace(c.Comment) != ""

	// If declining, update the event response
	if decline {
		if len(event.Attendees) == 0 {
			return fmt.Errorf("event has no attendees, cannot decline")
		}

		var selfIdx *int
		for i, a := range event.Attendees {
			if a.Self {
				selfIdx = &i
				break
			}
		}
		if selfIdx == nil {
			return fmt.Errorf("you are not an attendee of this event")
		}
		if event.Attendees[*selfIdx].Organizer {
			return fmt.Errorf("cannot decline your own event (you are the organizer)")
		}

		event.Attendees[*selfIdx].ResponseStatus = "declined"
		if strings.TrimSpace(c.Comment) != "" {
			event.Attendees[*selfIdx].Comment = strings.TrimSpace(c.Comment)
		}

		if _, err := svc.Events.Patch(calendarID, eventID, event).Do(); err != nil {
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
			"limitation":        proposeTimeErrorMessage,
			"issue_tracker_url": proposeTimeIssueTrackerURL,
			"error_message":     proposeTimeErrorMessage,
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
		return outfmt.WriteJSON(os.Stdout, result)
	}

	// Text output
	u.Out().Printf("# Error: %s\n", proposeTimeErrorMessage)
	u.Out().Printf("# Issue tracker: %s\n", proposeTimeIssueTrackerURL)
	u.Out().Println("# \"Propose new time\" is only available via browser")
	u.Out().Println("")
	u.Out().Printf("event\t%s\n", orEmpty(event.Summary, "(no title)"))
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
		u.Out().Printf("current\t%s - %s\n", start, end)
	}
	u.Out().Printf("propose_url\t%s\n", proposeURL)

	if decline {
		u.Out().Println("")
		u.Out().Println("declined\tyes")
		if strings.TrimSpace(c.Comment) != "" {
			u.Out().Printf("comment\t%s\n", strings.TrimSpace(c.Comment))
		}
	} else {
		u.Out().Println("")
		u.Out().Println("Tip: To notify the organizer, decline with a comment:")
		u.Out().Printf("  gog calendar propose-time %s %s --decline --comment \"Can we do 5pm instead?\"\n", calendarID, eventID)
	}

	// Open browser if requested
	if c.Open {
		u.Out().Println("")
		u.Out().Println("Opening browser...")
		if err := openProposeTimeBrowser(proposeURL); err != nil {
			u.Err().Printf("Failed to open browser: %v\n", err)
			u.Err().Println("Please open the propose_url manually.")
		}
	}

	return nil
}

// openProposeTimeBrowser opens the URL in the default browser.
var openProposeTimeBrowser = func(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
