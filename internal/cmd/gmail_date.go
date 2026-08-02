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

// formatGmailDateISO renders a message's internalDate as RFC3339 in loc.
//
// Two things make this the value a machine consumer should read:
//
//   - It carries an explicit offset. listDateLayout ("2006-01-02 15:04") does
//     not, so a caller that sees "2026-07-28 03:36" has to already know which
//     zone gog was configured for. Get that wrong and the timestamp can land on
//     the wrong calendar DAY, which is what makes it worse than merely
//     imprecise.
//   - internalDate is Gmail's own internal message timestamp (epoch
//     milliseconds, UTC), independent of the sender's formatting. For normal
//     SMTP-received mail it is when Google accepted the message, which Gmail
//     documents as more reliable than the Date header. It is not universally
//     independent of that header: for API-migrated mail the importing client
//     may derive internalDate FROM the Date header, in which case the two
//     agree by construction rather than by coincidence.
//
// Returns "" when internalDate is absent, so the field stays omitempty rather
// than claiming the Unix epoch.
func formatGmailDateISO(internalDateMillis int64, loc *time.Location) string {
	if internalDateMillis <= 0 {
		return ""
	}
	if loc == nil {
		loc = time.Local
	}
	// internalDate is epoch milliseconds, and time.RFC3339 has no fractional
	// seconds — formatting a nonzero-millisecond instant with it would truncate
	// to the preceding whole second. Whole seconds keep the fraction-free form.
	layout := time.RFC3339
	if internalDateMillis%1000 != 0 {
		layout = "2006-01-02T15:04:05.000Z07:00"
	}
	return time.UnixMilli(internalDateMillis).In(loc).Format(layout)
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
