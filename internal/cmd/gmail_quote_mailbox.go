package cmd

import (
	"html"
	"net/mail"
	"strings"
)

// parseQuoteMailbox splits a header holding one mailbox, such as From, into its
// display name and address. ok is false when the value is not a single
// parseable mailbox.
func parseQuoteMailbox(value string) (name, address string, ok bool) {
	addr, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return "", "", false
	}
	// ParseAddress unquotes a quoted local part ("john smith"@example.com). A
	// nameless Address's String is "<addr-spec>" with the local part re-quoted
	// where needed, so trimming the brackets leaves a valid addr-spec.
	address = strings.TrimSuffix(strings.TrimPrefix((&mail.Address{Address: addr.Address}).String(), "<"), ">")
	return strings.TrimSpace(addr.Name), address, true
}

// formatQuoteMailbox renders a header holding one mailbox the way Gmail shows
// a sender in a quote attribution: "Name <email>" with a display name,
// "<email>" without one. A value that is not a single parseable mailbox is
// returned trimmed and otherwise unchanged. (formatMailbox, in
// gmail_recipients.go, writes the wire form of an outgoing recipient instead.)
func formatQuoteMailbox(value string) string {
	name, address, ok := parseQuoteMailbox(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	if name == "" {
		return "<" + address + ">"
	}
	return name + " <" + address + ">"
}

// formatQuoteMailboxHTML is formatQuoteMailbox for HTML: the same text,
// escaped, with a parsed address linked via mailto:. A value that does not
// parse is escaped but not linked.
func formatQuoteMailboxHTML(value string) string {
	name, address, ok := parseQuoteMailbox(value)
	if !ok {
		return html.EscapeString(strings.TrimSpace(value))
	}
	link := `&lt;<a href="` + html.EscapeString(mailtoURI(address)) + `">` + html.EscapeString(address) + `</a>&gt;`
	if name == "" {
		return link
	}
	return html.EscapeString(name) + " " + link
}

// mailtoURI returns a mailto: URI for an addr-spec (RFC 6068). Both sides of
// the final "@" are percent-encoded except for unreserved characters and "+".
// Otherwise characters that are valid in a mailbox would change the link: "?"
// would start header fields, "#" a fragment that cuts the address short, and
// "," or a quoted "@" would split it. "+" stays raw so plus-addressed hrefs
// remain readable.
func mailtoURI(addrSpec string) string {
	at := strings.LastIndexByte(addrSpec, '@')
	if at < 0 {
		// A parsed addr-spec always has an "@"; escape anything else whole
		// rather than slicing at -1.
		return "mailto:" + mailtoEscape(addrSpec)
	}
	return "mailto:" + mailtoEscape(addrSpec[:at]) + "@" + mailtoEscape(addrSpec[at+1:])
}

func mailtoEscape(s string) string {
	const hexDigits = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', strings.IndexByte("-._~+", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0f])
		}
	}
	return b.String()
}
