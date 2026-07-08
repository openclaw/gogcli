package cmd

import (
	"context"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/timeparse"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarChangedCmd struct {
	CalendarID string   `arg:"" name:"calendarId" optional:"" help:"Calendar ID (default: primary)"`
	Cal        []string `name:"cal" help:"Calendar ID or name (can be repeated)"`
	Calendars  string   `name:"calendars" help:"Comma-separated calendar IDs, names, or indices from 'calendar calendars'"`
	Since      string   `name:"since" help:"Lower bound for last-modification time (RFC3339, date, or Go duration: 24h, 168h). Default: 720h (30 days)."`
	Max        int64    `name:"max" aliases:"limit" help:"Max results" default:"10"`
	All        bool     `name:"all" help:"Fetch from all calendars"`
	FailEmpty  bool     `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	Weekday    bool     `name:"weekday" help:"Include start/end day-of-week columns"`
	Location   bool     `name:"location" help:"Include event LOCATION column in table output"`
}

func (c *CalendarChangedCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	since, err := c.resolveSince()
	if err != nil {
		return err
	}

	_, svc, err := requireCalendarService(ctx, flags)
	if err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
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

	sinceRFC3339 := since.UTC().Format(time.RFC3339)

	switch {
	case c.All:
		cals, listErr := listCalendarList(ctx, svc)
		if listErr != nil {
			return listErr
		}
		ids := make([]string, 0, len(cals))
		for _, cal := range cals {
			if cal != nil && strings.TrimSpace(cal.Id) != "" {
				ids = append(ids, cal.Id)
			}
		}
		return c.listChangedMulti(ctx, svc, ids, sinceRFC3339, calendarTimezoneHints(cals))
	case len(calInputs) > 0:
		ids, resolveErr := resolveCalendarIDs(ctx, store, svc, calInputs)
		if resolveErr != nil {
			return resolveErr
		}
		if len(ids) == 0 {
			return usage("no calendars specified")
		}
		return c.listChangedMulti(ctx, svc, ids, sinceRFC3339, nil)
	default:
		calendarID, err = resolveCalendarSelector(ctx, store, svc, calendarID, true)
		if err != nil {
			return err
		}
		return c.listChangedSingle(ctx, svc, calendarID, sinceRFC3339)
	}
}

func (c *CalendarChangedCmd) resolveSince() (time.Time, error) {
	if strings.TrimSpace(c.Since) == "" {
		return time.Now().Add(-30 * 24 * time.Hour), nil
	}
	result, err := timeparse.ParseSince(c.Since, time.Now(), time.UTC)
	if err != nil {
		return time.Time{}, usagef("invalid --since value: %v", err)
	}
	return result.Time, nil
}

func (c *CalendarChangedCmd) listChangedSingle(ctx context.Context, svc *calendar.Service, calendarID, since string) error {
	calendarTimezone, loc := calendarDisplayTimezone(ctx, svc, calendarID, nil, nil)

	items, err := fetchChangedEvents(ctx, svc, calendarID, since)
	if err != nil {
		return err
	}

	events := make([]*eventWithCalendar, 0, len(items))
	for _, item := range items {
		redactCalendarEventForOutput(ctx, item)
		events = append(events, wrapEventWithCalendar(item, "", calendarTimezone, loc))
	}

	sortByUpdatedDesc(events)
	if int64(len(events)) > c.Max {
		events = events[:c.Max]
	}

	return c.writeOutput(ctx, events, since, false)
}

func (c *CalendarChangedCmd) listChangedMulti(ctx context.Context, svc *calendar.Service, calendarIDs []string, since string, hints map[string]calendarTimezoneHint) error {
	u := ui.FromContext(ctx)
	all := make([]*eventWithCalendar, 0)
	for _, calID := range calendarIDs {
		calID = strings.TrimSpace(calID)
		if calID == "" {
			continue
		}
		calendarTimezone, loc := calendarDisplayTimezone(ctx, svc, calID, hints, nil)
		items, err := fetchChangedEvents(ctx, svc, calID, since)
		if err != nil {
			u.Err().Linef("calendar %s: %v", calID, err)
			continue
		}
		for _, item := range items {
			redactCalendarEventForOutput(ctx, item)
			all = append(all, wrapEventWithCalendar(item, calID, calendarTimezone, loc))
		}
	}

	sortByUpdatedDesc(all)
	if int64(len(all)) > c.Max {
		all = all[:c.Max]
	}

	return c.writeOutput(ctx, all, since, true)
}

func fetchChangedEvents(ctx context.Context, svc *calendar.Service, calendarID, since string) ([]*calendar.Event, error) {
	fetch := func(pageToken string) ([]*calendar.Event, string, error) {
		// Calendar always returns entries deleted since updatedMin. Request them
		// explicitly too: deletions are changes and belong in this command's output.
		call := svc.Events.List(calendarID).
			UpdatedMin(since).
			ShowDeleted(true).
			OrderBy("updated").
			MaxResults(250).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Items, resp.NextPageToken, nil
	}
	items, err := collectAllPages("", fetch)
	return items, err
}

func sortByUpdatedDesc(events []*eventWithCalendar) {
	sort.SliceStable(events, func(i, j int) bool {
		a := calendarEvent(events[i]).Updated
		b := calendarEvent(events[j]).Updated
		return a > b
	})
}

func (c *CalendarChangedCmd) writeOutput(ctx context.Context, events []*eventWithCalendar, since string, includeCalendar bool) error {
	u := ui.FromContext(ctx)
	if outfmt.IsJSON(ctx) {
		jsonItems := make([]any, 0, len(events))
		for _, e := range events {
			jsonItems = append(jsonItems, e)
		}
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"events": jsonItems,
			"since":  since,
		}); err != nil {
			return err
		}
		if len(events) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(events) == 0 {
		u.Err().Println("No events")
		return failEmptyExit(c.FailEmpty)
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), compactCalendarRows(events), changedEventColumns(includeCalendar, c.Weekday, c.Location))
}

func changedEventColumns(includeCalendar, showWeekday, showLocation bool) []outfmt.Column[*eventWithCalendar] {
	columns := make([]outfmt.Column[*eventWithCalendar], 0, 8)
	columns = append(columns, outfmt.Column[*eventWithCalendar]{
		Header: "UPDATED",
		Value:  func(e *eventWithCalendar) string { return calendarEvent(e).Updated },
	})
	if includeCalendar {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "CALENDAR",
			Value:  func(e *eventWithCalendar) string { return e.CalendarID },
		})
	}
	columns = append(columns,
		outfmt.Column[*eventWithCalendar]{
			Header: "ID",
			Value:  func(e *eventWithCalendar) string { return calendarEvent(e).Id },
		},
		outfmt.Column[*eventWithCalendar]{
			Header: "START",
			Value:  eventDisplayStart,
		},
	)
	if showWeekday {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "START_DOW",
			Value: func(e *eventWithCalendar) string {
				startDay, _ := calendarEventWeekdays(e, includeCalendar)
				return startDay
			},
		})
	}
	columns = append(columns, outfmt.Column[*eventWithCalendar]{
		Header: "END",
		Value:  eventDisplayEnd,
	})
	if showWeekday {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "END_DOW",
			Value: func(e *eventWithCalendar) string {
				_, endDay := calendarEventWeekdays(e, includeCalendar)
				return endDay
			},
		})
	}
	columns = append(columns, outfmt.Column[*eventWithCalendar]{
		Header: "SUMMARY",
		Value:  func(e *eventWithCalendar) string { return calendarEvent(e).Summary },
	})
	if showLocation {
		columns = append(columns, outfmt.Column[*eventWithCalendar]{
			Header: "LOCATION",
			Value:  eventDisplayLocation,
		})
	}
	return columns
}
