package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TimeCmd struct {
	Now TimeNowCmd `cmd:"" name:"now" help:"Show current time"`
}

type TimeNowCmd struct {
	Timezone string `name:"timezone" help:"Timezone (e.g., America/New_York, UTC)"`
}

func (c *TimeNowCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	loc := time.Local
	tz := loc.String()
	if strings.TrimSpace(c.Timezone) != "" {
		var err error
		loc, err = time.LoadLocation(strings.TrimSpace(c.Timezone))
		if err != nil {
			return fmt.Errorf("invalid timezone %q: %w", c.Timezone, err)
		}
		tz = c.Timezone
	}

	now := time.Now().In(loc)
	formatted := now.Format("Monday, January 02, 2006 03:04 PM")
	offset := formatUTCOffset(now)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"timezone":     tz,
			"current_time": now.Format(time.RFC3339),
			"utc_offset":   offset,
			"formatted":    formatted,
		})
	}
	if u != nil {
		u.Out().Printf("timezone\t%s", tz)
		u.Out().Printf("current_time\t%s", now.Format(time.RFC3339))
		u.Out().Printf("utc_offset\t%s", offset)
		u.Out().Printf("formatted\t%s", formatted)
	}
	return nil
}

func formatUTCOffset(t time.Time) string {
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}
