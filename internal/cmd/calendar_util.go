package cmd

import (
	"strings"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/config"
)

func isAllDayEvent(e *calendar.Event) bool {
	return e != nil && e.Start != nil && e.Start.Date != ""
}

// resolveCalendarAliasID resolves a calendar ID, checking aliases first.
// Returns an error if the calendar ID is empty after resolution.
func resolveCalendarAliasID(calendarID string) (string, error) {
	calendarID = strings.TrimSpace(calendarID)
	if calendarID == "" {
		return "", usage("empty calendarId")
	}

	resolved, err := config.ResolveCalendarID(calendarID)
	if err != nil {
		return "", err
	}

	return resolved, nil
}
