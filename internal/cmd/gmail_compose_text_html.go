package cmd

import "strings"

// composeTextHTMLOptions holds --html-from-text for the commands that compose
// a message from --body or --body-file.
//
// Gmail web displays a text/plain-only message with its own line break about
// every 70 characters, even when each paragraph is one logical line in the
// MIME (#1058). Mail written in Gmail's composer always carries a text/html
// alternative, which reflows to the window. --html-from-text adds that
// alternative, built from the plain body, when no HTML body was given.
type composeTextHTMLOptions struct {
	//nolint:lll // flag help text
	HTMLFromText bool `name:"html-from-text" help:"When only a plain body is given, also send it as HTML so Gmail web reflows paragraphs instead of breaking lines about every 70 characters" env:"GOG_GMAIL_HTML_FROM_TEXT"`
}

// withTextHTML returns htmlBody unchanged unless --html-from-text is set, no
// HTML body was given, and the plain body has text. Then it returns the plain
// body as escaped HTML with each line's own text direction.
func (c composeTextHTMLOptions) withTextHTML(plainBody, htmlBody string) string {
	if !c.HTMLFromText || strings.TrimSpace(htmlBody) != "" || strings.TrimSpace(plainBody) == "" {
		return htmlBody
	}
	return directedTextToHTML(plainBody)
}
