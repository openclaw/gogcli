package cmd

import (
	htmlpkg "html"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
)

var (
	sanitizeURLPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\]\)]+`)
	// A NUL-delimited anchor placeholder from extractSanitizedHTMLText, or a bare URL.
	sanitizeLinkPattern = regexp.MustCompile("\x00[^\x00]*\x00|" + `https?://[^\s<>"'` + "`" + `\]\)]+`)
	sanitizeWhitespace  = regexp.MustCompile(`\s+`)
	sanitizeBlockTags  = map[string]bool{
		"article": true, "blockquote": true, "br": true, "dd": true, "div": true,
		"dl": true, "dt": true, "footer": true, "h1": true, "h2": true,
		"h3": true, "h4": true, "h5": true, "h6": true, "header": true,
		"hr": true, "li": true, "ol": true, "p": true, "pre": true,
		"section": true, "table": true, "tr": true, "ul": true,
	}
)

type gmailSanitizedThreadOutput struct {
	ID       string                        `json:"id,omitempty"`
	Messages []gmailSanitizedMessageOutput `json:"messages"`
}

type gmailSanitizedMessageOutput struct {
	ID           string             `json:"id,omitempty"`
	ThreadID     string             `json:"threadId,omitempty"`
	LabelIDs     []string           `json:"labelIds,omitempty"`
	Snippet      string             `json:"snippet,omitempty"`
	InternalDate int64              `json:"internalDate,omitempty"`
	SizeEstimate int64              `json:"sizeEstimate,omitempty"`
	Headers      map[string]string  `json:"headers"`
	Body         string             `json:"body,omitempty"`
	Attachments  []attachmentOutput `json:"attachments,omitempty"`
}

func sanitizeGmailText(value string) string {
	value = htmlpkg.UnescapeString(value)
	return sanitizeURLPattern.ReplaceAllString(value, "[url removed]")
}

type gmailLink struct {
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

// linkCollector numbers the links a body conversion keeps out of the text. Indexes are
// assigned in first-seen order; registering an href again returns its existing index,
// so a repeated link shares one [link:N] reference.
type linkCollector struct {
	links []gmailLink
	byURL map[string]int
}

func newLinkCollector() *linkCollector {
	return &linkCollector{byURL: map[string]int{}}
}

func (c *linkCollector) add(url, text string) int {
	if idx, ok := c.byURL[url]; ok {
		return idx
	}
	idx := len(c.links)
	c.links = append(c.links, gmailLink{URL: url, Text: text})
	c.byURL[url] = idx
	return idx
}

func sanitizeGmailBody(body string, isHTML bool) string {
	text, _ := sanitizeGmailBodyLinks(body, isHTML)
	return text
}

// sanitizeGmailBodyLinks converts a body like sanitizeGmailBody and also returns the
// links behind the body's [link:N] markers, ordered by index. gmail link replays this
// on the same message to resolve a marker, so the conversion must stay deterministic.
func sanitizeGmailBodyLinks(body string, isHTML bool) (string, []gmailLink) {
	if body == "" {
		return "", nil
	}
	// NUL delimits the anchor placeholders below and cannot occur in tokenizer output
	// (HTML parsing replaces it); strip it from the input so a crafted body cannot
	// fabricate a placeholder.
	text := strings.ReplaceAll(body, "\x00", "")
	anchorTexts := map[string]string{}
	if isHTML {
		text = extractSanitizedHTMLText(text, anchorTexts)
	}
	text = htmlpkg.UnescapeString(text)
	// One left-to-right pass numbers anchor placeholders and bare URLs alike, so
	// indexes follow document order. A bare URL has no anchor text; its marker
	// replaces it.
	links := newLinkCollector()
	text = sanitizeLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		url, linkText := match, ""
		if strings.HasPrefix(match, "\x00") {
			url = strings.Trim(match, "\x00")
			linkText = anchorTexts[url]
		}
		return "[link:" + strconv.Itoa(links.add(url, linkText)) + "]"
	})
	text = sanitizeWhitespace.ReplaceAllString(text, " ")
	return strings.TrimSpace(text), links.links
}

func extractSanitizedHTMLText(value string, anchorTexts map[string]string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(value))
	var out strings.Builder
	skipDepth := 0
	// Href and visible text of the anchor currently open; the placeholder is emitted
	// at the anchor's close so its marker follows the link text. Alt text of images
	// inside the anchor stands in when the anchor has no visible text of its own.
	anchorHref := ""
	var anchorText strings.Builder
	var anchorAlt strings.Builder
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(out.String())
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "script" || tag == literalStyle {
				skipDepth++
			}
			if tag == "a" && hasAttr {
				anchorHref = ""
				anchorText.Reset()
				anchorAlt.Reset()
				for {
					key, val, more := tokenizer.TagAttr()
					if strings.EqualFold(string(key), "href") {
						href := string(val)
						if strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "mailto:") {
							anchorHref = href
						}
					}
					if !more {
						break
					}
				}
			}
			if tag == "img" && anchorHref != "" && hasAttr {
				for {
					key, val, more := tokenizer.TagAttr()
					if strings.EqualFold(string(key), "alt") {
						anchorAlt.WriteString(" " + string(val))
					}
					if !more {
						break
					}
				}
			}
			if sanitizeBlockTags[tag] {
				out.WriteByte(' ')
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if (tag == "script" || tag == literalStyle) && skipDepth > 0 {
				skipDepth--
			}
			if tag == "a" && anchorHref != "" {
				if skipDepth == 0 {
					out.WriteString(" \x00" + anchorHref + "\x00")
					text := strings.TrimSpace(sanitizeWhitespace.ReplaceAllString(anchorText.String(), " "))
					if text == "" {
						text = strings.TrimSpace(sanitizeWhitespace.ReplaceAllString(anchorAlt.String(), " "))
					}
					// The first site with text names the link; later sites of the
					// same href share its index and therefore its text.
					if anchorTexts[anchorHref] == "" {
						anchorTexts[anchorHref] = text
					}
				}
				anchorHref = ""
				anchorText.Reset()
				anchorAlt.Reset()
			}
			if sanitizeBlockTags[tag] {
				out.WriteByte(' ')
			}
		case html.TextToken:
			if skipDepth == 0 {
				text := tokenizer.Text()
				out.Write(text)
				if anchorHref != "" {
					anchorText.Write(text)
				}
			}
		}
	}
}

func sanitizedGmailHeaders(p *gmail.MessagePart) map[string]string {
	headers := map[string]string{
		"from":        sanitizeGmailText(headerValue(p, "From")),
		"to":          sanitizeGmailText(headerValue(p, "To")),
		"cc":          sanitizeGmailText(headerValue(p, "Cc")),
		"bcc":         sanitizeGmailText(headerValue(p, "Bcc")),
		"subject":     sanitizeGmailText(headerValue(p, "Subject")),
		"date":        sanitizeGmailText(headerValue(p, "Date")),
		"message_id":  sanitizeGmailText(headerValue(p, "Message-ID")),
		"in_reply_to": sanitizeGmailText(headerValue(p, "In-Reply-To")),
		"references":  sanitizeGmailText(headerValue(p, "References")),
	}
	for key, value := range headers {
		if value == "" {
			delete(headers, key)
		}
	}
	return headers
}

func sanitizedGmailMessage(msg *gmail.Message, includeBody bool, useIndexedAttachmentIDs bool) gmailSanitizedMessageOutput {
	if msg == nil {
		return gmailSanitizedMessageOutput{Headers: map[string]string{}}
	}
	out := gmailSanitizedMessageOutput{
		ID:           msg.Id,
		ThreadID:     msg.ThreadId,
		LabelIDs:     msg.LabelIds,
		Snippet:      sanitizeGmailText(msg.Snippet),
		InternalDate: msg.InternalDate,
		SizeEstimate: msg.SizeEstimate,
		Headers:      sanitizedGmailHeaders(msg.Payload),
		Attachments:  attachmentOutputs(collectAttachments(msg.Payload), useIndexedAttachmentIDs),
	}
	if includeBody {
		body, isHTML := gmailcontent.BestBodyForDisplay(msg.Payload)
		out.Body = sanitizeGmailBody(body, isHTML)
	}
	return out
}

func sanitizedGmailThread(thread *gmail.Thread, includeBody bool, useIndexedAttachmentIDs bool) gmailSanitizedThreadOutput {
	if thread == nil {
		return gmailSanitizedThreadOutput{Messages: []gmailSanitizedMessageOutput{}}
	}
	out := gmailSanitizedThreadOutput{
		ID:       thread.Id,
		Messages: make([]gmailSanitizedMessageOutput, 0, len(thread.Messages)),
	}
	for _, msg := range thread.Messages {
		if msg == nil {
			continue
		}
		out.Messages = append(out.Messages, sanitizedGmailMessage(msg, includeBody, useIndexedAttachmentIDs))
	}
	return out
}
