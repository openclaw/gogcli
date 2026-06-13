package cmd

import (
	"context"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarEventsCmd struct {
	CalendarID        []string `arg:"" name:"calendarId" optional:"" help:"Calendar ID (default: primary); optional leading list/ls selector is accepted for compatibility"`
	Cal               []string `name:"cal" help:"Calendar ID or name (can be repeated)"`
	Calendars         string   `name:"calendars" help:"Comma-separated calendar IDs, names, or indices from 'calendar calendars'"`
	From              string   `name:"from" help:"Start time (RFC3339 with timezone, date, or relative: now, today, tomorrow, monday)"`
	To                string   `name:"to" help:"End time (RFC3339 with timezone, date, or relative: now, today, tomorrow, monday)"`
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
	Location          bool     `name:"location" help:"Include event LOCATION column in table output"`
	Sort              string   `name:"sort" help:"Sort events by start|end|summary|calendar (default: keep API order; with --all, start is recommended for chronological output)" enum:"start,end,summary,calendar," default:""`
	Order             string   `name:"order" help:"Sort order" enum:"asc,desc" default:"asc"`
}

func (c *CalendarEventsCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}

	calendarID, err := normalizeCalendarEventsArgs(c.CalendarID)
	if err != nil {
		return err
	}
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

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}
	if !c.All && len(calInputs) == 0 {
		calendarID, err = resolveCalendarSelector(ctx, store, svc, calendarID, true)
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
		return listAllCalendarsEvents(ctx, svc, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday, c.Location, c.Sort, c.Order)
	}
	if len(calInputs) > 0 {
		ids, err := resolveCalendarIDs(ctx, store, svc, calInputs)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return usage("no calendars specified")
		}
		return listSelectedCalendarsEvents(ctx, svc, ids, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday, c.Location, c.Sort, c.Order)
	}
	return listCalendarEvents(ctx, svc, calendarID, from, to, c.Max, c.Page, c.AllPages, c.FailEmpty, c.Query, c.PrivatePropFilter, c.SharedPropFilter, c.Fields, c.Weekday, c.Location, c.Sort, c.Order)
}

func normalizeCalendarEventsArgs(args []string) (string, error) {
	trimmed := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			trimmed = append(trimmed, arg)
		}
	}
	if len(trimmed) > 0 && (trimmed[0] == strList || trimmed[0] == "ls") {
		trimmed = trimmed[1:]
	}
	if len(trimmed) > 1 {
		return "", usage("calendar events accepts at most one calendarId")
	}
	if len(trimmed) == 0 {
		return "", nil
	}
	return trimmed[0], nil
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
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	eventID := normalizeCalendarEventID(c.EventID)
	if eventID == "" {
		return usage("empty eventId")
	}

	svc, err := calendarService(ctx, account)
	if err != nil {
		return err
	}
	calendarID, err := resolveCalendarSelector(ctx, store, svc, c.CalendarID, false)
	if err != nil {
		return err
	}

	event, err := svc.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return err
	}
	redactCalendarEventForOutput(ctx, event)
	tz, loc, _ := getCalendarLocation(ctx, svc, calendarID)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"event": wrapEventWithDaysWithTimezone(event, tz, loc)})
	}
	printCalendarEventWithTimezone(u, event, tz, loc)
	return nil
}
