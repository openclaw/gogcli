package cmd

import (
	"strings"

	"github.com/steipete/gogcli/internal/ui"
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
