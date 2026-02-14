package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type CalendarTeamCmd struct {
	GroupEmail string `arg:"" help:"Google Group email (e.g., engineering@company.com)"`
	FreeBusy   bool   `name:"freebusy" help:"Show only busy/free blocks (faster, single API call)"`
	Query      string `name:"query" short:"q" help:"Filter events by title (case-insensitive)"`
	Max        int64  `name:"max" aliases:"limit" help:"Max events per calendar" default:"100"`
	NoDedup    bool   `name:"no-dedup" help:"Show each person's view without deduplication"`
	TimeRangeFlags
}

// teamEvent represents a calendar event with owner information.
type teamEvent struct {
	Who            string `json:"who"`
	ID             string `json:"id"`
	Start          string `json:"start"`
	End            string `json:"end"`
	Summary        string `json:"summary"`
	Status         string `json:"status,omitempty"`
	StartDayOfWeek string `json:"startDayOfWeek,omitempty"`
	EndDayOfWeek   string `json:"endDayOfWeek,omitempty"`
	dedupeKey      string
	sortKey        time.Time
}

func (c *CalendarTeamCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	groupEmail := strings.TrimSpace(c.GroupEmail)
	if groupEmail == "" {
		return usage("group email required")
	}

	// Get calendar service first (for timezone resolution)
	calSvc, err := newCalendarService(ctx, account)
	if err != nil {
		return fmt.Errorf("calendar service: %w", err)
	}

	// Resolve time range (timezone-aware)
	tr, err := ResolveTimeRange(ctx, calSvc, c.TimeRangeFlags)
	if err != nil {
		return err
	}

	// Get group members via Cloud Identity API
	cloudSvc, err := newCloudIdentityService(ctx, account)
	if err != nil {
		return wrapCloudIdentityError(err, account)
	}

	memberEmails, err := collectGroupMemberEmails(ctx, cloudSvc, groupEmail)
	if err != nil {
		return fmt.Errorf("failed to list group members: %w", err)
	}

	if len(memberEmails) == 0 {
		u.Err().Printf("No user members in group %s", groupEmail)
		return nil
	}

	if c.FreeBusy {
		return c.runFreeBusy(ctx, calSvc, memberEmails, tr)
	}
	return c.runEvents(ctx, calSvc, u, memberEmails, tr)
}

func (c *CalendarTeamCmd) runFreeBusy(ctx context.Context, svc *calendar.Service, emails []string, tr *TimeRange) error {
	// Build FreeBusy request
	items := make([]*calendar.FreeBusyRequestItem, len(emails))
	for i, email := range emails {
		items[i] = &calendar.FreeBusyRequestItem{Id: email}
	}

	resp, err := svc.Freebusy.Query(&calendar.FreeBusyRequest{
		TimeMin: tr.From.Format(time.RFC3339),
		TimeMax: tr.To.Format(time.RFC3339),
		Items:   items,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("freebusy query: %w", err)
	}

	type busyResult struct {
		Email  string   `json:"email"`
		Busy   []string `json:"busy"`
		Errors []string `json:"errors,omitempty"`
	}

	results := make([]busyResult, 0, len(emails))
	for _, email := range emails {
		cal, ok := resp.Calendars[email]
		if !ok {
			continue
		}

		result := busyResult{Email: email}

		// Check for errors
		if len(cal.Errors) > 0 {
			for _, e := range cal.Errors {
				result.Errors = append(result.Errors, e.Reason)
			}
		}

		// Format busy blocks
		for _, busy := range cal.Busy {
			start, _ := time.Parse(time.RFC3339, busy.Start)
			end, _ := time.Parse(time.RFC3339, busy.End)
			result.Busy = append(result.Busy, fmt.Sprintf("%s-%s",
				start.In(tr.Location).Format("15:04"),
				end.In(tr.Location).Format("15:04"),
			))
		}

		results = append(results, result)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"group":    c.GroupEmail,
			"timeMin":  tr.From.Format(time.RFC3339),
			"timeMax":  tr.To.Format(time.RFC3339),
			"timezone": tr.Location.String(),
			"freebusy": results,
		})
	}

	// Text output
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "WHO\tBUSY BLOCKS")
	for _, r := range results {
		busyStr := strings.Join(r.Busy, ", ")
		if busyStr == "" {
			busyStr = "(free)"
		}
		if len(r.Errors) > 0 {
			busyStr = "error: " + strings.Join(r.Errors, ", ")
		}
		fmt.Fprintf(w, "%s\t%s\n", sanitizeTab(r.Email), sanitizeTab(busyStr))
	}
	return nil
}

func (c *CalendarTeamCmd) runEvents(ctx context.Context, svc *calendar.Service, u *ui.UI, emails []string, tr *TimeRange) error {
	var (
		mu     sync.Mutex
		events []teamEvent
		errors []string
		wg     sync.WaitGroup
		sem    = make(chan struct{}, 10) // max 10 concurrent requests
	)

	queryLower := strings.ToLower(c.Query)

	for _, email := range emails {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			call := svc.Events.List(email).
				SingleEvents(true).
				TimeMin(tr.From.Format(time.RFC3339)).
				TimeMax(tr.To.Format(time.RFC3339)).
				MaxResults(c.Max).
				OrderBy("startTime").
				Context(ctx)

			resp, err := call.Do()
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Sprintf("%s: %v", email, err))
				mu.Unlock()
				return
			}

			for _, ev := range resp.Items {
				if ev == nil {
					continue
				}

				// Skip declined events
				declined := false
				for _, att := range ev.Attendees {
					if att.Self && att.ResponseStatus == "declined" {
						declined = true
						break
					}
				}
				if declined {
					continue
				}

				summary := ev.Summary
				// Hide private events
				if ev.Visibility == "private" || ev.Visibility == "confidential" {
					summary = "(busy)"
				}

				// Apply query filter
				if queryLower != "" && !strings.Contains(strings.ToLower(summary), queryLower) {
					continue
				}

				start, end := formatEventTime(ev, tr.Location)
				startDay, endDay := eventDaysOfWeek(ev)
				startTime := parseEventStart(ev, tr.Location)
				dedupeKey := eventDedupeKey(ev, startTime)

				mu.Lock()
				events = append(events, teamEvent{
					Who:            email,
					ID:             ev.Id,
					Start:          start,
					End:            end,
					Summary:        summary,
					Status:         ev.Status,
					StartDayOfWeek: startDay,
					EndDayOfWeek:   endDay,
					dedupeKey:      dedupeKey,
					sortKey:        startTime,
				})
				mu.Unlock()
			}
		}(email)
	}

	wg.Wait()

	// Print warnings for errors
	for _, e := range errors {
		u.Err().Printf("Warning: %s", e)
	}

	// Sort by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].sortKey.Before(events[j].sortKey)
	})

	// Deduplicate by event ID unless --no-dedup
	if !c.NoDedup {
		events = dedupeTeamEvents(events)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"group":    c.GroupEmail,
			"timeMin":  tr.From.Format(time.RFC3339),
			"timeMax":  tr.To.Format(time.RFC3339),
			"timezone": tr.Location.String(),
			"events":   events,
		})
	}

	if len(events) == 0 {
		u.Err().Println("No events found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "WHO\tSTART\tEND\tSUMMARY")
	for _, ev := range events {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(ev.Who),
			sanitizeTab(ev.Start),
			sanitizeTab(ev.End),
			sanitizeTab(truncate(ev.Summary, 40)),
		)
	}
	return nil
}

func formatEventTime(ev *calendar.Event, loc *time.Location) (start, end string) {
	if ev.Start == nil {
		return "", ""
	}

	// All-day event
	if ev.Start.Date != "" {
		start = ev.Start.Date
		if ev.End != nil && ev.End.Date != "" {
			end = ev.End.Date
		}
		return
	}

	// Timed event
	if ev.Start.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, ev.Start.DateTime); err == nil {
			start = t.In(loc).Format("15:04")
		}
	}
	if ev.End != nil && ev.End.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, ev.End.DateTime); err == nil {
			end = t.In(loc).Format("15:04")
		}
	}
	return
}

func parseEventStart(ev *calendar.Event, loc *time.Location) time.Time {
	if ev.Start == nil {
		return time.Time{}
	}
	if ev.Start.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, ev.Start.DateTime); err == nil {
			return t
		}
	}
	if ev.Start.Date != "" {
		if t, err := time.ParseInLocation("2006-01-02", ev.Start.Date, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

func dedupeTeamEvents(events []teamEvent) []teamEvent {
	seen := make(map[string]int) // event ID -> index in result
	var result []teamEvent

	for _, ev := range events {
		key := ev.dedupeKey
		if key == "" {
			key = ev.ID
		}
		if idx, ok := seen[key]; ok {
			// Append this person to existing event
			result[idx].Who += ", " + ev.Who
		} else {
			seen[key] = len(result)
			result = append(result, ev)
		}
	}
	return result
}

func eventDedupeKey(ev *calendar.Event, startTime time.Time) string {
	uid := strings.TrimSpace(ev.ICalUID)
	if uid == "" {
		uid = strings.TrimSpace(ev.Id)
	}
	if uid == "" {
		return ""
	}
	if startTime.IsZero() {
		return uid
	}
	return fmt.Sprintf("%s|%s", uid, startTime.Format(time.RFC3339))
}
