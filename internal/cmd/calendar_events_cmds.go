package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarEventsCmd struct {
	CalendarID        string   `arg:"" name:"calendarId" optional:"" help:"Calendar ID (default: primary)"`
	Cal               []string `name:"cal" help:"Calendar ID or name (can be repeated)"`
	Calendars         string   `name:"calendars" help:"Comma-separated calendar IDs, names, or indices from 'calendar calendars'"`
	From              string   `name:"from" help:"Start time (RFC3339 with timezone, date, or relative: today, tomorrow, monday)"`
	To                string   `name:"to" help:"End time (RFC3339 with timezone, date, or relative)"`
	Today             bool     `name:"today" help:"Today only (timezone-aware)"`
	Tomorrow          bool     `name:"tomorrow" help:"Tomorrow only (timezone-aware)"`
	Week              bool     `name:"week" help:"This week (uses --week-start, default Mon)"`
	Days              int      `name:"days" help:"Next N days (timezone-aware)" default:"0"`
	WeekStart         string   `name:"week-start" help:"Week start day for --week (sun, mon, ...)" default:""`
	Max               int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	Page              string   `name:"page" aliases:"cursor" help:"Page token"`
	AllPages          bool     `name:"all-pages" aliases:"allpages" help:"Fetch all pages"`
	FailEmpty         bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Query             string   `name:"query" help:"Free text search"`
	All               bool     `name:"all" help:"Fetch events from all calendars"`
	PrivatePropFilter string   `name:"private-prop-filter" help:"Filter by private extended property (key=value)"`
	SharedPropFilter  string   `name:"shared-prop-filter" help:"Filter by shared extended property (key=value)"`
	Fields            string   `name:"fields" help:"Comma-separated fields to return"`
	Weekday           bool     `name:"weekday" help:"Include start/end day-of-week columns" default:"${calendar_weekday}"`
}

func (c *CalendarEventsCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarID := strings.TrimSpace(c.CalendarID)
	calInputs := append([]string{}, c.Cal...)
	if strings.TrimSpace(c.Calendars) != "" {
		calInputs = append(calInputs, splitCSV(c.Calendars)...)
	}
	if c.All && (calendarID != "" || len(calInputs) > 0) {
		return usage("calendarId or --cal/--calendars not allowed with --all flag")
	}
	if calendarID != "" && len(calInputs) > 0 {
		return usage("calendarId not allowed with --cal/--calendars")
	}
	if !c.All && calendarID == "" && len(calInputs) == 0 {
		calendarID = primaryCalendarID
	}
	if calendarID != "" {
		calendarID, err = resolveCalendarAliasID(calendarID)
		if err != nil {
			return err
		}
	}
	for i, input := range calInputs {
		resolved, ok, resolveErr := config.ResolveCalendarAlias(input)
		if resolveErr != nil {
			return resolveErr
		}
		if ok {
			calInputs[i] = resolved
		}
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}
	if !c.All {
		calendarID, err = resolveCalendarID(ctx, svc, calendarID)
		if err != nil {
			return err
		}
	}

	timeRange, err := ResolveTimeRange(ctx, svc, TimeRangeFlags{
		From:      c.From,
		To:        c.To,
		Today:     c.Today,
		Tomorrow:  c.Tomorrow,
		Week:      c.Week,
		Days:      c.Days,
		WeekStart: c.WeekStart,
	})
	if err != nil {
		return err
	}

	from, to := timeRange.FormatRFC3339()

	if c.All {
		return listAllCalendarsEvents(ctx, svc, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
	}
	if len(calInputs) > 0 {
		ids, err := resolveCalendarIDs(ctx, svc, calInputs)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return usage("no calendars specified")
		}
		return listSelectedCalendarsEvents(ctx, svc, ids, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
	}
	return listCalendarEvents(ctx, svc, calendarID, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday)
}

type CalendarEventCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
}

func (c *CalendarEventCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	calendarID := strings.TrimSpace(c.CalendarID)
	eventID := normalizeCalendarEventID(c.EventID)
	if calendarID == "" {
		return usage("empty calendarId")
	}
	calendarID, err = resolveCalendarAliasID(calendarID)
	if err != nil {
		return err
	}
	if eventID == "" {
		return usage("empty eventId")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}
	calendarID, err = resolveCalendarID(ctx, svc, calendarID)
	if err != nil {
		return err
	}

	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return err
	}
	tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"event": wrapEventWithDaysWithTimezone(event, tz, loc)})
	}
	printCalendarEventWithTimezone(u, event, tz, loc)
	return nil
}
