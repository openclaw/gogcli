package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Matches: "2h ago", "30m ago", "1d ago", "2w ago", "1mo ago"
var relativeAgoRegex = regexp.MustCompile(`^(\d+)(mo|w|d|h|m)\s*ago$`)

// Matches: "30m", "2h", "1d" (future, for reminders)
var relativeFutureRegex = regexp.MustCompile(`^(\d+)(mo|w|d|h|m)$`)

// ParseRelativeTime parses human-friendly time expressions.
// Supports: "2h ago", "yesterday", "monday", "next tue", "30m", RFC3339.
func ParseRelativeTime(s string, now time.Time) (time.Time, error) {
	expr := strings.TrimSpace(s)
	if expr == "" {
		return time.Time{}, fmt.Errorf("empty time expression")
	}

	// Try RFC3339 first (before lowercasing).
	if t, err := time.Parse(time.RFC3339, expr); err == nil {
		return t, nil
	}

	// Try ISO 8601 with numeric timezone without colon (e.g., -0800).
	if t, err := time.Parse("2006-01-02T15:04:05-0700", expr); err == nil {
		return t, nil
	}

	exprLower := strings.ToLower(expr)

	if matches := relativeAgoRegex.FindStringSubmatch(exprLower); matches != nil {
		return applyRelativeDuration(matches[1], matches[2], now, -1)
	}
	if matches := relativeFutureRegex.FindStringSubmatch(exprLower); matches != nil {
		return applyRelativeDuration(matches[1], matches[2], now, 1)
	}

	// Try relative day expressions.
	switch exprLower {
	case "now":
		return now, nil
	case "today":
		return startOfDay(now), nil
	case "tomorrow":
		return startOfDay(now.AddDate(0, 0, 1)), nil
	case "yesterday":
		return startOfDay(now.AddDate(0, 0, -1)), nil
	}

	// Try day of week (this week or next).
	if t, ok := parseWeekday(exprLower, now); ok {
		return t, nil
	}

	loc := now.Location()
	if loc == nil {
		loc = time.Local
	}

	// Try date only (YYYY-MM-DD).
	if t, err := time.ParseInLocation("2006-01-02", expr, loc); err == nil {
		return t, nil
	}

	// Try date with time but no timezone.
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", expr, loc); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", expr, loc); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q as time", s)
}

func applyRelativeDuration(value string, unit string, now time.Time, direction int) (time.Time, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid duration %q", value)
	}
	if n == 0 {
		return now, nil
	}
	if direction < 0 {
		n = -n
	}

	switch unit {
	case "mo":
		return now.AddDate(0, n, 0), nil
	case "w":
		return now.AddDate(0, 0, n*7), nil
	case "d":
		return now.AddDate(0, 0, n), nil
	case "h":
		return now.Add(time.Duration(n) * time.Hour), nil
	case "m":
		return now.Add(time.Duration(n) * time.Minute), nil
	default:
		return time.Time{}, fmt.Errorf("invalid duration unit %q", unit)
	}
}

// startOfDay returns the start of the day (00:00:00) in the given time's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// endOfDay returns the end of the day (23:59:59.999) in the given time's location.
func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// parseWeekday parses weekday expressions like "monday", "next tuesday".
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
		daysUntil += 7
	}

	if daysUntil == 0 {
		return startOfDay(now), true
	}
	return startOfDay(now.AddDate(0, 0, daysUntil)), true
}
