package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type TimeCmd struct {
	Now TimeNowCmd `cmd:"" name:"now" help:"Show current time"`
}

type TimeNowCmd struct {
	Timezone string `name:"timezone" help:"Timezone (e.g., America/New_York, UTC). Default: GOG_TIMEZONE, config, then local"`
}

func (c *TimeNowCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)
	loc, err := resolveOutputLocation(ctx, c.Timezone, false, stderrWriter(ctx))
	if err != nil {
		if strings.TrimSpace(c.Timezone) != "" {
			return usage(err.Error())
		}
		return err
	}
	tz := loc.String()

	now := time.Now().In(loc)
	formatted := now.Format("Monday, January 02, 2006 03:04 PM")
	offset := formatUTCOffset(now)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"timezone":     tz,
			"current_time": now.Format(time.RFC3339),
			"utc_offset":   offset,
			"formatted":    formatted,
		})
	}
	if u != nil {
		u.Out().Linef("timezone\t%s", tz)
		u.Out().Linef("current_time\t%s", now.Format(time.RFC3339))
		u.Out().Linef("utc_offset\t%s", offset)
		u.Out().Linef("formatted\t%s", formatted)
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
