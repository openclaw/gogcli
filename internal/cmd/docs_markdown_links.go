package cmd

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"google.golang.org/api/docs/v1"
)

func markdownMayContainHeadingLinks(markdown string) bool {
	return strings.Contains(markdown, "](#")
}

func rewriteMarkdownHeadingLinks(ctx context.Context, svc *docs.Service, docID string) (int, error) {
	doc, err := svc.Documents.Get(docID).
		Fields("body/content(startIndex,endIndex,paragraph(paragraphStyle(namedStyleType,headingId),elements(startIndex,endIndex,textRun(content,textStyle/link))))").
		Context(ctx).
		Do()
	if err != nil {
		return 0, err
	}
	if doc == nil || doc.Body == nil {
		return 0, nil
	}

	headingBySlug := map[string]string{}
	slugCounts := map[string]int{}
	for _, el := range doc.Body.Content {
		if el == nil || el.Paragraph == nil || el.Paragraph.ParagraphStyle == nil {
			continue
		}
		style := el.Paragraph.ParagraphStyle
		if !strings.HasPrefix(style.NamedStyleType, "HEADING_") || strings.TrimSpace(style.HeadingId) == "" {
			continue
		}
		text := markdownHeadingParagraphText(el.Paragraph)
		slug := markdownHeadingSlug(text, slugCounts)
		if slug == "" {
			continue
		}
		headingBySlug[slug] = style.HeadingId
	}
	if len(headingBySlug) == 0 {
		return 0, nil
	}

	var requests []*docs.Request
	for _, el := range doc.Body.Content {
		if el == nil || el.Paragraph == nil {
			continue
		}
		for _, pe := range el.Paragraph.Elements {
			if pe == nil || pe.TextRun == nil || pe.TextRun.TextStyle == nil || pe.TextRun.TextStyle.Link == nil {
				continue
			}
			link := pe.TextRun.TextStyle.Link
			if link.Url == "" || strings.HasPrefix(link.Url, "#heading=") {
				continue
			}
			slug, ok := strings.CutPrefix(link.Url, "#")
			if !ok || strings.TrimSpace(slug) == "" {
				continue
			}
			headingID := headingBySlug[strings.TrimSpace(slug)]
			if headingID == "" {
				continue
			}
			requests = append(requests, &docs.Request{
				UpdateTextStyle: &docs.UpdateTextStyleRequest{
					Range: &docs.Range{
						StartIndex: pe.StartIndex,
						EndIndex:   pe.EndIndex,
					},
					TextStyle: &docs.TextStyle{Link: &docs.Link{HeadingId: headingID}},
					Fields:    "link",
				},
			})
		}
	}
	if len(requests) == 0 {
		return 0, nil
	}
	_, err = svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{Requests: requests}).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	return len(requests), nil
}

func markdownHeadingParagraphText(p *docs.Paragraph) string {
	var b strings.Builder
	for _, el := range p.Elements {
		if el != nil && el.TextRun != nil {
			b.WriteString(el.TextRun.Content)
		}
	}
	return strings.TrimSpace(b.String())
}

func markdownHeadingSlug(text string, seen map[string]int) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastHyphen := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || r == '-':
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return ""
	}
	n := seen[slug]
	seen[slug] = n + 1
	if n == 0 {
		return slug
	}
	return slug + "-" + strconv.Itoa(n)
}
