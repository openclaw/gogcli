package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarTimeCmd struct {
	CalendarID string `name:"calendar" help:"Calendar ID to get timezone from" default:"primary"`
	Timezone   string `name:"timezone" help:"Override timezone (e.g., America/New_York, UTC)"`
}

func (c *CalendarTimeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	var tz string
	var loc *time.Location

	// Check for explicitly configured timezone (flag, env, or config)
	loc, err = getConfiguredTimezone(c.Timezone)
	if err != nil {
		return err
	}

	if loc != nil {
		// Timezone was explicitly configured
		tz = loc.String()
	} else {
		// Fall back to Google Calendar's timezone
		svc, err := newCalendarService(ctx, account)
		if err != nil {
			return err
		}

		cal, err := svc.CalendarList.Get(c.CalendarID).Do()
		if err != nil {
			return fmt.Errorf("failed to get calendar %q: %w", c.CalendarID, err)
		}

		tz = cal.TimeZone
		if tz == "" {
			return fmt.Errorf("calendar %q has no timezone set", c.CalendarID)
		}

		loc, err = time.LoadLocation(tz)
		if err != nil {
			return fmt.Errorf("invalid calendar timezone %q: %w", tz, err)
		}
	}

	now := time.Now().In(loc)
	formatted := now.Format("Monday, January 02, 2006 03:04 PM")

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"timezone":     tz,
			"current_time": now.Format(time.RFC3339),
			"formatted":    formatted,
		})
	}

	u.Out().Printf("timezone\t%s", tz)
	u.Out().Printf("current_time\t%s", now.Format(time.RFC3339))
	u.Out().Printf("formatted\t%s", formatted)
	return nil
}
