package cmd

import (
	"fmt"
	"strings"
	"time"
)

type repeatUnit int

const (
	repeatNone repeatUnit = iota
	repeatDaily
	repeatWeekly
	repeatMonthly
	repeatYearly
)

func parseRepeatUnit(raw string) (repeatUnit, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return repeatNone, nil
	}
	switch raw {
	case "daily", "day":
		return repeatDaily, nil
	case "weekly", "week":
		return repeatWeekly, nil
	case "monthly", "month":
		return repeatMonthly, nil
	case "yearly", "year", "annually":
		return repeatYearly, nil
	default:
		return repeatNone, fmt.Errorf("invalid repeat value %q (must be daily, weekly, monthly, or yearly)", raw)
	}
}

func parseTaskDate(value string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false, fmt.Errorf("empty date")
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, true, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, false, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", value, time.Local); err == nil {
		return t, true, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("invalid date/time %q (expected RFC3339 or YYYY-MM-DD)", value)
}

func expandRepeatSchedule(start time.Time, unit repeatUnit, count int, until *time.Time) []time.Time {
	if unit == repeatNone {
		return []time.Time{start}
	}
	if count < 0 {
		count = 0
	}
	out := []time.Time{}
	for i := 0; ; i++ {
		t := addRepeat(start, unit, i)
		if until != nil && t.After(*until) {
			break
		}
		out = append(out, t)
		if count > 0 && len(out) >= count {
			break
		}
	}
	return out
}

func addRepeat(t time.Time, unit repeatUnit, n int) time.Time {
	switch unit {
	case repeatDaily:
		return t.AddDate(0, 0, n)
	case repeatWeekly:
		return t.AddDate(0, 0, 7*n)
	case repeatMonthly:
		return t.AddDate(0, n, 0)
	case repeatYearly:
		return t.AddDate(n, 0, 0)
	default:
		return t
	}
}

func formatTaskDue(t time.Time, hasTime bool) string {
	if hasTime {
		return t.Format(time.RFC3339)
	}
	return t.Format("2006-01-02")
}
