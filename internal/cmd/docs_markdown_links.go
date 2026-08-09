package cmd

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/docsmarkdown"
)

func markdownMayContainHeadingLinks(markdown string) bool {
	return strings.Contains(markdown, "](#")
}

type markdownHeadingTarget struct {
	headingID string
	tabID     string
}

type markdownHeadingMatchKey struct {
	text       string
	occurrence int
}

type markdownParagraphRef struct {
	paragraph  *docs.Paragraph
	startIndex int64
	endIndex   int64
}

func rewriteMarkdownHeadingLinks(ctx context.Context, svc *docs.Service, docID string, tabID string, explicitAnchors []docsmarkdown.ExplicitHeadingAnchor) (int, error) {
	return rewriteMarkdownHeadingLinksFromIndex(ctx, svc, docID, tabID, explicitAnchors, 0)
}

func rewriteMarkdownHeadingLinksFromIndex(ctx context.Context, svc *docs.Service, docID string, tabID string, explicitAnchors []docsmarkdown.ExplicitHeadingAnchor, minIndex int64) (int, error) {
	return rewriteMarkdownHeadingLinksInRange(ctx, svc, docID, tabID, explicitAnchors, minIndex, 0)
}

func rewriteMarkdownHeadingLinksInRange(ctx context.Context, svc *docs.Service, docID string, tabID string, explicitAnchors []docsmarkdown.ExplicitHeadingAnchor, minIndex int64, maxIndex int64) (int, error) {
	getCall := svc.Documents.Get(docID).Context(ctx)
	if tabID != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Do()
	if err != nil {
		return 0, err
	}

	content, resolvedTabID, err := markdownHeadingLinkContent(doc, tabID)
	if err != nil {
		return 0, err
	}
	if len(content) == 0 {
		return 0, nil
	}
	paragraphs, autoHeadingBySlug, explicitHeadingBySlug := markdownHeadingLinkTargets(content, resolvedTabID, explicitAnchors, minIndex, maxIndex)
	if len(autoHeadingBySlug) == 0 && len(explicitHeadingBySlug) == 0 {
		return 0, nil
	}

	var requests []*docs.Request
	for _, ref := range paragraphs {
		if ref.paragraph == nil {
			continue
		}
		for _, pe := range ref.paragraph.Elements {
			if pe == nil || pe.TextRun == nil || pe.TextRun.TextStyle == nil || pe.TextRun.TextStyle.Link == nil {
				continue
			}
			if minIndex > 0 && pe.StartIndex < minIndex {
				continue
			}
			if maxIndex > 0 && pe.StartIndex >= maxIndex {
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
			target, ok := explicitHeadingBySlug[strings.TrimSpace(slug)]
			if !ok {
				target, ok = autoHeadingBySlug[strings.TrimSpace(slug)]
			}
			if !ok || target.headingID == "" {
				continue
			}
			rng := &docs.Range{
				StartIndex: pe.StartIndex,
				EndIndex:   pe.EndIndex,
				TabId:      resolvedTabID,
			}
			linkTarget := &docs.Link{HeadingId: target.headingID}
			if target.tabID != "" {
				linkTarget = &docs.Link{Heading: &docs.HeadingLink{Id: target.headingID, TabId: target.tabID}}
			}
			requests = append(requests, &docs.Request{
				UpdateTextStyle: &docs.UpdateTextStyleRequest{
					Range:     rng,
					TextStyle: &docs.TextStyle{Link: linkTarget},
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

func markdownHeadingLinkTargets(content []*docs.StructuralElement, resolvedTabID string, explicitAnchors []docsmarkdown.ExplicitHeadingAnchor, minIndex int64, maxIndex int64) ([]markdownParagraphRef, map[string]markdownHeadingTarget, map[string]markdownHeadingTarget) {
	paragraphs := markdownParagraphsInContent(content, minIndex)
	autoHeadingBySlug := map[string]markdownHeadingTarget{}
	explicitHeadingBySlug := map[string]markdownHeadingTarget{}
	explicitHeadingByKey := map[markdownHeadingMatchKey]string{}
	for _, explicit := range explicitAnchors {
		anchor := strings.TrimSpace(explicit.Anchor)
		text := docsmarkdown.HeadingNormalizedText(explicit.Text)
		if anchor == "" || text == "" || explicit.Occurrence <= 0 {
			continue
		}
		explicitHeadingByKey[markdownHeadingMatchKey{
			text:       text,
			occurrence: explicit.Occurrence,
		}] = anchor
	}
	slugCounts := map[string]int{}
	usedHeadingSlugs := map[string]bool{}
	headingTextCounts := map[string]int{}
	for _, ref := range paragraphs {
		if ref.paragraph == nil || ref.paragraph.ParagraphStyle == nil {
			continue
		}
		if minIndex > 0 && ref.startIndex < minIndex {
			continue
		}
		if maxIndex > 0 && ref.startIndex >= maxIndex {
			continue
		}
		style := ref.paragraph.ParagraphStyle
		if !strings.HasPrefix(style.NamedStyleType, "HEADING_") || strings.TrimSpace(style.HeadingId) == "" {
			continue
		}
		text := markdownHeadingParagraphText(ref.paragraph)
		target := markdownHeadingTarget{headingID: style.HeadingId, tabID: resolvedTabID}
		matchText := docsmarkdown.HeadingNormalizedText(text)
		headingTextCounts[matchText]++
		explicit := explicitHeadingByKey[markdownHeadingMatchKey{
			text:       matchText,
			occurrence: headingTextCounts[matchText],
		}]
		if explicit != "" {
			explicitHeadingBySlug[explicit] = target
			usedHeadingSlugs[explicit] = true
		} else if slug := markdownHeadingSlug(text, slugCounts, usedHeadingSlugs); slug != "" {
			autoHeadingBySlug[slug] = target
		}
	}
	return paragraphs, autoHeadingBySlug, explicitHeadingBySlug
}

func markdownHeadingLinkContent(doc *docs.Document, tabID string) ([]*docs.StructuralElement, string, error) {
	if doc == nil {
		return nil, "", nil
	}
	if tabID == "" {
		if doc.Body == nil {
			return nil, "", nil
		}
		return doc.Body.Content, "", nil
	}
	tab, err := findTab(flattenTabs(doc.Tabs), tabID)
	if err != nil {
		return nil, "", err
	}
	if tab == nil || tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
		return nil, "", nil
	}
	resolvedTabID := tabID
	if tab.TabProperties != nil && strings.TrimSpace(tab.TabProperties.TabId) != "" {
		resolvedTabID = tab.TabProperties.TabId
	}
	return tab.DocumentTab.Body.Content, resolvedTabID, nil
}

func markdownParagraphsInContent(content []*docs.StructuralElement, minIndex int64) []markdownParagraphRef {
	var paragraphs []markdownParagraphRef
	appendMarkdownParagraphs(&paragraphs, content, minIndex)
	return paragraphs
}

func appendMarkdownParagraphs(paragraphs *[]markdownParagraphRef, content []*docs.StructuralElement, minIndex int64) {
	for _, el := range content {
		if el == nil {
			continue
		}
		if el.Paragraph != nil {
			if minIndex == 0 || el.EndIndex > minIndex {
				*paragraphs = append(*paragraphs, markdownParagraphRef{
					paragraph:  el.Paragraph,
					startIndex: el.StartIndex,
					endIndex:   el.EndIndex,
				})
			}
		}
		if el.Table == nil {
			continue
		}
		for _, row := range el.Table.TableRows {
			if row == nil {
				continue
			}
			for _, cell := range row.TableCells {
				if cell != nil {
					appendMarkdownParagraphs(paragraphs, cell.Content, minIndex)
				}
			}
		}
	}
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

func markdownHeadingSlug(text string, seen map[string]int, used map[string]bool) string {
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
	for {
		candidate := slug
		if n > 0 {
			candidate = slug + "-" + strconv.Itoa(n)
		}
		seen[slug] = n + 1
		n++
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}
