package cmd

import (
	"fmt"
	"strings"
)

const (
	eventTypeDefault         = "default"
	eventTypeFocusTime       = "focusTime"
	eventTypeOutOfOffice     = "outOfOffice"
	eventTypeWorkingLocation = "workingLocation"

	defaultFocusSummary     = "Focus Time"
	defaultOOOSummary       = "Out of office"
	defaultOOODeclineMsg    = "I am out of office and will respond when I return."
	defaultFocusAutoDecline = literalAll
	defaultFocusChatStatus  = "doNotDisturb"
	defaultOOOAutoDecline   = literalAll
)

func normalizeEventType(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return "", nil
	}
	switch raw {
	case eventTypeDefault:
		return eventTypeDefault, nil
	case "focus", "focus-time", "focustime", "focus_time":
		return eventTypeFocusTime, nil
	case "out-of-office", "ooo", "outofoffice", "out_of_office":
		return eventTypeOutOfOffice, nil
	case "working-location", "workinglocation", "working_location", "wl":
		return eventTypeWorkingLocation, nil
	default:
		return "", fmt.Errorf("invalid event type: %q (must be %s, focus-time, out-of-office, or working-location)", raw, eventTypeDefault)
	}
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
			return "", fmt.Errorf("event-type flags are mixed; choose one of focus-time, out-of-office, or working-location")
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
			return "", fmt.Errorf("focus-time cannot be combined with out-of-office or working-location flags")
		}
	case eventTypeOutOfOffice:
		if focusFlags || workingFlags {
			return "", fmt.Errorf("out-of-office cannot be combined with focus-time or working-location flags")
		}
	case eventTypeWorkingLocation:
		if focusFlags || oooFlags {
			return "", fmt.Errorf("working-location cannot be combined with focus-time or out-of-office flags")
		}
	}
	return eventType, nil
}
