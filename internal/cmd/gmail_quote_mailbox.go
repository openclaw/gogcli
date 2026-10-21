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
	name, address = quoteMailboxParts(addr)
	return name, address, true
}

// quoteMailboxParts returns a parsed mailbox's display name and addr-spec.
// net/mail's parsers unquote a quoted local part ("john smith"@example.com). A
// nameless Address's String is "<addr-spec>" with the local part re-quoted
// where needed, so trimming the brackets leaves a valid addr-spec.
func quoteMailboxParts(addr *mail.Address) (name, address string) {
	address = strings.TrimSuffix(strings.TrimPrefix((&mail.Address{Address: addr.Address}).String(), "<"), ">")
	return strings.TrimSpace(addr.Name), address
}

// formatQuoteMailbox renders a header holding one mailbox the way Gmail shows
// a sender in a reply's attribution line or a forward's header block:
// "Name <email>" with a display name, "<email>" without one. A value that is
// not a single parseable mailbox is returned trimmed and otherwise unchanged.
// (formatMailbox, in gmail_recipients.go, writes the wire form of an outgoing
// recipient instead.)
func formatQuoteMailbox(value string) string {
	name, address, ok := parseQuoteMailbox(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return quoteMailboxText(name, address)
}

// formatQuoteMailboxHTML is formatQuoteMailbox for HTML: the same text,
// escaped, with a parsed address linked via mailto: and the name bold or not,
// per style. A value that does not parse is escaped but not linked.
func formatQuoteMailboxHTML(value string, style nameStyle) string {
	name, address, ok := parseQuoteMailbox(value)
	if !ok {
		return html.EscapeString(strings.TrimSpace(value))
	}
	return quoteMailboxHTML(name, address, style)
}

// formatQuoteMailboxList renders a header holding a list of mailboxes, such as
// To or Cc: each mailbox as formatQuoteMailbox renders one, joined by ", ".
// Like Gmail, it lists a group's members without the group's name, so a group
// with none ("undisclosed-recipients:;") renders empty. A value that is not a
// parseable address list is returned trimmed and otherwise unchanged.
func formatQuoteMailboxList(value string) string {
	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	mailboxes := make([]string, len(addrs))
	for i, addr := range addrs {
		mailboxes[i] = quoteMailboxText(quoteMailboxParts(addr))
	}
	return strings.Join(mailboxes, ", ")
}

// formatQuoteMailboxListHTML is formatQuoteMailboxList for HTML: each mailbox
// as formatQuoteMailboxHTML renders one, with the name not bold. A value that
// does not parse is escaped but not linked.
func formatQuoteMailboxListHTML(value string) string {
	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		return html.EscapeString(strings.TrimSpace(value))
	}
	mailboxes := make([]string, len(addrs))
	for i, addr := range addrs {
		name, address := quoteMailboxParts(addr)
		mailboxes[i] = quoteMailboxHTML(name, address, plainName)
	}
	return strings.Join(mailboxes, ", ")
}

// quoteMailboxText renders one mailbox: "Name <email>", or "<email>" when name
// is empty.
func quoteMailboxText(name, address string) string {
	if name == "" {
		return "<" + address + ">"
	}
	return name + " <" + address + ">"
}

// nameStyle says how quoteMailboxHTML sets a display name. Gmail Web bolds
// the sender's name in a forward's header block, with the gmail_sendername
// class, but not in a reply's attribution line. The class matters: when Gmail
// rebuilds the text part of a message sent through the API, it turns bold into
// *asterisks* but leaves a gmail_sendername name as it is. (Only the delivered
// copy shows this; the Sent copy keeps the text part gog wrote.)
type nameStyle bool

const (
	plainName nameStyle = false
	boldName  nameStyle = true
)

// quoteMailboxHTML is quoteMailboxText for HTML: the name escaped and bold or
// not, per style, the address linked via mailto:.
func quoteMailboxHTML(name, address string, style nameStyle) string {
	link := `&lt;<a href="` + html.EscapeString(mailtoURI(address)) + `">` + html.EscapeString(address) + `</a>&gt;`
	if name == "" {
		return link
	}
	name = html.EscapeString(name)
	if style == boldName {
		name = `<strong class="gmail_sendername">` + name + "</strong>"
	}
	return name + " " + link
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
