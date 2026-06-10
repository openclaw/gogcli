package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsTablesCmd struct {
	List DocsTablesListCmd `cmd:"" name:"list" aliases:"ls" help:"List tables in a Google Doc"`
}

type DocsImagesCmd struct {
	List DocsImagesListCmd `cmd:"" name:"list" aliases:"ls" help:"List images in a Google Doc"`
}

type DocsHeadingsCmd struct {
	List DocsHeadingsListCmd `cmd:"" name:"list" aliases:"ls" help:"List headings in a Google Doc"`
}

type DocsParagraphsCmd struct {
	List DocsParagraphsListCmd `cmd:"" name:"list" aliases:"ls" help:"List paragraphs in a Google Doc"`
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
	Level int    `name:"level" help:"Heading level to include (1-6)"`
}

type DocsParagraphsListCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Tab   string `name:"tab" help:"Tab title or ID (omit for default)"`
	Style string `name:"style" help:"Named paragraph style to include (for example NORMAL_TEXT or HEADING_2)"`
}

type docsEnumeratorScope struct {
	tabID    string
	tabTitle string
	content  []*docs.StructuralElement
	doc      *docs.Document
}

type docsTableListItem struct {
	Index      int      `json:"index"`
	StartIndex int64    `json:"startIndex,omitempty"`
	EndIndex   int64    `json:"endIndex,omitempty"`
	Rows       int      `json:"rows"`
	Cols       int      `json:"cols"`
	HeaderRow  []string `json:"headerRow"`
	TabID      string   `json:"tabId,omitempty"`
	TabTitle   string   `json:"tabTitle,omitempty"`
}

type docsImageListItem struct {
	Index      int     `json:"index"`
	ObjectID   string  `json:"objectId"`
	StartIndex int64   `json:"startIndex,omitempty"`
	Alt        string  `json:"alt"`
	Positioned bool    `json:"positioned"`
	WidthPt    float64 `json:"widthPt,omitempty"`
	HeightPt   float64 `json:"heightPt,omitempty"`
	WidthUnit  string  `json:"widthUnit,omitempty"`
	HeightUnit string  `json:"heightUnit,omitempty"`
	TabID      string  `json:"tabId,omitempty"`
	TabTitle   string  `json:"tabTitle,omitempty"`
}

type docsHeadingListItem struct {
	Index      int    `json:"index"`
	Level      int    `json:"level"`
	StartIndex int64  `json:"startIndex,omitempty"`
	EndIndex   int64  `json:"endIndex,omitempty"`
	Style      string `json:"style"`
	HeadingID  string `json:"headingId,omitempty"`
	Text       string `json:"text"`
	TabID      string `json:"tabId,omitempty"`
	TabTitle   string `json:"tabTitle,omitempty"`
}

type docsParagraphListItem struct {
	Index      int    `json:"index"`
	StartIndex int64  `json:"startIndex,omitempty"`
	EndIndex   int64  `json:"endIndex,omitempty"`
	Style      string `json:"style"`
	Bullet     bool   `json:"bullet"`
	NestLevel  int    `json:"nestLevel,omitempty"`
	Text       string `json:"text"`
	TabID      string `json:"tabId,omitempty"`
	TabTitle   string `json:"tabTitle,omitempty"`
}

func (c *DocsTablesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	scope, err := fetchDocsEnumeratorScope(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}

	items := enumerateDocsTables(scope)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, items)
	}

	u := ui.FromContext(ctx)
	for _, item := range items {
		if outfmt.IsPlain(ctx) {
			u.Out().Linef("table\t%d\t%d\t%d\t%d\t%d\t%s", item.Index, item.Rows, item.Cols, item.StartIndex, item.EndIndex, docsTSV(strings.Join(item.HeaderRow, " | ")))
			continue
		}
		u.Out().Linef("table %d  rows=%d cols=%d", item.Index, item.Rows, item.Cols)
		if len(item.HeaderRow) > 0 {
			u.Out().Linef("  row 1: %s", strings.Join(item.HeaderRow, " | "))
		}
	}
	return nil
}

func (c *DocsImagesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	scope, err := fetchDocsEnumeratorScope(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}

	items := enumerateDocsImages(scope)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, items)
	}

	u := ui.FromContext(ctx)
	for _, item := range items {
		if outfmt.IsPlain(ctx) {
			u.Out().Linef("image\t%d\t%s\t%d\t%s\t%s\t%t", item.Index, item.ObjectID, item.StartIndex, docsTSV(item.Alt), docsTSV(formatDocsImageSize(item)), item.Positioned)
			continue
		}
		u.Out().Linef("image %d  objectId=%s  alt=%q  size=%s", item.Index, item.ObjectID, item.Alt, formatDocsImageSize(item))
	}
	return nil
}

func (c *DocsHeadingsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	if c.Level < 0 || c.Level > 6 {
		return usage("--level must be between 1 and 6")
	}
	scope, err := fetchDocsEnumeratorScope(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}

	items := enumerateDocsHeadings(scope, c.Level)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, items)
	}

	u := ui.FromContext(ctx)
	for _, item := range items {
		if outfmt.IsPlain(ctx) {
			u.Out().Linef("heading\t%d\t%d\t%d\t%d\t%s", item.Index, item.Level, item.StartIndex, item.EndIndex, docsTSV(item.Text))
			continue
		}
		u.Out().Linef("heading %d  level=%d  index=%d  %q", item.Index, item.Level, item.StartIndex, item.Text)
	}
	return nil
}

func (c *DocsParagraphsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	style := strings.ToUpper(strings.TrimSpace(c.Style))
	scope, err := fetchDocsEnumeratorScope(ctx, flags, c.DocID, c.Tab)
	if err != nil {
		return err
	}

	items := enumerateDocsParagraphs(scope, style)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, items)
	}

	u := ui.FromContext(ctx)
	for _, item := range items {
		if outfmt.IsPlain(ctx) {
			u.Out().Linef("paragraph\t%d\t%s\t%d\t%d\t%t\t%d\t%s", item.Index, item.Style, item.StartIndex, item.EndIndex, item.Bullet, item.NestLevel, docsTSV(item.Text))
			continue
		}
		u.Out().Linef("paragraph %d  style=%s  index=%d  %q", item.Index, item.Style, item.StartIndex, item.Text)
	}
	return nil
}

func fetchDocsEnumeratorScope(ctx context.Context, flags *RootFlags, rawDocID, rawTab string) (*docsEnumeratorScope, error) {
	id := strings.TrimSpace(rawDocID)
	if id == "" {
		return nil, usage("empty docId")
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return nil, err
	}

	tabQuery := strings.TrimSpace(rawTab)
	getCall := svc.Documents.Get(id)
	if tabQuery != "" {
		getCall = getCall.IncludeTabsContent(true)
	}
	doc, err := getCall.Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return nil, fmt.Errorf("doc not found or not a Google Doc (id=%s)", id)
		}
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("doc not found")
	}

	scope := &docsEnumeratorScope{doc: doc}
	if tabQuery != "" {
		tabs := flattenTabs(doc.Tabs)
		tab, tabErr := findTab(tabs, tabQuery)
		if tabErr != nil {
			return nil, tabErr
		}
		if tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
			return nil, fmt.Errorf("tab has no content: %s", tabQuery)
		}
		scope.content = tab.DocumentTab.Body.Content
		if tab.TabProperties != nil {
			scope.tabID = tab.TabProperties.TabId
			scope.tabTitle = tab.TabProperties.Title
		}
		return scope, nil
	}

	if doc.Body == nil {
		return nil, errors.New("document has no body")
	}
	scope.content = doc.Body.Content
	return scope, nil
}

func enumerateDocsTables(scope *docsEnumeratorScope) []docsTableListItem {
	var items []docsTableListItem
	var walk func(content []*docs.StructuralElement)
	walk = func(content []*docs.StructuralElement) {
		for _, elem := range content {
			if elem == nil {
				continue
			}
			if elem.Table != nil {
				items = append(items, docsTableListItem{
					Index:      len(items) + 1,
					StartIndex: elem.StartIndex,
					EndIndex:   elem.EndIndex,
					Rows:       len(elem.Table.TableRows),
					Cols:       docsTableMaxColumnCount(elem.Table),
					HeaderRow:  docsTableHeaderRow(elem.Table),
					TabID:      scope.tabID,
					TabTitle:   scope.tabTitle,
				})
				for _, row := range elem.Table.TableRows {
					if row == nil {
						continue
					}
					for _, cell := range row.TableCells {
						if cell != nil {
							walk(cell.Content)
						}
					}
				}
			}
		}
	}
	walk(scope.content)
	return items
}

func enumerateDocsImages(scope *docsEnumeratorScope) []docsImageListItem {
	var items []docsImageListItem
	inlineProps := make(map[string]*docs.InlineObjectProperties)
	for id, obj := range scope.doc.InlineObjects {
		if obj.InlineObjectProperties != nil {
			inlineProps[id] = obj.InlineObjectProperties
		}
	}

	var walk func(content []*docs.StructuralElement)
	walk = func(content []*docs.StructuralElement) {
		for _, elem := range content {
			if elem == nil {
				continue
			}
			if elem.Paragraph != nil {
				for _, pe := range elem.Paragraph.Elements {
					if pe == nil || pe.InlineObjectElement == nil {
						continue
					}
					objID := pe.InlineObjectElement.InlineObjectId
					item := docsImageListItem{
						Index:      len(items) + 1,
						ObjectID:   objID,
						StartIndex: pe.StartIndex,
						TabID:      scope.tabID,
						TabTitle:   scope.tabTitle,
					}
					if props := inlineProps[objID]; props != nil && props.EmbeddedObject != nil {
						fillDocsImageEmbeddedObject(&item, props.EmbeddedObject)
					}
					items = append(items, item)
				}
			}
			if elem.Table != nil {
				for _, row := range elem.Table.TableRows {
					if row == nil {
						continue
					}
					for _, cell := range row.TableCells {
						if cell != nil {
							walk(cell.Content)
						}
					}
				}
			}
		}
	}
	walk(scope.content)

	if scope.tabID == "" {
		ids := make([]string, 0, len(scope.doc.PositionedObjects))
		for id := range scope.doc.PositionedObjects {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			obj := scope.doc.PositionedObjects[id]
			item := docsImageListItem{
				Index:      len(items) + 1,
				ObjectID:   id,
				Positioned: true,
			}
			if obj.PositionedObjectProperties != nil && obj.PositionedObjectProperties.EmbeddedObject != nil {
				fillDocsImageEmbeddedObject(&item, obj.PositionedObjectProperties.EmbeddedObject)
			}
			items = append(items, item)
		}
	}
	return items
}

func enumerateDocsHeadings(scope *docsEnumeratorScope, level int) []docsHeadingListItem {
	var items []docsHeadingListItem
	walkDocsParagraphs(scope.content, func(elem *docs.StructuralElement, p *docs.Paragraph) {
		style := docsParagraphStyle(p)
		headingLevel, ok := docsHeadingLevel(style)
		if !ok || (level > 0 && headingLevel != level) {
			return
		}
		headingID := ""
		if p.ParagraphStyle != nil {
			headingID = p.ParagraphStyle.HeadingId
		}
		items = append(items, docsHeadingListItem{
			Index:      len(items) + 1,
			Level:      headingLevel,
			StartIndex: elem.StartIndex,
			EndIndex:   elem.EndIndex,
			Style:      style,
			HeadingID:  headingID,
			Text:       paragraphText(p),
			TabID:      scope.tabID,
			TabTitle:   scope.tabTitle,
		})
	})
	return items
}

func enumerateDocsParagraphs(scope *docsEnumeratorScope, styleFilter string) []docsParagraphListItem {
	var items []docsParagraphListItem
	walkDocsParagraphs(scope.content, func(elem *docs.StructuralElement, p *docs.Paragraph) {
		style := docsParagraphStyle(p)
		if styleFilter != "" && style != styleFilter {
			return
		}
		item := docsParagraphListItem{
			Index:      len(items) + 1,
			StartIndex: elem.StartIndex,
			EndIndex:   elem.EndIndex,
			Style:      style,
			Text:       paragraphText(p),
			TabID:      scope.tabID,
			TabTitle:   scope.tabTitle,
		}
		if p.Bullet != nil {
			item.Bullet = true
			item.NestLevel = int(p.Bullet.NestingLevel)
		}
		items = append(items, item)
	})
	return items
}

func walkDocsParagraphs(content []*docs.StructuralElement, visit func(*docs.StructuralElement, *docs.Paragraph)) {
	for _, elem := range content {
		if elem == nil {
			continue
		}
		if elem.Paragraph != nil {
			visit(elem, elem.Paragraph)
		}
		if elem.Table != nil {
			for _, row := range elem.Table.TableRows {
				if row == nil {
					continue
				}
				for _, cell := range row.TableCells {
					if cell != nil {
						walkDocsParagraphs(cell.Content, visit)
					}
				}
			}
		}
	}
}

func docsTableMaxColumnCount(table *docs.Table) int {
	if table == nil || len(table.TableRows) == 0 {
		return 0
	}
	maxCols := 0
	for _, row := range table.TableRows {
		if row == nil {
			continue
		}
		if len(row.TableCells) > maxCols {
			maxCols = len(row.TableCells)
		}
	}
	return maxCols
}

func docsTableHeaderRow(table *docs.Table) []string {
	if table == nil || len(table.TableRows) == 0 {
		return nil
	}
	if table.TableRows[0] == nil {
		return nil
	}
	header := make([]string, 0, len(table.TableRows[0].TableCells))
	for _, cell := range table.TableRows[0].TableCells {
		header = append(header, docsTableCellPlainText(cell))
	}
	return header
}

func docsTableCellPlainText(cell *docs.TableCell) string {
	if cell == nil {
		return ""
	}
	var b strings.Builder
	for _, elem := range cell.Content {
		appendDocsStructuralPlainText(&b, elem)
	}
	return strings.TrimSpace(b.String())
}

func appendDocsStructuralPlainText(b *strings.Builder, elem *docs.StructuralElement) {
	if elem == nil {
		return
	}
	if elem.Paragraph != nil {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(paragraphText(elem.Paragraph))
	}
	if elem.Table != nil {
		for _, row := range elem.Table.TableRows {
			if row == nil {
				continue
			}
			for _, cell := range row.TableCells {
				if text := docsTableCellPlainText(cell); text != "" {
					if b.Len() > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(text)
				}
			}
		}
	}
}

func fillDocsImageEmbeddedObject(item *docsImageListItem, obj *docs.EmbeddedObject) {
	if item == nil || obj == nil {
		return
	}
	item.Alt = firstNonEmpty(obj.Title, obj.Description)
	if obj.Size == nil {
		return
	}
	if obj.Size.Width != nil {
		item.WidthPt = obj.Size.Width.Magnitude
		item.WidthUnit = obj.Size.Width.Unit
	}
	if obj.Size.Height != nil {
		item.HeightPt = obj.Size.Height.Magnitude
		item.HeightUnit = obj.Size.Height.Unit
	}
}

func docsParagraphStyle(p *docs.Paragraph) string {
	if p != nil && p.ParagraphStyle != nil && strings.TrimSpace(p.ParagraphStyle.NamedStyleType) != "" {
		return strings.ToUpper(strings.TrimSpace(p.ParagraphStyle.NamedStyleType))
	}
	return docsNamedStyleNormalText
}

func docsHeadingLevel(style string) (int, bool) {
	raw := strings.TrimPrefix(style, "HEADING_")
	if raw == style {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 6 {
		return 0, false
	}
	return n, true
}

func formatDocsImageSize(item docsImageListItem) string {
	if item.WidthPt == 0 && item.HeightPt == 0 {
		return ""
	}
	width := docsFormatDimension(item.WidthPt, item.WidthUnit)
	height := docsFormatDimension(item.HeightPt, item.HeightUnit)
	if width == "" {
		return "x" + height
	}
	if height == "" {
		return width + "x"
	}
	return width + "x" + height
}

func docsFormatDimension(value float64, unit string) string {
	if value == 0 {
		return ""
	}
	if unit == "" {
		unit = "PT"
	}
	if value == float64(int64(value)) {
		return fmt.Sprintf("%.0f%s", value, strings.ToLower(unit))
	}
	return fmt.Sprintf("%.3g%s", value, strings.ToLower(unit))
}

func docsTSV(s string) string {
	replacer := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ")
	return replacer.Replace(s)
}
