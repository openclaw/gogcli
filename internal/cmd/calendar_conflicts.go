package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type conflict struct {
	Start     string   `json:"start"`
	End       string   `json:"end"`
	Calendars []string `json:"calendars"`
}

type CalendarConflictsCmd struct {
	From      string `name:"from" help:"Start time (RFC3339, date, or relative: today, tomorrow, monday)"`
	To        string `name:"to" help:"End time (RFC3339, date, or relative)"`
	Today     bool   `name:"today" help:"Today only (timezone-aware)"`
	Week      bool   `name:"week" help:"This week (uses --week-start, default Mon)"`
	Days      int    `name:"days" help:"Next N days (timezone-aware)" default:"0"`
	WeekStart string `name:"week-start" help:"Week start day for --week (sun, mon, ...)" default:""`
	Calendars string `name:"calendars" help:"Comma-separated calendar IDs" default:"primary"`
}

func (c *CalendarConflictsCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	calendarIDs := splitCSV(c.Calendars)
	if len(calendarIDs) == 0 {
		return errors.New("no calendar IDs provided")
	}

	svc, err := newCalendarService(ctx, account)
	if err != nil {
		return err
	}

	// Use timezone-aware time resolution
	timeRange, err := ResolveTimeRange(ctx, svc, TimeRangeFlags{
		From:      c.From,
		To:        c.To,
		Today:     c.Today,
		Week:      c.Week,
		Days:      c.Days,
		WeekStart: c.WeekStart,
	})
	if err != nil {
		return err
	}

	from, to := timeRange.FormatRFC3339()

	items := make([]*calendar.FreeBusyRequestItem, 0, len(calendarIDs))
	for _, id := range calendarIDs {
		items = append(items, &calendar.FreeBusyRequestItem{Id: id})
	}

	resp, err := svc.Freebusy.Query(&calendar.FreeBusyRequest{
		TimeMin: from,
		TimeMax: to,
		Items:   items,
	}).Do()
	if err != nil {
		return err
	}

	conflicts := detectConflicts(resp.Calendars)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"conflicts": conflicts,
			"count":     len(conflicts),
		})
	}

	if len(conflicts) == 0 {
		u.Out().Println("No conflicts found")
		return nil
	}

	fmt.Printf("CONFLICTS FOUND: %d\n\n", len(conflicts))
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "START\tEND\tCALENDARS")
	for _, c := range conflicts {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Start, c.End, strings.Join(c.Calendars, ", "))
	}
	_ = tw.Flush()
	return nil
}

// detectConflicts finds overlapping busy periods across calendars
func detectConflicts(calendars map[string]calendar.FreeBusyCalendar) []conflict {
	if len(calendars) < 2 {
		return []conflict{}
	}

	type busyPeriod struct {
		start      time.Time
		end        time.Time
		calendarID string
	}

	var allBusy []busyPeriod
	for calID, cal := range calendars {
		for _, b := range cal.Busy {
			start, err := time.Parse(time.RFC3339, b.Start)
			if err != nil {
				continue
			}
			end, err := time.Parse(time.RFC3339, b.End)
			if err != nil {
				continue
			}
			allBusy = append(allBusy, busyPeriod{
				start:      start,
				end:        end,
				calendarID: calID,
			})
		}
	}

	var conflicts []conflict
	seen := make(map[string]bool)

	for i := 0; i < len(allBusy); i++ {
		for j := i + 1; j < len(allBusy); j++ {
			a := allBusy[i]
			b := allBusy[j]

			if a.calendarID == b.calendarID {
				continue
			}

			if a.start.Before(b.end) && a.end.After(b.start) {
				overlapStart := a.start
				if b.start.After(a.start) {
					overlapStart = b.start
				}
				overlapEnd := a.end
				if b.end.Before(a.end) {
					overlapEnd = b.end
				}

				calendarsInvolved := []string{a.calendarID, b.calendarID}
				if a.calendarID > b.calendarID {
					calendarsInvolved = []string{b.calendarID, a.calendarID}
				}
				// Stable key to avoid duplicates.
				key := fmt.Sprintf("%s|%s|%s", overlapStart.Format(time.RFC3339), overlapEnd.Format(time.RFC3339), strings.Join(calendarsInvolved, ","))

				if !seen[key] {
					seen[key] = true
					conflicts = append(conflicts, conflict{
						Start:     overlapStart.Format(time.RFC3339),
						End:       overlapEnd.Format(time.RFC3339),
						Calendars: calendarsInvolved,
					})
				}
			}
		}
	}

	return conflicts
}
