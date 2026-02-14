package timeparse

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrEmptyDate          = errors.New("empty date")
	ErrInvalidDate        = errors.New("invalid date")
	ErrEmptyDateTime      = errors.New("empty date/time")
	ErrInvalidDateTime    = errors.New("invalid date/time")
	ErrEmptyTimeExpr      = errors.New("empty time expression")
	ErrInvalidTimeExpr    = errors.New("invalid time expression")
	ErrEmptySince         = errors.New("empty since value")
	ErrInvalidSince       = errors.New("invalid since value")
	ErrInvalidDateLayouts = errors.New("invalid date format")
)

// ParsedDateTime represents a parsed time expression and whether the input
// carried an explicit clock component.
type ParsedDateTime struct {
	Time    time.Time
	HasTime bool
}

// SinceResult keeps normalized "since" values, preserving RFC3339Nano output
// when the input used fractional seconds.
type SinceResult struct {
	Time           time.Time
	UseRFC3339Nano bool
}

// ParseDate parses a strict date in YYYY-MM-DD format.
func ParseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, ErrEmptyDate
	}

	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidDate, value)
	}

	return t, nil
}

// ParseDateTimeOrDate parses flexible date inputs commonly used across commands.
// Supported: RFC3339/RFC3339Nano, ISO-8601 numeric offset (-0800),
// YYYY-MM-DD, and local datetime layouts without timezone.
func ParseDateTimeOrDate(value string, loc *time.Location) (ParsedDateTime, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ParsedDateTime{}, ErrEmptyDateTime
	}

	if loc == nil {
		loc = time.Local
	}

	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ParsedDateTime{Time: t, HasTime: true}, nil
	}

	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return ParsedDateTime{Time: t, HasTime: true}, nil
	}

	if t, err := time.Parse("2006-01-02T15:04:05-0700", value); err == nil {
		return ParsedDateTime{Time: t, HasTime: true}, nil
	}

	if t, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
		return ParsedDateTime{Time: t, HasTime: false}, nil
	}

	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return ParsedDateTime{Time: t, HasTime: true}, nil
		}
	}

	return ParsedDateTime{}, fmt.Errorf("%w: %q", ErrInvalidDateTime, value)
}

// ParseRangeExpr parses calendar range expressions.
// Supported: absolute datetime/date forms from ParseDateTimeOrDate plus
// relative forms (now/today/tomorrow/yesterday/monday/next monday).
func ParseRangeExpr(expr string, now time.Time, loc *time.Location) (time.Time, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return time.Time{}, ErrEmptyTimeExpr
	}

	exprLower := strings.ToLower(expr)
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

	if t, ok := parseWeekday(exprLower, now); ok {
		return t, nil
	}

	parsed, err := ParseDateTimeOrDate(expr, loc)
	if err == nil {
		return parsed.Time, nil
	}

	return time.Time{}, fmt.Errorf("%w: %q (try: 2026-01-05, today, tomorrow, monday)", ErrInvalidTimeExpr, expr)
}

// ParseSince parses --since values for tracking style queries.
// Supported: duration (24h), date (YYYY-MM-DD), RFC3339(+nano), and
// local datetime layouts.
func ParseSince(value string, now time.Time, loc *time.Location) (SinceResult, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return SinceResult{}, ErrEmptySince
	}

	if d, err := time.ParseDuration(value); err == nil {
		return SinceResult{Time: now.Add(-d).UTC()}, nil
	}

	if t, err := ParseDate(value); err == nil {
		return SinceResult{Time: t.UTC()}, nil
	}

	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return SinceResult{Time: t.UTC(), UseRFC3339Nano: strings.Contains(value, ".")}, nil
	}

	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return SinceResult{Time: t.UTC()}, nil
	}

	parsed, err := ParseDateTimeOrDate(value, loc)
	if err == nil {
		return SinceResult{Time: parsed.Time.UTC()}, nil
	}

	return SinceResult{}, fmt.Errorf("%w: %q", ErrInvalidSince, value)
}

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

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
