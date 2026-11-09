package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type CalendarSearchCmd struct {
	Query string `arg:"" name:"query" help:"Search query"`
	TimeRangeFlags
	CalendarID string `name:"calendar" help:"Calendar ID" default:"primary"`
	Max        int64  `name:"max" aliases:"limit" help:"Max results" default:"25"`
}

func (c *CalendarSearchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	query := strings.TrimSpace(c.Query)
	if query == "" {
		return usage("search query cannot be empty")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireCalendarService(ctx, flags)
	if err != nil {
		return err
	}
	store, err := commandConfigStore(ctx)
	if err != nil {
		return err
	}
	calendarID, err := resolveCalendarSelector(ctx, store, svc, c.CalendarID, true)
	if err != nil {
		return err
	}

	timeRange, err := ResolveTimeRangeWithDefaults(ctx, svc, c.TimeRangeFlags, TimeRangeDefaults{
		FromOffset:            -30 * 24 * time.Hour,
		ToOffset:              90 * 24 * time.Hour,
		ToFromOffsetIfInvalid: 90 * 24 * time.Hour,
	})
	if err != nil {
		return err
	}
	from, to := timeRange.FormatRFC3339()

	items, more, err := listCalendarEventsUpTo(ctx, func() *calendar.EventsListCall {
		return svc.Events.List(calendarID).
			Q(query).
			TimeMin(from).
			TimeMax(to).
			SingleEvents(true).
			OrderBy("startTime")
	}, c.Max)
	if err != nil {
		return err
	}
	redactCalendarEventsForOutput(ctx, items)
	if more {
		u.Err().Linef("# More results: raise --max (currently %d) to see more matching events", c.Max)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"events": wrapEventsWithDays(items),
			"query":  query,
		})
	}

	if len(items) == 0 {
		u.Err().Println("No events found")
		return nil
	}

	tw := tabwriter.NewWriter(stdoutWriter(ctx), 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTART\tEND\tSUMMARY")
	for _, e := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Id, eventStart(e), eventEnd(e), e.Summary)
	}
	_ = tw.Flush()
	return nil
}
