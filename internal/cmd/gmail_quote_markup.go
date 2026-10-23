package cmd

import "html"

// Markup for the lines that introduce a quote: a reply's attribution and a
// forward's header block.

// gmailQuoteOpen opens a quote as Gmail Web does: the outer div with Gmail's
// quote classes, then the div that holds the line introducing the quote, a
// reply's attribution or a forward's header block. Callers close both. gog
// writes that line in English, so it is marked left-to-right; otherwise,
// inside right-to-left text such as a Hebrew reply quoting this message, its
// words and punctuation would reorder.
const gmailQuoteOpen = `<div class="gmail_quote gmail_quote_container"><div dir="ltr" class="gmail_attr">`

// isolateDirection wraps HTML taken from the original message, such as a
// linked address, in a span with dir="auto". The span gives the text the
// direction of its first strong character and isolates it from the line
// around it, so a right-to-left name can't pull a neighboring address or
// punctuation into its own order (next to a Hebrew name,
// "<0501234567@example.com>" renders as "0501234567> NAME@example.com>").
// Empty text stays empty, so formatQuoteAttribution still sees a missing
// sender or date.
func isolateDirection(htmlText string) string {
	if htmlText == "" {
		return ""
	}
	return `<span dir="auto">` + htmlText + "</span>"
}

// isolateTextDirection escapes text taken from the original message, such as
// a name, a subject or a date, for HTML and wraps it as isolateDirection does.
func isolateTextDirection(text string) string {
	return isolateDirection(html.EscapeString(text))
}
