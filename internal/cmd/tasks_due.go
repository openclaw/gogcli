package cmd

import (
	"strings"

	"github.com/openclaw/gogcli/internal/ui"
)

func warnTasksDueTime(u *ui.UI, due string) {
	if u == nil {
		return
	}
	due = strings.TrimSpace(due)
	if due == "" {
		return
	}
	if strings.Contains(due, "T") || strings.Contains(due, ":") {
		u.Err().Println("Note: Google Tasks treats due dates as date-only; time components may be ignored.")
	}
}

func normalizeTaskDue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, hasTime, err := parseTaskDate(value)
	if err != nil {
		return "", newUsageError(err)
	}
	return formatTaskDue(parsed, hasTime), nil
}
