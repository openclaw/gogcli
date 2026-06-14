//nolint:wsl_v5 // Recursive projection keeps source-order traversal stages adjacent.
package docssed

import (
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"
)

// DocumentProjection is the sed-facing view of a Google Docs document.
type DocumentProjection struct {
	RevisionID string
	Legacy     *DocumentSegment
	Tabs       []DocumentSegment
}

// DocumentSegment is one independently indexed document body.
type DocumentSegment struct {
	TabID          string
	Title          string
	BodyStartIndex int64
	BodyEndIndex   int64
	Blocks         []DocumentBlock
	TextRuns       []DocumentTextRun
	Paragraphs     []DocumentParagraph
	Tables         []DocumentTable
	Images         []DocumentImage
}

// DocumentBlockKind identifies one top-level structural element.
type DocumentBlockKind string

const (
	DocumentBlockParagraph       DocumentBlockKind = "paragraph"
	DocumentBlockTable           DocumentBlockKind = "table"
	DocumentBlockTableOfContents DocumentBlockKind = "table_of_contents"
	DocumentBlockSectionBreak    DocumentBlockKind = "section_break"
)

// DocumentBlock preserves top-level source order. ItemIndex addresses the
// corresponding Paragraphs or Tables entry, and is -1 for other block kinds.
type DocumentBlock struct {
	Kind       DocumentBlockKind
	StartIndex int64
	EndIndex   int64
	ItemIndex  int
}

// DocumentTextRun is one indexed text run.
type DocumentTextRun struct {
	Text       string
	StartIndex int64
	EndIndex   int64
}

// DocumentParagraph is one paragraph with its concatenated text.
type DocumentParagraph struct {
	Text               string
	StartIndex         int64
	EndIndex           int64
	NamedStyle         string
	BulletListID       string
	BulletNestingLevel int64
	LeadingTab         bool
}

// DocumentTable is one table in source order, including nested tables.
type DocumentTable struct {
	StartIndex      int64
	EndIndex        int64
	DeclaredRows    int64
	DeclaredColumns int64
	Rows            []DocumentTableRow
}

// DocumentTableRow contains projected cells in source order.
type DocumentTableRow struct {
	StartIndex int64
	EndIndex   int64
	Cells      []DocumentTableCell
}

// DocumentTableCell is one cell's direct paragraph text and indexed range.
type DocumentTableCell struct {
	Text           string
	StartIndex     int64
	EndIndex       int64
	TextStartIndex int64
	TextEndIndex   int64
	RowSpan        int64
	ColumnSpan     int64
}

// DocumentImage describes an inline or positioned image.
type DocumentImage struct {
	ObjectID     string
	Index        int64
	Alt          string
	IsPositioned bool
}

// ProjectDocument builds a stable traversal view from legacy and tab-aware Docs fields.
func ProjectDocument(document *docs.Document) DocumentProjection {
	if document == nil {
		return DocumentProjection{}
	}

	projection := DocumentProjection{RevisionID: document.RevisionId}
	if document.Body != nil || len(document.InlineObjects) > 0 || len(document.PositionedObjects) > 0 {
		legacy := projectDocumentSegment(
			"",
			"",
			document.Body,
			document.InlineObjects,
			document.PositionedObjects,
		)
		projection.Legacy = &legacy
	}
	projectTabs(document.Tabs, &projection.Tabs)
	return projection
}

func projectTabs(tabs []*docs.Tab, projected *[]DocumentSegment) {
	for _, tab := range tabs {
		if tab == nil {
			continue
		}
		tabID := ""
		title := ""
		if tab.TabProperties != nil {
			tabID = tab.TabProperties.TabId
			title = tab.TabProperties.Title
		}
		var body *docs.Body
		var inlineObjects map[string]docs.InlineObject
		var positionedObjects map[string]docs.PositionedObject
		if tab.DocumentTab != nil {
			body = tab.DocumentTab.Body
			inlineObjects = tab.DocumentTab.InlineObjects
			positionedObjects = tab.DocumentTab.PositionedObjects
		}
		*projected = append(*projected, projectDocumentSegment(
			tabID,
			title,
			body,
			inlineObjects,
			positionedObjects,
		))
		projectTabs(tab.ChildTabs, projected)
	}
}

func projectDocumentSegment(
	tabID string,
	title string,
	body *docs.Body,
	inlineObjects map[string]docs.InlineObject,
	positionedObjects map[string]docs.PositionedObject,
) DocumentSegment {
	segment := DocumentSegment{TabID: tabID, Title: title}
	projector := documentProjector{
		segment:           &segment,
		inlineObjects:     inlineObjects,
		positionedObjects: positionedObjects,
		seenPositioned:    make(map[string]bool, len(positionedObjects)),
	}
	if body != nil {
		segment.BodyStartIndex, segment.BodyEndIndex = documentBodyRange(body.Content)
		projector.walkContent(body.Content, true)
	}
	projector.appendUnanchoredPositionedImages()
	return segment
}

type documentProjector struct {
	segment           *DocumentSegment
	inlineObjects     map[string]docs.InlineObject
	positionedObjects map[string]docs.PositionedObject
	seenPositioned    map[string]bool
}

func (p *documentProjector) walkContent(content []*docs.StructuralElement, topLevel bool) {
	for _, element := range content {
		if element == nil {
			continue
		}
		if element.Paragraph != nil {
			paragraphIndex := len(p.segment.Paragraphs)
			p.projectParagraph(element)
			if topLevel {
				p.appendBlock(DocumentBlockParagraph, element, paragraphIndex)
			}
		}
		if element.Table != nil {
			tableIndex := len(p.segment.Tables)
			p.segment.Tables = append(p.segment.Tables, projectTable(element))
			if topLevel {
				p.appendBlock(DocumentBlockTable, element, tableIndex)
			}
			for _, row := range element.Table.TableRows {
				if row == nil {
					continue
				}
				for _, cell := range row.TableCells {
					if cell != nil {
						p.walkContent(cell.Content, false)
					}
				}
			}
		}
		if topLevel && element.TableOfContents != nil {
			p.appendBlock(DocumentBlockTableOfContents, element, -1)
		}
		if topLevel && element.SectionBreak != nil {
			p.appendBlock(DocumentBlockSectionBreak, element, -1)
		}
	}
}

func (p *documentProjector) appendBlock(
	kind DocumentBlockKind,
	element *docs.StructuralElement,
	itemIndex int,
) {
	p.segment.Blocks = append(p.segment.Blocks, DocumentBlock{
		Kind:       kind,
		StartIndex: element.StartIndex,
		EndIndex:   element.EndIndex,
		ItemIndex:  itemIndex,
	})
}

func (p *documentProjector) projectParagraph(element *docs.StructuralElement) {
	var text strings.Builder
	for _, objectID := range element.Paragraph.PositionedObjectIds {
		if p.seenPositioned[objectID] {
			continue
		}
		object, ok := p.positionedObjects[objectID]
		if !ok {
			continue
		}
		p.segment.Images = append(p.segment.Images, DocumentImage{
			ObjectID:     objectID,
			Index:        element.StartIndex,
			Alt:          positionedObjectAlt(object),
			IsPositioned: true,
		})
		p.seenPositioned[objectID] = true
	}
	for _, paragraphElement := range element.Paragraph.Elements {
		if paragraphElement == nil {
			continue
		}
		if paragraphElement.TextRun != nil {
			runText := paragraphElement.TextRun.Content
			text.WriteString(runText)
			p.segment.TextRuns = append(p.segment.TextRuns, DocumentTextRun{
				Text:       runText,
				StartIndex: paragraphElement.StartIndex,
				EndIndex:   paragraphElement.EndIndex,
			})
		}
		if paragraphElement.InlineObjectElement != nil {
			objectID := paragraphElement.InlineObjectElement.InlineObjectId
			p.segment.Images = append(p.segment.Images, DocumentImage{
				ObjectID: objectID,
				Index:    paragraphElement.StartIndex,
				Alt:      inlineObjectAlt(p.inlineObjects[objectID]),
			})
		}
	}

	paragraphText := text.String()
	projected := DocumentParagraph{
		Text:       paragraphText,
		StartIndex: element.StartIndex,
		EndIndex:   element.EndIndex,
		LeadingTab: strings.HasPrefix(paragraphText, "\t"),
	}
	if element.Paragraph.ParagraphStyle != nil {
		projected.NamedStyle = element.Paragraph.ParagraphStyle.NamedStyleType
	}
	if element.Paragraph.Bullet != nil {
		projected.BulletListID = element.Paragraph.Bullet.ListId
		projected.BulletNestingLevel = element.Paragraph.Bullet.NestingLevel
	}
	p.segment.Paragraphs = append(p.segment.Paragraphs, projected)
}

func projectTable(element *docs.StructuralElement) DocumentTable {
	table := DocumentTable{
		StartIndex:      element.StartIndex,
		EndIndex:        element.EndIndex,
		DeclaredRows:    element.Table.Rows,
		DeclaredColumns: element.Table.Columns,
	}
	for _, sourceRow := range element.Table.TableRows {
		row := DocumentTableRow{}
		if sourceRow != nil {
			row.StartIndex = sourceRow.StartIndex
			row.EndIndex = sourceRow.EndIndex
			for _, sourceCell := range sourceRow.TableCells {
				row.Cells = append(row.Cells, projectTableCell(sourceCell))
			}
		}
		table.Rows = append(table.Rows, row)
	}
	return table
}

func projectTableCell(cell *docs.TableCell) DocumentTableCell {
	if cell == nil {
		return DocumentTableCell{}
	}
	var text strings.Builder
	var textStartIndex int64
	var textEndIndex int64
	var foundText bool
	for _, element := range cell.Content {
		if element == nil || element.Paragraph == nil {
			continue
		}
		for _, paragraphElement := range element.Paragraph.Elements {
			if paragraphElement == nil || paragraphElement.TextRun == nil {
				continue
			}
			text.WriteString(paragraphElement.TextRun.Content)
			if !foundText {
				textStartIndex = paragraphElement.StartIndex
				foundText = true
			}
			textEndIndex = paragraphElement.EndIndex
		}
	}
	projected := DocumentTableCell{
		Text:           text.String(),
		StartIndex:     cell.StartIndex,
		EndIndex:       cell.EndIndex,
		TextStartIndex: textStartIndex,
		TextEndIndex:   textEndIndex,
	}
	if cell.TableCellStyle != nil {
		projected.RowSpan = cell.TableCellStyle.RowSpan
		projected.ColumnSpan = cell.TableCellStyle.ColumnSpan
	}
	return projected
}

func (p *documentProjector) appendUnanchoredPositionedImages() {
	ids := make([]string, 0, len(p.positionedObjects))
	for objectID := range p.positionedObjects {
		if p.seenPositioned[objectID] {
			continue
		}
		ids = append(ids, objectID)
	}
	sort.Strings(ids)
	for _, objectID := range ids {
		p.segment.Images = append(p.segment.Images, DocumentImage{
			ObjectID:     objectID,
			Alt:          positionedObjectAlt(p.positionedObjects[objectID]),
			IsPositioned: true,
		})
	}
}

func documentBodyRange(content []*docs.StructuralElement) (int64, int64) {
	var startIndex int64
	var endIndex int64
	var found bool
	for _, element := range content {
		if element == nil {
			continue
		}
		if !found {
			startIndex = element.StartIndex
			found = true
		}
		if element.EndIndex > endIndex {
			endIndex = element.EndIndex
		}
	}
	return startIndex, endIndex
}

func inlineObjectAlt(object docs.InlineObject) string {
	if object.InlineObjectProperties == nil {
		return ""
	}
	return embeddedObjectAlt(object.InlineObjectProperties.EmbeddedObject)
}

func positionedObjectAlt(object docs.PositionedObject) string {
	if object.PositionedObjectProperties == nil {
		return ""
	}
	return embeddedObjectAlt(object.PositionedObjectProperties.EmbeddedObject)
}

func embeddedObjectAlt(object *docs.EmbeddedObject) string {
	if object == nil {
		return ""
	}
	if object.Title != "" {
		return object.Title
	}
	return object.Description
}
