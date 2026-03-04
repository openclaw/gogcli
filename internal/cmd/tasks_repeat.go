package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/timeparse"
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
	if dateOnly, err := timeparse.ParseDate(value); err == nil {
		return dateOnly, false, nil
	}

	parsed, err := timeparse.ParseDateTimeOrDate(value, time.Local)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid date/time %q (expected RFC3339 or YYYY-MM-DD)", strings.TrimSpace(value))
	}
	return parsed.Time, parsed.HasTime, nil
}

func parseRepeatRRule(raw string) (repeatUnit, int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (must include FREQ)", raw)
	}

	if strings.HasPrefix(strings.ToUpper(trimmed), "RRULE:") {
		trimmed = strings.TrimSpace(trimmed[len("RRULE:"):])
	}
	if trimmed == "" {
		return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (must include FREQ)", raw)
	}

	unit := repeatNone
	interval := 1
	seenFreq := false
	seenInterval := false
	for _, part := range strings.Split(trimmed, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (malformed token %q)", raw, part)
		}

		key := strings.ToUpper(strings.TrimSpace(kv[0]))
		value := strings.ToUpper(strings.TrimSpace(kv[1]))

		switch key {
		case "FREQ":
			if seenFreq {
				return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (duplicate FREQ)", raw)
			}
			seenFreq = true
			switch value {
			case "DAILY":
				unit = repeatDaily
			case "WEEKLY":
				unit = repeatWeekly
			case "MONTHLY":
				unit = repeatMonthly
			case "YEARLY":
				unit = repeatYearly
			default:
				return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (unsupported FREQ %q)", raw, value)
			}
		case "INTERVAL":
			if seenInterval {
				return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (duplicate INTERVAL)", raw)
			}
			seenInterval = true
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (INTERVAL must be a positive integer)", raw)
			}
			interval = parsed
		default:
			return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (unsupported key %q; only FREQ and INTERVAL are supported)", raw, key)
		}
	}

	if unit == repeatNone {
		return repeatNone, 0, fmt.Errorf("invalid --recur-rrule %q (missing FREQ)", raw)
	}

	return unit, interval, nil
}

func expandRepeatSchedule(start time.Time, unit repeatUnit, interval int, count int, until *time.Time) []time.Time {
	if unit == repeatNone {
		return []time.Time{start}
	}
	if interval <= 0 {
		interval = 1
	}
	if count < 0 {
		count = 0
	}
	// Defensive guard: if neither count nor until is set, return single occurrence
	// to prevent infinite loop (caller should validate, but be safe)
	if count == 0 && until == nil {
		return []time.Time{start}
	}
	out := []time.Time{}
	for i := 0; ; i++ {
		t := addRepeat(start, unit, i*interval)
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
	return t.UTC().Format(time.RFC3339)
}
