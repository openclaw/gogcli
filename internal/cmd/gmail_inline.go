package cmd

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/gmailcontent"
	"github.com/openclaw/gogcli/internal/mailmime"
)

var (
	cidSrcsetReferencePattern = regexp.MustCompile(`(?i)(?:^|,)\s*cid:([^"'[:space:]<>\),]+)`)
	cidCSSReferencePattern    = regexp.MustCompile(`(?i)url\(\s*['"]?cid:([^"'[:space:]<>\)]+)['"]?\s*\)`)
)

var cidURLAttributes = map[string]struct{}{
	"background": {},
	"data":       {},
	"href":       {},
	"poster":     {},
	"src":        {},
	"srcset":     {},
	"xlink:href": {},
}

func preserveReferencedInlineResources(ctx context.Context, svc *gmail.Service, messageID string, payload *gmail.MessagePart, htmlBody string) ([]mailmime.Attachment, error) {
	references := referencedContentIDs(htmlBody)
	if len(references) == 0 {
		return nil, nil
	}

	parts := indexMessagePartsByContentID(payload)
	out := make([]mailmime.Attachment, 0, len(references))
	for _, contentID := range references {
		part := parts[canonicalContentID(contentID)]
		if part == nil {
			return nil, fmt.Errorf("HTML references cid:%s but the message has no matching MIME part", contentID)
		}
		attachment, err := mailAttachmentFromMessagePart(ctx, svc, messageID, part)
		if err != nil {
			return nil, fmt.Errorf("load cid:%s: %w", contentID, err)
		}
		attachment.Inline = true
		attachment.ContentID = normalizeContentID(headerValue(part, "Content-ID"))
		attachment.ContentLocation = strings.TrimSpace(headerValue(part, "Content-Location"))
		out = append(out, attachment)
	}
	return out, nil
}

func preserveForwardMessageParts(ctx context.Context, svc *gmail.Service, messageID string, payload *gmail.MessagePart, htmlBody string, includeAttachments bool) ([]mailmime.Attachment, error) {
	inline, err := preserveReferencedInlineResources(ctx, svc, messageID, payload, htmlBody)
	if err != nil {
		return nil, err
	}
	if !includeAttachments {
		return inline, nil
	}

	inlineIDs := make(map[string]struct{}, len(inline))
	for _, attachment := range inline {
		inlineIDs[canonicalContentID(attachment.ContentID)] = struct{}{}
	}

	out := append([]mailmime.Attachment{}, inline...)
	var walk func(*gmail.MessagePart) error
	walk = func(part *gmail.MessagePart) error {
		if part == nil {
			return nil
		}
		contentID := canonicalContentID(headerValue(part, "Content-ID"))
		if _, ok := inlineIDs[contentID]; ok && contentID != "" {
			return nil
		}
		if isOrdinaryAttachmentPart(part) {
			attachment, loadErr := mailAttachmentFromMessagePart(ctx, svc, messageID, part)
			if loadErr != nil {
				return fmt.Errorf("load attachment %q: %w", part.Filename, loadErr)
			}
			out = append(out, attachment)
			return nil
		}
		for _, child := range part.Parts {
			if walkErr := walk(child); walkErr != nil {
				return walkErr
			}
		}
		return nil
	}
	if err := walk(payload); err != nil {
		return nil, err
	}
	return out, nil
}

func referencedContentIDs(htmlBody string) []string {
	doc, err := nethtml.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return nil
	}

	var candidates []string
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			for _, attr := range node.Attr {
				name := strings.ToLower(attr.Key)
				switch {
				case name == "style":
					candidates = append(candidates, contentIDsFromMatches(cidCSSReferencePattern.FindAllStringSubmatch(attr.Val, -1))...)
				case isCIDURLAttribute(attr):
					candidates = append(candidates, contentIDsFromURLAttribute(attr)...)
				}
			}
			if strings.EqualFold(node.Data, "style") {
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == nethtml.TextNode {
						candidates = append(candidates, contentIDsFromMatches(cidCSSReferencePattern.FindAllStringSubmatch(child.Data, -1))...)
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, contentID := range candidates {
		if decoded, decodeErr := url.PathUnescape(contentID); decodeErr == nil {
			contentID = decoded
		}
		contentID = normalizeContentID(contentID)
		key := canonicalContentID(contentID)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, contentID)
	}
	return out
}

func contentIDsFromMatches(matches [][]string) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		out = append(out, match[1])
	}
	return out
}

func contentIDsFromURLAttribute(attr nethtml.Attribute) []string {
	value := strings.TrimSpace(attr.Val)
	if cidURLAttributeName(attr) == "srcset" {
		return contentIDsFromMatches(cidSrcsetReferencePattern.FindAllStringSubmatch(value, -1))
	}
	if len(value) < len("cid:") || !strings.EqualFold(value[:len("cid:")], "cid:") {
		return nil
	}
	return []string{strings.TrimSpace(value[len("cid:"):])}
}

func isCIDURLAttribute(attr nethtml.Attribute) bool {
	_, ok := cidURLAttributes[cidURLAttributeName(attr)]
	return ok
}

func cidURLAttributeName(attr nethtml.Attribute) string {
	name := strings.ToLower(attr.Key)
	if attr.Namespace != "" {
		name = strings.ToLower(attr.Namespace) + ":" + name
	}
	return name
}

func indexMessagePartsByContentID(payload *gmail.MessagePart) map[string]*gmail.MessagePart {
	out := make(map[string]*gmail.MessagePart)
	var walk func(*gmail.MessagePart)
	walk = func(part *gmail.MessagePart) {
		if part == nil {
			return
		}
		if contentID := canonicalContentID(headerValue(part, "Content-ID")); contentID != "" {
			if _, exists := out[contentID]; !exists {
				out[contentID] = part
			}
		}
		for _, child := range part.Parts {
			walk(child)
		}
	}
	walk(payload)
	return out
}

func mailAttachmentFromMessagePart(ctx context.Context, svc *gmail.Service, messageID string, part *gmail.MessagePart) (mailmime.Attachment, error) {
	if part == nil || part.Body == nil {
		return mailmime.Attachment{}, fmt.Errorf("empty MIME part")
	}

	var (
		data []byte
		err  error
	)
	switch {
	case strings.TrimSpace(part.Body.AttachmentId) != "":
		data, err = fetchAttachmentBytes(ctx, svc, messageID, part.Body.AttachmentId)
	case part.Body.Data != "":
		data, err = gmailcontent.DecodeBase64URLBytes(part.Body.Data)
	case part.Body.Size == 0:
		data = []byte{}
	default:
		err = fmt.Errorf("MIME part has no body data")
	}
	if err != nil {
		return mailmime.Attachment{}, err
	}

	fallbackFilename := defaultAttachmentFilename
	if canonicalContentID(headerValue(part, "Content-ID")) != "" {
		fallbackFilename = "inline"
	}
	filename := sanitizeAttachmentFilename(part.Filename, fallbackFilename)
	return mailmime.Attachment{
		Filename: filename,
		MIMEType: strings.TrimSpace(part.MimeType),
		Data:     data,
		DataSet:  true,
	}, nil
}

func isOrdinaryAttachmentPart(part *gmail.MessagePart) bool {
	if part == nil || part.Body == nil {
		return false
	}
	if strings.TrimSpace(part.Body.AttachmentId) != "" {
		return true
	}
	if part.Body.Data == "" && part.Body.Size != 0 {
		return false
	}
	if strings.TrimSpace(part.Filename) != "" {
		return true
	}
	disposition, _, err := mime.ParseMediaType(headerValue(part, "Content-Disposition"))
	return err == nil && strings.EqualFold(disposition, "attachment")
}

func normalizeContentID(value string) string {
	return strings.Trim(strings.TrimSpace(value), "<>")
}

func canonicalContentID(value string) string {
	return strings.ToLower(normalizeContentID(value))
}
