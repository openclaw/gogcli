package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
)

type DocsTablesCmd struct {
	List DocsTablesListCmd `cmd:"" name:"list" aliases:"ls" help:"List native tables in document order"`
}

type DocsImagesCmd struct {
	List DocsImagesListCmd `cmd:"" name:"list" aliases:"ls" help:"List inline and positioned images"`
}

type DocsHeadingsCmd struct {
	List DocsHeadingsListCmd `cmd:"" name:"list" aliases:"ls" help:"List heading paragraphs"`
}

type DocsParagraphsCmd struct {
	List DocsParagraphsListCmd `cmd:"" name:"list" aliases:"ls" help:"List paragraphs"`
}

type DocsTablesListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
}

type DocsImagesListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
}

type DocsHeadingsListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
	Level int    `name:"level" help:"Only return this heading level (1-6)"`
}

type DocsParagraphsListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
	Style string `name:"style" help:"Only return this named style (for example NORMAL_TEXT or HEADING_2)"`
}

type docsTableListItem struct {
	Index      int      `json:"index"`
	StartIndex int64    `json:"startIndex"`
	Rows       int      `json:"rows"`
	Columns    int      `json:"columns"`
	Header     []string `json:"header"`
}

type docsImageListItem struct {
	Index      int     `json:"index"`
	ObjectID   string  `json:"objectId"`
	StartIndex int64   `json:"startIndex,omitempty"`
	Alt        string  `json:"alt"`
	Positioned bool    `json:"positioned"`
	Width      float64 `json:"width,omitempty"`
	Height     float64 `json:"height,omitempty"`
	SizeUnit   string  `json:"sizeUnit,omitempty"`
}

type docsParagraphListItem struct {
	Index      int    `json:"index"`
	StartIndex int64  `json:"startIndex"`
	EndIndex   int64  `json:"endIndex"`
	Style      string `json:"style"`
	Text       string `json:"text"`
}

type docsParagraphInspectItem struct {
	Index      int                `json:"index"`
	StartIndex int64              `json:"startIndex"`
	EndIndex   int64              `json:"endIndex"`
	Style      string             `json:"style"`
	Text       string             `json:"text"`
	IsEmpty    bool               `json:"isEmpty"`
	Runs       []docsParagraphRun `json:"runs"`
}

type docsParagraphRun struct {
	StartIndex    int64                 `json:"startIndex"`
	EndIndex      int64                 `json:"endIndex"`
	Text          string                `json:"text"`
	Bold          bool                  `json:"bold"`
	Italic        bool                  `json:"italic"`
	Underline     bool                  `json:"underline"`
	Strikethrough bool                  `json:"strikethrough"`
	Link          *docsParagraphRunLink `json:"link"`
}

type docsParagraphRunLink struct {
	URL        string `json:"url,omitempty"`
	BookmarkID string `json:"bookmarkId,omitempty"`
	HeadingID  string `json:"headingId,omitempty"`
	TabID      string `json:"tabId,omitempty"`
}

func (c *DocsTablesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	doc, tabID, err := loadDocsEnumeratorDocument(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}
	tables := collectAllTablesWithIndex(doc)
	items := make([]docsTableListItem, 0, len(tables))
	for i, table := range tables {
		rows := int(table.table.Rows)
		if rows == 0 {
			rows = len(table.table.TableRows)
		}
		columns := int(table.table.Columns)
		var header []string
		if len(table.table.TableRows) > 0 {
			if columns == 0 {
				columns = len(table.table.TableRows[0].TableCells)
			}
			header = tableRowText(table.table.TableRows[0])
		}
		items = append(items, docsTableListItem{
			Index:      i + 1,
			StartIndex: table.startIdx,
			Rows:       rows,
			Columns:    columns,
			Header:     header,
		})
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": doc.DocumentId,
			"tabId":      tabID,
			"tables":     items,
		})
	}
	w, flush := tableWriter(ctx)
	defer flush()
	if !outfmt.IsPlain(ctx) {
		fmt.Fprintln(w, "#\tSTART\tROWS\tCOLS\tHEADER")
	}
	for _, item := range items {
		fmt.Fprintf(w, "%d\t%d\t%d\t%d\t%s\n",
			item.Index, item.StartIndex, item.Rows, item.Columns, docsTSVField(strings.Join(item.Header, " | ")))
	}
	return nil
}

func (c *DocsImagesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	doc, tabID, err := loadDocsEnumeratorDocument(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}
	items := enumerateDocsImages(doc)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": doc.DocumentId,
			"tabId":      tabID,
			"images":     items,
		})
	}
	w, flush := tableWriter(ctx)
	defer flush()
	if !outfmt.IsPlain(ctx) {
		fmt.Fprintln(w, "#\tOBJECT ID\tSTART\tPOSITIONED\tWIDTH\tHEIGHT\tUNIT\tALT")
	}
	for _, item := range items {
		fmt.Fprintf(w, "%d\t%s\t%d\t%t\t%s\t%s\t%s\t%s\n",
			item.Index,
			item.ObjectID,
			item.StartIndex,
			item.Positioned,
			formatOptionalFloat(item.Width),
			formatOptionalFloat(item.Height),
			item.SizeUnit,
			docsTSVField(item.Alt),
		)
	}
	return nil
}

func (c *DocsHeadingsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Level < 0 || c.Level > 6 {
		return usage("level must be between 1 and 6")
	}
	doc, tabID, err := loadDocsEnumeratorDocument(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}
	paragraphs := enumerateDocsParagraphs(doc)
	items := make([]docsParagraphListItem, 0)
	headingIndex := 0
	for _, paragraph := range paragraphs {
		level, ok := headingLevel(paragraph.Style)
		if !ok {
			continue
		}
		headingIndex++
		if c.Level > 0 && level != c.Level {
			continue
		}
		items = append(items, docsParagraphListItem{
			Index:      headingIndex,
			StartIndex: paragraph.StartIndex,
			EndIndex:   paragraph.EndIndex,
			Style:      paragraph.Style,
			Text:       paragraph.Text,
		})
	}
	return writeDocsParagraphEnumerator(ctx, doc.DocumentId, tabID, "headings", items, nil)
}

func (c *DocsParagraphsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	doc, tabID, err := loadDocsEnumeratorDocument(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}
	style := strings.ToUpper(strings.TrimSpace(c.Style))
	paragraphs := enumerateDocsParagraphs(doc)
	items := make([]docsParagraphListItem, 0)
	inspectItems := make([]docsParagraphInspectItem, 0)
	for i, paragraph := range paragraphs {
		if style != "" && paragraph.Style != style {
			continue
		}
		item := docsParagraphListItem{
			Index:      i + 1,
			StartIndex: paragraph.StartIndex,
			EndIndex:   paragraph.EndIndex,
			Style:      paragraph.Style,
			Text:       paragraph.Text,
		}
		items = append(items, item)
		inspectItems = append(inspectItems, docsParagraphInspectItem{
			Index:      item.Index,
			StartIndex: item.StartIndex,
			EndIndex:   item.EndIndex,
			Style:      item.Style,
			Text:       item.Text,
			IsEmpty:    paragraph.IsEmpty,
			Runs:       paragraph.Runs,
		})
	}
	return writeDocsParagraphEnumerator(ctx, doc.DocumentId, tabID, "paragraphs", items, inspectItems)
}

func loadDocsEnumeratorDocument(
	ctx context.Context,
	flags *RootFlags,
	docID string,
	tabQuery string,
) (*docs.Document, string, error) {
	id := normalizeGoogleID(strings.TrimSpace(docID))
	if id == "" {
		return nil, "", usage("empty docId")
	}
	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return nil, "", err
	}
	tabQuery = strings.TrimSpace(tabQuery)
	call := svc.Documents.Get(id).Context(ctx)
	if tabQuery != "" {
		call = call.IncludeTabsContent(true)
	}
	doc, err := call.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, "", fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return nil, "", err
	}
	if doc == nil {
		return nil, "", fmt.Errorf("doc not found")
	}
	if tabQuery == "" {
		return doc, "", nil
	}
	tab, err := findTab(flattenTabs(doc.Tabs), tabQuery)
	if err != nil {
		return nil, "", err
	}
	projected, err := projectRawDocumentTab(doc, tab)
	if err != nil {
		return nil, "", err
	}
	tabID := ""
	if tab.TabProperties != nil {
		tabID = tab.TabProperties.TabId
	}
	return projected, tabID, nil
}

func writeDocsParagraphEnumerator(
	ctx context.Context,
	documentID string,
	tabID string,
	key string,
	items []docsParagraphListItem,
	jsonItems any,
) error {
	if outfmt.IsJSON(ctx) {
		if jsonItems == nil {
			jsonItems = items
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"documentId": documentID,
			"tabId":      tabID,
			key:          jsonItems,
		})
	}
	w, flush := tableWriter(ctx)
	defer flush()
	if !outfmt.IsPlain(ctx) {
		fmt.Fprintln(w, "#\tSTART\tEND\tSTYLE\tTEXT")
	}
	for _, item := range items {
		fmt.Fprintf(w, "%d\t%d\t%d\t%s\t%s\n",
			item.Index, item.StartIndex, item.EndIndex, item.Style, docsTSVField(item.Text))
	}
	return nil
}

func tableRowText(row *docs.TableRow) []string {
	if row == nil {
		return nil
	}
	cells := make([]string, 0, len(row.TableCells))
	for _, cell := range row.TableCells {
		var text strings.Builder
		for _, element := range cell.Content {
			if element != nil && element.Paragraph != nil {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(paragraphText(element.Paragraph))
			}
		}
		cells = append(cells, strings.TrimSpace(text.String()))
	}
	return cells
}

func enumerateDocsImages(doc *docs.Document) []docsImageListItem {
	if doc == nil {
		return nil
	}
	inline := make(map[string]*docs.EmbeddedObject, len(doc.InlineObjects))
	for id, object := range doc.InlineObjects {
		if object.InlineObjectProperties != nil &&
			object.InlineObjectProperties.EmbeddedObject != nil &&
			object.InlineObjectProperties.EmbeddedObject.ImageProperties != nil {
			inline[id] = object.InlineObjectProperties.EmbeddedObject
		}
	}

	items := make([]docsImageListItem, 0, len(inline)+len(doc.PositionedObjects))
	seenPositioned := make(map[string]bool, len(doc.PositionedObjects))
	var walk func([]*docs.StructuralElement)
	walk = func(content []*docs.StructuralElement) {
		for _, element := range content {
			if element == nil {
				continue
			}
			if element.Paragraph != nil {
				for _, paragraphElement := range element.Paragraph.Elements {
					if paragraphElement == nil || paragraphElement.InlineObjectElement == nil {
						continue
					}
					id := paragraphElement.InlineObjectElement.InlineObjectId
					object, ok := inline[id]
					if !ok {
						continue
					}
					item := docsImageListItem{
						ObjectID:   id,
						StartIndex: paragraphElement.StartIndex,
					}
					applyEmbeddedObjectImage(&item, object)
					items = append(items, item)
				}
				for _, id := range element.Paragraph.PositionedObjectIds {
					object, ok := doc.PositionedObjects[id]
					if !ok || object.PositionedObjectProperties == nil ||
						object.PositionedObjectProperties.EmbeddedObject == nil ||
						object.PositionedObjectProperties.EmbeddedObject.ImageProperties == nil {
						continue
					}
					item := docsImageListItem{
						ObjectID:   id,
						StartIndex: element.StartIndex,
						Positioned: true,
					}
					applyEmbeddedObjectImage(&item, object.PositionedObjectProperties.EmbeddedObject)
					items = append(items, item)
					seenPositioned[id] = true
				}
			}
			if element.Table != nil {
				for _, row := range element.Table.TableRows {
					for _, cell := range row.TableCells {
						walk(cell.Content)
					}
				}
			}
		}
	}
	if doc.Body != nil {
		walk(doc.Body.Content)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].StartIndex < items[j].StartIndex
	})

	positionedIDs := make([]string, 0, len(doc.PositionedObjects))
	for id, object := range doc.PositionedObjects {
		if seenPositioned[id] ||
			object.PositionedObjectProperties == nil ||
			object.PositionedObjectProperties.EmbeddedObject == nil ||
			object.PositionedObjectProperties.EmbeddedObject.ImageProperties == nil {
			continue
		}
		positionedIDs = append(positionedIDs, id)
	}
	sort.Strings(positionedIDs)
	for _, id := range positionedIDs {
		object := doc.PositionedObjects[id]
		item := docsImageListItem{ObjectID: id, Positioned: true}
		if object.PositionedObjectProperties != nil {
			applyEmbeddedObjectImage(&item, object.PositionedObjectProperties.EmbeddedObject)
		}
		items = append(items, item)
	}
	for i := range items {
		items[i].Index = i + 1
	}
	return items
}

func applyEmbeddedObjectImage(item *docsImageListItem, object *docs.EmbeddedObject) {
	if item == nil || object == nil {
		return
	}
	item.Alt = strings.TrimSpace(strings.Join([]string{object.Title, object.Description}, " "))
	if object.Size == nil {
		return
	}
	if object.Size.Width != nil {
		item.Width = object.Size.Width.Magnitude
		item.SizeUnit = object.Size.Width.Unit
	}
	if object.Size.Height != nil {
		item.Height = object.Size.Height.Magnitude
		if item.SizeUnit == "" {
			item.SizeUnit = object.Size.Height.Unit
		}
	}
}

func headingLevel(style string) (int, bool) {
	const prefix = "HEADING_"
	if !strings.HasPrefix(style, prefix) {
		return 0, false
	}
	level, err := strconv.Atoi(strings.TrimPrefix(style, prefix))
	if err != nil || level < 1 || level > 6 {
		return 0, false
	}
	return level, true
}

func formatOptionalFloat(value float64) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

type docsEnumeratedParagraph struct {
	StartIndex int64
	EndIndex   int64
	Style      string
	Text       string
	IsEmpty    bool
	Runs       []docsParagraphRun
}

func enumerateDocsParagraphs(doc *docs.Document) []docsEnumeratedParagraph {
	if doc == nil || doc.Body == nil {
		return nil
	}
	var paragraphs []docsEnumeratedParagraph
	var walk func([]*docs.StructuralElement)
	walk = func(content []*docs.StructuralElement) {
		for _, element := range content {
			if element == nil {
				continue
			}
			if element.Paragraph != nil {
				style := docsNamedStyleNormalText
				if element.Paragraph.ParagraphStyle != nil &&
					element.Paragraph.ParagraphStyle.NamedStyleType != "" {
					style = element.Paragraph.ParagraphStyle.NamedStyleType
				}
				isEmpty, runs := inspectDocsParagraph(element.Paragraph)
				paragraphs = append(paragraphs, docsEnumeratedParagraph{
					StartIndex: element.StartIndex,
					EndIndex:   element.EndIndex,
					Style:      style,
					Text:       paragraphText(element.Paragraph),
					IsEmpty:    isEmpty,
					Runs:       runs,
				})
			}
			if element.Table != nil {
				for _, row := range element.Table.TableRows {
					for _, cell := range row.TableCells {
						walk(cell.Content)
					}
				}
			}
			if element.TableOfContents != nil {
				walk(element.TableOfContents.Content)
			}
		}
	}
	walk(doc.Body.Content)
	return paragraphs
}

func inspectDocsParagraph(paragraph *docs.Paragraph) (bool, []docsParagraphRun) {
	if paragraph == nil {
		return true, []docsParagraphRun{}
	}
	runs := make([]docsParagraphRun, 0, len(paragraph.Elements))
	hasContent := len(paragraph.PositionedObjectIds) > 0
	for _, element := range paragraph.Elements {
		if element == nil {
			continue
		}
		if element.TextRun == nil {
			hasContent = hasContent || docsParagraphElementHasNonTextContent(element)
			continue
		}

		textRun := element.TextRun
		if strings.TrimSpace(textRun.Content) != "" {
			hasContent = true
		}
		run := docsParagraphRun{
			StartIndex: element.StartIndex,
			EndIndex:   element.EndIndex,
			Text:       textRun.Content,
		}
		if textRun.TextStyle != nil {
			run.Bold = textRun.TextStyle.Bold
			run.Italic = textRun.TextStyle.Italic
			run.Underline = textRun.TextStyle.Underline
			run.Strikethrough = textRun.TextStyle.Strikethrough
			run.Link = docsParagraphRunLinkFrom(textRun.TextStyle.Link)
		}
		runs = append(runs, run)
	}
	return !hasContent, runs
}

func docsParagraphElementHasNonTextContent(element *docs.ParagraphElement) bool {
	return element.AutoText != nil ||
		element.ColumnBreak != nil ||
		element.DateElement != nil ||
		element.Equation != nil ||
		element.FootnoteReference != nil ||
		element.HorizontalRule != nil ||
		element.InlineObjectElement != nil ||
		element.PageBreak != nil ||
		element.Person != nil ||
		element.RichLink != nil
}

func docsParagraphRunLinkFrom(link *docs.Link) *docsParagraphRunLink {
	if link == nil {
		return nil
	}
	result := &docsParagraphRunLink{
		URL:        link.Url,
		BookmarkID: link.BookmarkId,
		HeadingID:  link.HeadingId,
		TabID:      link.TabId,
	}
	if link.Bookmark != nil {
		result.BookmarkID = link.Bookmark.Id
		result.TabID = link.Bookmark.TabId
	}
	if link.Heading != nil {
		result.HeadingID = link.Heading.Id
		result.TabID = link.Heading.TabId
	}
	if result.URL == "" && result.BookmarkID == "" && result.HeadingID == "" && result.TabID == "" {
		return nil
	}
	return result
}

func docsTSVField(value string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"\t", "\\t",
		"\r", "\\r",
		"\n", "\\n",
	).Replace(value)
}
