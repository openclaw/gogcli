package cmd

import (
	"strings"
)

const (
	eventTypeDefault         = "default"
	eventTypeBirthday        = "birthday"
	eventTypeFocusTime       = "focusTime"
	eventTypeFromGmail       = "fromGmail"
	eventTypeOutOfOffice     = "outOfOffice"
	eventTypeWorkingLocation = "workingLocation"

	defaultFocusSummary     = "Focus Time"
	defaultOOOSummary       = "Out of office"
	defaultOOODeclineMsg    = "I am out of office and will respond when I return."
	defaultFocusAutoDecline = literalAll
	defaultFocusChatStatus  = "doNotDisturb"
	defaultOOOAutoDecline   = literalAll
)

// eventTypeAliases maps user-supplied event type spellings (canonical and
// friendly aliases, all lowercase) to the canonical Calendar API eventType
// value. It is the single source of truth for both the create/update path
// (which further restricts to creatableEventTypes) and the events.list
// eventTypes filter (which accepts every type the API can return).
var eventTypeAliases = map[string]string{
	"default":          eventTypeDefault,
	"birthday":         eventTypeBirthday,
	"focus":            eventTypeFocusTime,
	"focus-time":       eventTypeFocusTime,
	"focustime":        eventTypeFocusTime,
	"focus_time":       eventTypeFocusTime,
	"from-gmail":       eventTypeFromGmail,
	"fromgmail":        eventTypeFromGmail,
	"from_gmail":       eventTypeFromGmail,
	"ooo":              eventTypeOutOfOffice,
	"out-of-office":    eventTypeOutOfOffice,
	"outofoffice":      eventTypeOutOfOffice,
	"out_of_office":    eventTypeOutOfOffice,
	"wl":               eventTypeWorkingLocation,
	"working-location": eventTypeWorkingLocation,
	"workinglocation":  eventTypeWorkingLocation,
	"working_location": eventTypeWorkingLocation,
}

// creatableEventTypes are the event types that can be set when creating or
// updating an event. birthday and fromGmail are read-only — they sync from
// Google Contacts and Gmail — so the create/update path rejects them, even
// though they remain valid values for the events.list filter.
var creatableEventTypes = map[string]bool{
	eventTypeDefault:         true,
	eventTypeFocusTime:       true,
	eventTypeOutOfOffice:     true,
	eventTypeWorkingLocation: true,
}

// normalizeEventType maps a user-supplied event type to a canonical value for
// the create/update path, which accepts only the user-creatable types. An empty
// value returns an empty string so callers can fall back to the boolean
// event-type flags.
func normalizeEventType(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", nil
	}
	if canonical, ok := eventTypeAliases[raw]; ok && creatableEventTypes[canonical] {
		return canonical, nil
	}
	return "", usagef("invalid event type: %q (must be one of: default, focus-time, out-of-office, working-location)", raw)
}

func resolveEventType(raw string, focusFlags, oooFlags, workingFlags bool) (string, error) {
	eventType, err := normalizeEventType(raw)
	if err != nil {
		return "", err
	}
	if eventType == "" {
		count := 0
		if focusFlags {
			count++
		}
		if oooFlags {
			count++
		}
		if workingFlags {
			count++
		}
		if count > 1 {
			return "", usage("event-type flags are mixed; choose one of focus-time, out-of-office, or working-location")
		}
		switch {
		case focusFlags:
			return eventTypeFocusTime, nil
		case oooFlags:
			return eventTypeOutOfOffice, nil
		case workingFlags:
			return eventTypeWorkingLocation, nil
		default:
			return "", nil
		}
	}
	switch eventType {
	case eventTypeFocusTime:
		if oooFlags || workingFlags {
			return "", usage("focus-time cannot be combined with out-of-office or working-location flags")
		}
	case eventTypeOutOfOffice:
		if focusFlags || workingFlags {
			return "", usage("out-of-office cannot be combined with focus-time or working-location flags")
		}
	case eventTypeWorkingLocation:
		if focusFlags || oooFlags {
			return "", usage("working-location cannot be combined with focus-time or out-of-office flags")
		}
	}
	return eventType, nil
}

// normalizeFilterEventType maps a user-supplied event type (canonical or a
// friendly alias) to a canonical Calendar API eventType value for the
// events.list eventTypes filter. Unlike normalizeEventType, it accepts every
// type the API can return — including the read-only birthday and fromGmail,
// which are common things to filter on.
func normalizeFilterEventType(raw string) (string, error) {
	if canonical, ok := eventTypeAliases[strings.TrimSpace(strings.ToLower(raw))]; ok {
		return canonical, nil
	}
	return "", usagef("invalid event type: %q (must be one of: default, birthday, focus-time, from-gmail, out-of-office, working-location)", raw)
}

// resolveFilterEventTypes flattens repeated and comma-separated --event-types
// values into a deduplicated list of canonical Calendar API eventType values,
// preserving first-seen order. A nil/empty slice (flag absent) leaves the
// request unfiltered — the API default of returning all types — while a flag
// that resolves to no values (e.g. --event-types "") is a usage error.
func resolveFilterEventTypes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{})
	for _, item := range raw {
		for _, part := range splitCSV(item) {
			canonical, err := normalizeFilterEventType(part)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[canonical]; ok {
				continue
			}
			seen[canonical] = struct{}{}
			out = append(out, canonical)
		}
	}
	if len(out) == 0 {
		return nil, usage("--event-types must include at least one value")
	}
	return out, nil
}
