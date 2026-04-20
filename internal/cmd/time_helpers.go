package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

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
// Uses Calendars.Get (not CalendarList.Get) so it works for service accounts
// whose "primary" calendar may not appear in their CalendarList.
func getCalendarLocation(ctx context.Context, svc *calendar.Service, calendarID string) (string, *time.Location, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		return "", nil, fmt.Errorf("calendarId required")
	}

	cal, err := svc.Calendars.Get(calendarID).Context(ctx).Do()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get calendar %q: %w", calendarID, err)
	}
	if cal.TimeZone == "" {
		return "", nil, fmt.Errorf("calendar %q has no timezone set", calendarID)
	}
	loc, err := loadTimezoneLocation(cal.TimeZone)
	if err != nil {
		return "", nil, fmt.Errorf("invalid calendar timezone %q: %w", cal.TimeZone, err)
	}
	return cal.TimeZone, loc, nil
}

// getUserTimezone fetches the user's timezone.
// It first tries calendarList/primary, then Calendars.Get("primary") for
// service-account compatibility, then falls back to calendar-list entries
// because some Workspace accounts return 404 for the primary alias.
func getUserTimezone(ctx context.Context, svc *calendar.Service) (*time.Location, error) {
	calEntry, listAliasErr := svc.CalendarList.Get(primaryCalendarID).Context(ctx).Do()
	if listAliasErr == nil {
		return loadTimezoneOrUTC(calEntry.TimeZone)
	}
	if !isGoogleNotFound(listAliasErr) {
		return nil, fmt.Errorf("failed to get primary calendar: %w", listAliasErr)
	}

	cal, directErr := svc.Calendars.Get(primaryCalendarID).Context(ctx).Do()
	if directErr == nil {
		return loadTimezoneOrUTC(cal.TimeZone)
	}
	if !isGoogleNotFound(directErr) {
		return nil, fmt.Errorf("failed to get primary calendar via calendar list and direct lookup: %w", errors.Join(listAliasErr, directErr))
	}

	calendars, listErr := listCalendarList(ctx, svc)
	if listErr != nil {
		return nil, fmt.Errorf("failed to get primary calendar via alias, direct lookup, and fallback calendar list lookup: %w", errors.Join(listAliasErr, directErr, listErr))
	}

	return timezoneFromCalendarList(calendars)
}

func timezoneFromCalendarList(calendars []*calendar.CalendarListEntry) (*time.Location, error) {
	var fallback *time.Location
	var firstInvalid error
	for _, cal := range calendars {
		if cal == nil {
			continue
		}
		tz := strings.TrimSpace(cal.TimeZone)
		if tz == "" {
			continue
		}
		loc, err := loadTimezoneLocation(tz)
		if err != nil {
			if firstInvalid == nil {
				firstInvalid = fmt.Errorf("invalid calendar timezone %q on calendar %q: %w", tz, cal.Id, err)
			}
			continue
		}
		if cal.Primary {
			return loc, nil
		}
		if fallback == nil {
			fallback = loc
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	if firstInvalid != nil {
		return nil, firstInvalid
	}

	return time.UTC, nil
}

func loadTimezoneOrUTC(timezone string) (*time.Location, error) {
	if strings.TrimSpace(timezone) == "" {
		return time.UTC, nil
	}
	loc, err := loadTimezoneLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid calendar timezone %q: %w", timezone, err)
	}
	return loc, nil
}

func isGoogleNotFound(err error) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && gerr.Code == http.StatusNotFound
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

func parseTimeExprEndOfDay(expr string, now time.Time, loc *time.Location) (time.Time, error) {
	t, err := parseTimeExpr(expr, now, loc)
	if err != nil {
		return t, err
	}
	if isDayExpr(expr, now, loc) {
		return endOfDay(t), nil
	}
	return t, nil
}

func isDayExpr(expr string, now time.Time, loc *time.Location) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	exprLower := strings.ToLower(expr)
	switch exprLower {
	case "today", "tomorrow", "yesterday":
		return true
	case "now":
		return false
	}
	if _, ok := parseWeekday(exprLower, now); ok {
		return true
	}
	if _, err := time.ParseInLocation("2006-01-02", expr, loc); err == nil {
		return true
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
	return timeparse.ParseWeekdayExpr(expr, now)
}

// startOfDay returns the start of the day (00:00:00) in the given time's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// endOfDay returns the start of the next day so that when formatted as
// RFC3339 (second precision) and used as an exclusive timeMax, the entire
// target day is included. Previously, 23:59:59.999999999 was truncated to
// 23:59:59 by time.RFC3339, which excluded events starting at that second.
func endOfDay(t time.Time) time.Time {
	return startOfDay(t.AddDate(0, 0, 1))
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
	return timeparse.ParseWeekdayName(value)
}
