package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type CalendarCreateCalendarCmd struct {
	Summary     string `arg:"" name:"summary" help:"Calendar display name"`
	Description string `name:"description" help:"Calendar description"`
	TimeZone    string `name:"timezone" aliases:"tz" help:"IANA timezone (e.g., America/New_York)"`
	Location    string `name:"location" help:"Calendar location"`
}

func (c *CalendarCreateCalendarCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	summary := strings.TrimSpace(c.Summary)
	if summary == "" {
		return usage("required: calendar name (positional argument)")
	}

	cal := &calendar.Calendar{
		Summary:     summary,
		Description: strings.TrimSpace(c.Description),
		TimeZone:    strings.TrimSpace(c.TimeZone),
		Location:    strings.TrimSpace(c.Location),
	}
	if cal.TimeZone != "" {
		if _, tzErr := loadTimezoneLocation(cal.TimeZone); tzErr != nil {
			return usagef("invalid timezone %q: %v", cal.TimeZone, tzErr)
		}
	}

	if dryRunErr := dryRunExit(ctx, flags, "calendar.create-calendar", map[string]any{
		"calendar": cal,
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

	created, err := svc.Calendars.Insert(cal).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create calendar: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"calendar": created})
	}

	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("summary\t%s", created.Summary)
	if created.TimeZone != "" {
		u.Out().Linef("timezone\t%s", created.TimeZone)
	}
	if created.Description != "" {
		u.Out().Linef("description\t%s", created.Description)
	}
	if created.Location != "" {
		u.Out().Linef("location\t%s", created.Location)
	}
	return nil
}
