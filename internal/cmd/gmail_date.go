package cmd

import (
	"net/mail"
	"strings"
	"time"
)

func formatGmailDateInLocation(raw string, loc *time.Location) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	if t, err := mailParseDate(raw); err == nil {
		return t.In(loc).Format("2006-01-02 15:04")
	}
	return raw
}

func mailParseDate(s string) (time.Time, error) {
	// net/mail has the most compatible Date parser, but we keep this isolated for easier tests/mocks later.
	return mail.ParseDate(s)
}
