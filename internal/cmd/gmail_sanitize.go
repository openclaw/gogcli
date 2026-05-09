package cmd

import (
	htmlpkg "html"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"google.golang.org/api/gmail/v1"
)

var (
	sanitizeURLPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\]\)]+`)
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

func sanitizeGmailBody(body string, isHTML bool) string {
	if body == "" {
		return ""
	}
	text := body
	if isHTML {
		text = extractSanitizedHTMLText(text)
	}
	text = sanitizeGmailText(text)
	text = whitespacePattern.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func extractSanitizedHTMLText(value string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(value))
	var out strings.Builder
	skipDepth := 0
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			text := whitespacePattern.ReplaceAllString(out.String(), " ")
			return strings.TrimSpace(text)
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "script" || tag == "style" {
				skipDepth++
			}
			if sanitizeBlockTags[tag] {
				out.WriteByte(' ')
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if (tag == "script" || tag == "style") && skipDepth > 0 {
				skipDepth--
			}
			if sanitizeBlockTags[tag] {
				out.WriteByte(' ')
			}
		case html.TextToken:
			if skipDepth == 0 {
				out.Write(tokenizer.Text())
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

func sanitizedGmailMessage(msg *gmail.Message, includeBody bool) gmailSanitizedMessageOutput {
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
		Attachments:  attachmentOutputs(collectAttachments(msg.Payload)),
	}
	if includeBody {
		body, isHTML := bestBodyForDisplay(msg.Payload)
		out.Body = sanitizeGmailBody(body, isHTML)
	}
	return out
}

func sanitizedGmailThread(thread *gmail.Thread, includeBody bool) gmailSanitizedThreadOutput {
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
		out.Messages = append(out.Messages, sanitizedGmailMessage(msg, includeBody))
	}
	return out
}
