package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

const tzUTC = "UTC"

func buildEventDateTime(value string, allDay bool) *calendar.EventDateTime {
	value = strings.TrimSpace(value)
	if allDay {
		return &calendar.EventDateTime{Date: value}
	}

	edt := &calendar.EventDateTime{DateTime: value}
	if tz := extractTimezone(value); tz != "" {
		edt.TimeZone = tz
	}
	return edt
}

func etcGMTForOffsetSeconds(offset int) (string, bool) {
	if offset == 0 || offset%3600 != 0 {
		return "", false
	}
	hours := offset / 3600
	if hours > 0 {
		// NOTE: IANA "Etc/GMT" names use reversed signs (e.g. +02:00 => Etc/GMT-2).
		return fmt.Sprintf("Etc/GMT-%d", hours), true
	}
	return fmt.Sprintf("Etc/GMT+%d", -hours), true
}

func usIANAForOffsetAt(t time.Time, offset int) string {
	switch offset {
	case -4 * 3600, -5 * 3600, -6 * 3600, -7 * 3600, -8 * 3600:
		for _, candidate := range []string{
			"America/New_York",
			"America/Chicago",
			"America/Denver",
			"America/Phoenix",
			"America/Los_Angeles",
		} {
			loc, err := time.LoadLocation(candidate)
			if err != nil {
				continue
			}
			_, candidateOffset := t.In(loc).Zone()
			if candidateOffset == offset {
				return candidate
			}
		}
	}
	return ""
}

// extractTimezone attempts to determine a timezone from an RFC3339 datetime string.
// Returns an IANA timezone name if determinable, empty string otherwise.
func extractTimezone(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return ""
	}

	_, offset := t.Zone()
	if offset == 0 {
		return tzUTC
	}

	// RFC3339 values have a fixed offset, but Google Calendar requires an IANA timezone
	// name for recurring events. We guess by checking which common zones match the
	// offset at this instant.
	if tz := usIANAForOffsetAt(t, offset); tz != "" {
		return tz
	}

	// Fallback for fixed whole-hour offsets when no regional timezone match is found.
	if tz, ok := etcGMTForOffsetSeconds(offset); ok {
		return tz
	}
	return ""
}

func buildConferenceData(withMeet bool) *calendar.ConferenceData {
	if !withMeet {
		return nil
	}
	return &calendar.ConferenceData{
		CreateRequest: &calendar.CreateConferenceRequest{
			RequestId: fmt.Sprintf("gogcli-%d", time.Now().UnixNano()),
			ConferenceSolutionKey: &calendar.ConferenceSolutionKey{
				Type: "hangoutsMeet",
			},
		},
	}
}

func buildRecurrence(rules []string) []string {
	if len(rules) == 0 {
		return nil
	}
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r != "" {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var durationRegex = regexp.MustCompile(`^(\d+)(w|d|h|m)?$`)

func parseDuration(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	match := durationRegex.FindStringSubmatch(s)
	if match == nil {
		return 0, fmt.Errorf("invalid duration format: %q (expected e.g., 30, 30m, 1h, 3d, 1w)", s)
	}

	value, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration number: %q", match[1])
	}

	unit := match[2]
	switch unit {
	case "w":
		value *= 7 * 24 * 60
	case "d":
		value *= 24 * 60
	case "h":
		value *= 60
	case "m", "":
	}

	if value < 0 || value > 40320 {
		return 0, fmt.Errorf("reminder duration must be 0-40320 minutes (got %d)", value)
	}

	return value, nil
}

func parseReminder(s string) (string, int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0, fmt.Errorf("empty reminder")
	}

	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid reminder format: %q (expected method:duration, e.g., popup:30m)", s)
	}

	method := strings.TrimSpace(strings.ToLower(parts[0]))
	if method != "email" && method != "popup" {
		return "", 0, fmt.Errorf("invalid reminder method: %q (expected 'email' or 'popup')", method)
	}

	minutes, err := parseDuration(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid reminder duration: %w", err)
	}

	return method, minutes, nil
}

//nolint:nilnil // nil return is intentional: nil means "use calendar defaults"
func buildReminders(reminders []string) (*calendar.EventReminders, error) {
	if len(reminders) == 0 {
		return nil, nil
	}

	var filtered []string
	for _, r := range reminders {
		if strings.TrimSpace(r) != "" {
			filtered = append(filtered, r)
		}
	}

	if len(filtered) == 0 {
		return nil, nil
	}

	if len(filtered) > 5 {
		return nil, fmt.Errorf("maximum 5 reminders allowed (got %d)", len(filtered))
	}

	overrides := make([]*calendar.EventReminder, 0, len(filtered))
	for _, r := range filtered {
		method, minutes, err := parseReminder(r)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, &calendar.EventReminder{
			Method:  method,
			Minutes: minutes,
		})
	}

	// ForceSendFields ensures UseDefault=false is sent (not omitted as zero value)
	return &calendar.EventReminders{
		UseDefault:      false,
		Overrides:       overrides,
		ForceSendFields: []string{"UseDefault"},
	}, nil
}

func buildAttachments(urls []string) []*calendar.EventAttachment {
	if len(urls) == 0 {
		return nil
	}
	out := make([]*calendar.EventAttachment, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u != "" {
			out = append(out, &calendar.EventAttachment{FileUrl: u})
		}
	}
	return out
}

func buildExtendedProperties(privateProps, sharedProps []string) *calendar.EventExtendedProperties {
	if len(privateProps) == 0 && len(sharedProps) == 0 {
		return nil
	}
	props := &calendar.EventExtendedProperties{}

	if len(privateProps) > 0 {
		props.Private = make(map[string]string)
		for _, p := range privateProps {
			if k, v, ok := strings.Cut(p, "="); ok {
				props.Private[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	if len(sharedProps) > 0 {
		props.Shared = make(map[string]string)
		for _, p := range sharedProps {
			if k, v, ok := strings.Cut(p, "="); ok {
				props.Shared[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	return props
}
