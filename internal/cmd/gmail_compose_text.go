package cmd

import (
	stdhtml "html"
	"slices"
	"strings"

	nethtml "golang.org/x/net/html"

	"github.com/openclaw/gogcli/internal/gmailcontent"
)

// Conversions between plain text and HTML: the author's text, which is marked
// with its direction, and text taken from the original message.

// composeText is the user's text above a quote or a forwarded message (a quoted
// reply's body, which by then includes any signature, or a forward's note) in
// whichever forms were given, any HTML already marked with its direction (see
// directedHTML). A form that was given comes back as given; a missing one is
// filled in from the other and trimmed. Whitespace-only counts as missing.
type composeText struct {
	plain string
	html  string
}

// hasHTML reports whether HTML was given.
func (t composeText) hasHTML() bool {
	return strings.TrimSpace(t.html) != ""
}

// plainVersion returns the plain text, derived from the HTML when only HTML
// was given.
func (t composeText) plainVersion() string {
	if strings.TrimSpace(t.plain) == "" && t.hasHTML() {
		return htmlToPlainText(t.html)
	}
	return t.plain
}

// htmlVersion returns the HTML, or the plain text through directedTextToHTML
// when only plain text was given.
func (t composeText) htmlVersion() string {
	if !t.hasHTML() {
		return directedTextToHTML(strings.TrimSpace(t.plain))
	}
	return t.html
}

// htmlOnlyWithoutText reports whether the text was given only as HTML with no
// plain-text rendering, such as a lone image, so a plain part could not carry it.
func (t composeText) htmlOnlyWithoutText() bool {
	return t.hasHTML() && strings.TrimSpace(t.plainVersion()) == ""
}

// byteOrderMark is the character some Windows editors start a UTF-8 file with.
const byteOrderMark = "\ufeff"

// directedTextToHTML renders plain text the author wrote as HTML the way
// Gmail's mobile apps write it: each line in its own <div dir="auto">, a blank
// line as <div dir="auto"><br></div>, one div to a line of HTML. dir="auto"
// gives each line the direction of its first letter, so a Hebrew or Arabic line
// reads right to left among English ones, and an English line keeps its
// direction inside a right-to-left reply that quotes it.
func directedTextToHTML(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, line := range lines {
		content := "<br>"
		if strings.TrimSpace(line) != "" {
			content = stdhtml.EscapeString(line)
		}
		lines[i] = `<div dir="auto">` + content + "</div>"
	}
	return strings.Join(lines, "\n")
}

// directedHTML wraps HTML the author gave in a <div dir="auto">, so it takes
// the direction of its first letter rather than the direction around it; dir
// attributes inside it still win. The HTML is trimmed, along with a byte order
// mark at its start. Empty or whitespace-only HTML comes back as given, and so
// does a document (see isHTMLDocument), since a div around it would move what
// belongs in its head into the body.
func directedHTML(htmlText string) string {
	if strings.TrimSpace(htmlText) == "" || isHTMLDocument(htmlText) {
		return htmlText
	}
	return `<div dir="auto">` + strings.TrimSpace(strings.TrimPrefix(htmlText, byteOrderMark)) + "</div>"
}

// isHTMLDocument reports whether htmlText is laid out as a whole HTML document
// rather than a fragment: the first thing in it, after a leading byte order
// mark, comments and whitespace, is a doctype, an <html>, <head> or <body> tag,
// or an element that belongs in a document's head, such as a <style>.
func isHTMLDocument(htmlText string) bool {
	z := nethtml.NewTokenizer(strings.NewReader(htmlText))
	for {
		switch z.Next() {
		case nethtml.CommentToken:
			continue
		case nethtml.TextToken:
			if strings.TrimSpace(strings.TrimPrefix(string(z.Text()), byteOrderMark)) == "" {
				continue
			}
			return false
		case nethtml.DoctypeToken:
			return true
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			name, _ := z.TagName()
			return slices.Contains([]string{"html", "head", "body", "base", "link", "meta", "style", "title"}, string(name))
		default:
			return false
		}
	}
}

// breakAfter returns the <br> that follows the author's HTML, before a quote or
// a forwarded message: one after HTML gog marked, which ends in a block (a div
// per line, the div around given HTML, or the signature's), leaving a blank
// line as Gmail does; two after a document, which may end inline, or after
// nothing at all.
func breakAfter(htmlText string) string {
	if strings.TrimSpace(htmlText) == "" || isHTMLDocument(htmlText) {
		return "<br><br>"
	}
	return "<br>"
}

// escapeTextToHTML renders text from the original message as HTML, keeping
// its line breaks; the author's own text goes through directedTextToHTML.
func escapeTextToHTML(value string) string {
	value = stdhtml.EscapeString(value)
	return strings.ReplaceAll(value, "\n", "<br>\n")
}

func htmlToPlainText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	doc, err := nethtml.Parse(strings.NewReader(value))
	if err != nil {
		return gmailcontent.StripHTMLTags(value)
	}

	var out strings.Builder
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n == nil {
			return
		}
		switch n.Type {
		case nethtml.TextNode:
			out.WriteString(n.Data)
		case nethtml.ElementNode:
			if hiddenHTMLElement(n) {
				return
			}
			switch strings.ToLower(n.Data) {
			case "head", literalStyle, "script", "template", "noscript", literalTitle:
				return
			case "br":
				writeHTMLNewline(&out)
				return
			case "div", "p", "li":
				writeHTMLNewline(&out)
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walk(child)
				}
				writeHTMLNewline(&out)
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	lines := strings.Split(stdhtml.UnescapeString(out.String()), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, "\n")
}

func hiddenHTMLElement(n *nethtml.Node) bool {
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "hidden":
			return true
		case "aria-hidden":
			if strings.EqualFold(strings.TrimSpace(attr.Val), "true") {
				return true
			}
		case "type":
			if strings.EqualFold(n.Data, "input") && strings.EqualFold(strings.TrimSpace(attr.Val), "hidden") {
				return true
			}
		case literalStyle:
			if hiddenInlineStyle(attr.Val) {
				return true
			}
		}
	}
	return false
}

func hiddenInlineStyle(value string) bool {
	for declaration := range strings.SplitSeq(value, ";") {
		property, setting, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		property = strings.ToLower(strings.TrimSpace(property))
		setting = strings.ToLower(strings.TrimSpace(setting))
		switch property {
		case "display", "visibility":
			if strings.HasPrefix(setting, "none") || strings.HasPrefix(setting, "hidden") {
				return true
			}
		case "mso-hide":
			if strings.HasPrefix(setting, "all") {
				return true
			}
		}
	}
	return false
}

func writeHTMLNewline(out *strings.Builder) {
	if out.Len() == 0 {
		return
	}
	if !strings.HasSuffix(out.String(), "\n") {
		out.WriteByte('\n')
	}
}
