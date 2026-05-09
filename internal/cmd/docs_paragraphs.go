package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"
)

// docParagraph represents a single numbered element in a Google Doc's structure.
type docParagraph struct {
	Num        int    `json:"num"`
	StartIndex int64  `json:"startIndex"`
	EndIndex   int64  `json:"endIndex"`
	Type       string `json:"type"`
	IsBullet   bool   `json:"bullet"`
	NestLevel  int    `json:"nestLevel,omitempty"`
	Text       string `json:"text"`
	ElemType   string `json:"elemType"` // "paragraph", "table", "toc", "sectionBreak"
	TableRows  int    `json:"tableRows,omitempty"`
	TableCols  int    `json:"tableCols,omitempty"`
}

// paragraphMap holds the structured view of a Google Doc's content.
type paragraphMap struct {
	DocumentID string         `json:"documentId"`
	RevisionID string         `json:"revisionId"`
	TabID      string         `json:"tab,omitempty"`
	Paragraphs []docParagraph `json:"paragraphs"`
}

// buildParagraphMap traverses the document body and numbers each paragraph
// and table sequentially (1-based). The initial SectionBreak at index 0 is
// skipped as it is not user-editable.
func buildParagraphMap(doc *docs.Document, tabID string) (*paragraphMap, error) {
	if doc == nil {
		return nil, fmt.Errorf("nil document")
	}

	var content []*docs.StructuralElement
	var revisionID string

	if tabID != "" && len(doc.Tabs) > 0 {
		tabs := flattenTabs(doc.Tabs)
		tab, tabErr := findTab(tabs, tabID)
		if tabErr != nil {
			return nil, tabErr
		}
		if tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
			return nil, fmt.Errorf("tab has no content: %s", tabID)
		}
		content = tab.DocumentTab.Body.Content
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
		}
	} else {
		if doc.Body == nil {
			return nil, fmt.Errorf("document has no body")
		}
		content = doc.Body.Content
	}
	revisionID = doc.RevisionId

	pm := &paragraphMap{
		DocumentID: doc.DocumentId,
		RevisionID: revisionID,
		TabID:      tabID,
	}

	num := 0
	for _, el := range content {
		if el == nil {
			continue
		}

		switch {
		case el.SectionBreak != nil:
			// Skip section breaks — not user-editable.
			continue

		case el.Paragraph != nil:
			num++
			dp := docParagraph{
				Num:        num,
				StartIndex: el.StartIndex,
				EndIndex:   el.EndIndex,
				ElemType:   "paragraph",
				Text:       paragraphText(el.Paragraph),
			}

			// Extract named style type.
			if el.Paragraph.ParagraphStyle != nil {
				dp.Type = el.Paragraph.ParagraphStyle.NamedStyleType
			}
			if dp.Type == "" {
				dp.Type = "NORMAL_TEXT"
			}

			// Extract bullet info.
			if el.Paragraph.Bullet != nil {
				dp.IsBullet = true
				dp.NestLevel = int(el.Paragraph.Bullet.NestingLevel)
			}

			pm.Paragraphs = append(pm.Paragraphs, dp)

		case el.Table != nil:
			num++
			rows := len(el.Table.TableRows)
			cols := 0
			if rows > 0 && len(el.Table.TableRows[0].TableCells) > 0 {
				cols = len(el.Table.TableRows[0].TableCells)
			}
			dp := docParagraph{
				Num:        num,
				StartIndex: el.StartIndex,
				EndIndex:   el.EndIndex,
				Type:       "TABLE",
				ElemType:   "table",
				Text:       tablePreviewText(el.Table),
				TableRows:  rows,
				TableCols:  cols,
			}
			pm.Paragraphs = append(pm.Paragraphs, dp)

		case el.TableOfContents != nil:
			num++
			dp := docParagraph{
				Num:        num,
				StartIndex: el.StartIndex,
				EndIndex:   el.EndIndex,
				Type:       "TABLE_OF_CONTENTS",
				ElemType:   "toc",
				Text:       "[table of contents]",
			}
			pm.Paragraphs = append(pm.Paragraphs, dp)
		}
	}

	return pm, nil
}

// paragraphText extracts the plain text from a Paragraph element.
func paragraphText(p *docs.Paragraph) string {
	if p == nil {
		return ""
	}
	var sb strings.Builder
	for _, elem := range p.Elements {
		if elem.TextRun != nil {
			sb.WriteString(elem.TextRun.Content)
		}
	}
	// Trim the trailing newline that Google Docs adds to every paragraph.
	return strings.TrimRight(sb.String(), "\n")
}

// get returns the paragraph at the given 1-based number.
func (pm *paragraphMap) get(num int) (*docParagraph, error) {
	if num < 1 || num > len(pm.Paragraphs) {
		return nil, fmt.Errorf("paragraph %d out of range (document has %d paragraphs)", num, len(pm.Paragraphs))
	}
	return &pm.Paragraphs[num-1], nil
}

// fetchAndBuildMap fetches the document and builds a paragraph map.
func fetchAndBuildMap(ctx context.Context, svc *docs.Service, docID, tabID string) (*paragraphMap, error) {
	getCall := svc.Documents.Get(docID)
	if tabID != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("doc not found")
	}

	return buildParagraphMap(doc, tabID)
}

// tablePreviewText returns a short preview of the table content.
func tablePreviewText(t *docs.Table) string {
	if t == nil || len(t.TableRows) == 0 {
		return "[empty table]"
	}
	// Show first row cells as a preview.
	var cells []string
	for _, cell := range t.TableRows[0].TableCells {
		var text strings.Builder
		for _, el := range cell.Content {
			if el.Paragraph != nil {
				text.WriteString(paragraphText(el.Paragraph))
			}
		}
		cells = append(cells, strings.TrimSpace(text.String()))
	}
	preview := strings.Join(cells, " | ")
	if len(preview) > 60 {
		preview = preview[:57] + "..."
	}
	return preview
}
