package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/timeparse"
)

// TimeRangeFlags provides common time range options for calendar commands.
// Embed this struct in commands that need time range support.
type TimeRangeFlags struct {
	From      string `name:"from" help:"Start time (RFC3339, date, or relative: today, tomorrow, monday)"`
	To        string `name:"to" help:"End time (RFC3339, date, or relative)"`
	Today     bool   `name:"today" help:"Today only"`
	Tomorrow  bool   `name:"tomorrow" help:"Tomorrow only"`
	Week      bool   `name:"week" help:"This week (uses --week-start, default Mon)"`
	Days      int    `name:"days" help:"Next N days" default:"0"`
	WeekStart string `name:"week-start" help:"Week start day for --week (sun, mon, ...)" default:""`
}

// TimeRange represents a resolved time range with timezone.
type TimeRange struct {
	From     time.Time
	To       time.Time
	Location *time.Location
}

// getCalendarLocation fetches a calendar's timezone and returns it as a location.
func getCalendarLocation(ctx context.Context, svc *calendar.Service, calendarID string) (string, *time.Location, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		return "", nil, fmt.Errorf("calendarId required")
	}

	cal, err := svc.CalendarList.Get(calendarID).Context(ctx).Do()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get calendar %q: %w", calendarID, err)
	}
	if cal.TimeZone == "" {
		return "", nil, fmt.Errorf("calendar %q has no timezone set", calendarID)
	}
	loc, err := time.LoadLocation(cal.TimeZone)
	if err != nil {
		return "", nil, fmt.Errorf("invalid calendar timezone %q: %w", cal.TimeZone, err)
	}
	return cal.TimeZone, loc, nil
}

// getUserTimezone fetches the timezone from the user's primary calendar.
func getUserTimezone(ctx context.Context, svc *calendar.Service) (*time.Location, error) {
	cal, err := svc.CalendarList.Get("primary").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get primary calendar: %w", err)
	}

	if cal.TimeZone == "" {
		// Fall back to UTC if no timezone set
		return time.UTC, nil
	}

	loc, err := time.LoadLocation(cal.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar timezone %q: %w", cal.TimeZone, err)
	}

	return loc, nil
}

// ResolveTimeRange resolves the time range flags into absolute times.
// If no flags are provided, defaults to "next 7 days" from now.
func ResolveTimeRange(ctx context.Context, svc *calendar.Service, flags TimeRangeFlags) (*TimeRange, error) {
	defaults := TimeRangeDefaults{
		FromOffset:   0,
		ToOffset:     7 * 24 * time.Hour,
		ToFromOffset: 7 * 24 * time.Hour,
	}
	return ResolveTimeRangeWithDefaults(ctx, svc, flags, defaults)
}

// TimeRangeDefaults controls the default window when flags are missing.
type TimeRangeDefaults struct {
	FromOffset   time.Duration
	ToOffset     time.Duration
	ToFromOffset time.Duration
}

// ResolveTimeRangeWithDefaults resolves the time range flags into absolute times,
// using provided defaults when --from/--to are not set.
func ResolveTimeRangeWithDefaults(ctx context.Context, svc *calendar.Service, flags TimeRangeFlags, defaults TimeRangeDefaults) (*TimeRange, error) {
	loc, err := getUserTimezone(ctx, svc)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(loc)
	var from, to time.Time

	weekStart, err := resolveWeekStart(flags.WeekStart)
	if err != nil {
		return nil, err
	}

	// Handle convenience flags first
	switch {
	case flags.Today:
		from = startOfDay(now)
		to = endOfDay(now)
	case flags.Tomorrow:
		tomorrow := now.AddDate(0, 0, 1)
		from = startOfDay(tomorrow)
		to = endOfDay(tomorrow)
	case flags.Week:
		from = startOfWeek(now, weekStart)
		to = endOfWeek(now, weekStart)
	case flags.Days > 0:
		from = startOfDay(now)
		to = endOfDay(now.AddDate(0, 0, flags.Days-1))
	default:
		// Parse --from and --to, or use defaults
		if flags.From != "" {
			from, err = parseTimeExpr(flags.From, now, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid --from: %w", err)
			}
		} else {
			from = now.Add(defaults.FromOffset)
		}

		switch {
		case flags.To != "":
			to, err = parseTimeExprEndOfDay(flags.To, now, loc)
			if err != nil {
				return nil, fmt.Errorf("invalid --to: %w", err)
			}
		case flags.From != "" && defaults.ToFromOffset != 0:
			to = from.Add(defaults.ToFromOffset)
		default:
			to = now.Add(defaults.ToOffset)
		}
	}

	return &TimeRange{
		From:     from,
		To:       to,
		Location: loc,
	}, nil
}

// parseTimeExprEndOfDay is like parseTimeExpr but interprets date-only values
// and relative day expressions as end of day (23:59:59.999999999) instead of
// start of day. This is useful for --to flags where "2026-01-05" should mean
// "through the end of Jan 5" rather than "midnight at the start of Jan 5".
func parseTimeExprEndOfDay(expr string, now time.Time, loc *time.Location) (time.Time, error) {
	t, err := parseTimeExpr(expr, now, loc)
	if err != nil {
		return t, err
	}
	// Only adjust to end-of-day for date-only or relative day expressions.
	// If the input is a full timestamp (contains "T" or "t" indicating time
	// components), respect the exact time the user specified — even midnight.
	if isDateOnlyOrRelative(expr) {
		return endOfDay(t), nil
	}
	return t, nil
}

// isDateOnlyOrRelative returns true if the expression is a date-only string
// (YYYY-MM-DD) or a relative day keyword (today, tomorrow, yesterday, weekday
// names). These should be adjusted to end-of-day when used as an upper bound.
// Point-in-time values like "now" and full timestamps return false.
func isDateOnlyOrRelative(expr string) bool {
	trimmed := strings.TrimSpace(expr)
	lower := strings.ToLower(trimmed)

	// Relative day keywords
	switch lower {
	case "today", "tomorrow", "yesterday":
		return true
	}

	// Weekday names: "monday", "next tuesday", etc.
	candidate := lower
	candidate = strings.TrimPrefix(candidate, "next ")
	weekdays := []string{
		"sunday", "sun", "monday", "mon", "tuesday", "tue",
		"wednesday", "wed", "thursday", "thu", "friday", "fri", "saturday", "sat",
	}
	for _, wd := range weekdays {
		if candidate == wd {
			return true
		}
	}

	// Date-only: YYYY-MM-DD (exactly 10 chars, no time component)
	if len(trimmed) == 10 {
		if _, err := time.Parse("2006-01-02", trimmed); err == nil {
			return true
		}
	}

	return false
}

// parseTimeExpr parses a time expression which can be:
// - RFC3339: 2026-01-05T14:00:00-08:00
// - ISO 8601 with numeric timezone: 2026-01-05T14:00:00-0800 (no colon)
// - Date only: 2026-01-05 (interpreted as start of day in user's timezone)
// - Relative: today, tomorrow, monday, next tuesday
func parseTimeExpr(expr string, now time.Time, loc *time.Location) (time.Time, error) {
	return timeparse.ParseRangeExpr(expr, now, loc)
}

// parseWeekday parses weekday expressions like "monday", "next tuesday"
func parseWeekday(expr string, now time.Time) (time.Time, bool) {
	expr = strings.TrimSpace(expr)
	next := false
	if strings.HasPrefix(expr, "next ") {
		next = true
		expr = strings.TrimPrefix(expr, "next ")
	}

	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"sun":       time.Sunday,
		"monday":    time.Monday,
		"mon":       time.Monday,
		"tuesday":   time.Tuesday,
		"tue":       time.Tuesday,
		"wednesday": time.Wednesday,
		"wed":       time.Wednesday,
		"thursday":  time.Thursday,
		"thu":       time.Thursday,
		"friday":    time.Friday,
		"fri":       time.Friday,
		"saturday":  time.Saturday,
		"sat":       time.Saturday,
	}

	targetDay, ok := weekdays[expr]
	if !ok {
		return time.Time{}, false
	}

	currentDay := now.Weekday()
	daysUntil := int(targetDay) - int(currentDay)
	if daysUntil < 0 || (daysUntil == 0 && next) {
		daysUntil += 7 // Next week
	}

	if daysUntil == 0 {
		return startOfDay(now), true
	}
	return startOfDay(now.AddDate(0, 0, daysUntil)), true
}

// startOfDay returns the start of the day (00:00:00) in the given time's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// endOfDay returns the end of the day (23:59:59.999) in the given time's location.
func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// startOfWeek returns the start of the week for the given time.
func startOfWeek(t time.Time, weekStart time.Weekday) time.Time {
	weekday := int(t.Weekday())
	start := int(weekStart)
	daysBack := weekday - start
	if daysBack < 0 {
		daysBack += 7
	}
	return startOfDay(t.AddDate(0, 0, -daysBack))
}

// endOfWeek returns the end of the week for the given time.
func endOfWeek(t time.Time, weekStart time.Weekday) time.Time {
	start := startOfWeek(t, weekStart)
	return endOfDay(start.AddDate(0, 0, 6))
}

// FormatRFC3339 formats a time as RFC3339 for API calls.
func (tr *TimeRange) FormatRFC3339() (from, to string) {
	return tr.From.Format(time.RFC3339), tr.To.Format(time.RFC3339)
}

// FormatHuman returns a human-readable description of the time range.
func (tr *TimeRange) FormatHuman() string {
	fromDate := tr.From.Format("Mon Jan 2")
	toDate := tr.To.Format("Mon Jan 2")

	if fromDate == toDate {
		return fromDate
	}
	return fmt.Sprintf("%s to %s", fromDate, toDate)
}

func resolveWeekStart(value string) (time.Weekday, error) {
	if value == "" {
		return time.Monday, nil
	}
	if wd, ok := parseWeekStart(value); ok {
		return wd, nil
	}
	return time.Monday, fmt.Errorf("invalid --week-start %q (use sun, mon, ...)", value)
}

func parseWeekStart(value string) (time.Weekday, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sun", "sunday":
		return time.Sunday, true
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tues", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thur", "thurs", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	default:
		return time.Monday, false
	}
}
