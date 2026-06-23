package cmd

import (
	"net/mail"
	"strings"
	"time"
)

const (
	// listDateLayout renders message-list and thread dates, e.g. "2026-06-17 00:29".
	listDateLayout = "2006-01-02 15:04"
	// quoteDateLayout renders dates in the Gmail-style human form used for reply
	// attributions and forwarded-message headers, e.g. "Wed, Jun 17, 2026 at 12:29 AM".
	quoteDateLayout = "Mon, Jan 2, 2006 at 3:04 PM"
)

// formatMailDateInLocation parses an RFC 2822 Date header and renders it with
// layout, converted into loc (a nil loc falls back to time.Local). Empty input
// and dates that cannot be parsed are returned trimmed and otherwise unchanged,
// so an unexpected Date value never breaks the output. A parseable date's
// weekday is recomputed from the instant in loc, which also corrects a wrong
// weekday in the source header.
func formatMailDateInLocation(raw string, loc *time.Location, layout string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	if t, err := mailParseDate(raw); err == nil {
		return t.In(loc).Format(layout)
	}
	return raw
}

// formatGmailDateInLocation renders a Date header for message and thread listings.
func formatGmailDateInLocation(raw string, loc *time.Location) string {
	return formatMailDateInLocation(raw, loc, listDateLayout)
}

// formatQuoteDate renders an original message's Date header in the Gmail-style
// human form used for quoted replies and forwarded-message headers, converted
// into loc (matching how Gmail shows the attribution in the reader's timezone).
func formatQuoteDate(raw string, loc *time.Location) string {
	return formatMailDateInLocation(raw, loc, quoteDateLayout)
}

func mailParseDate(s string) (time.Time, error) {
	// net/mail has the most compatible Date parser, but we keep this isolated for easier tests/mocks later.
	return mail.ParseDate(s)
}
