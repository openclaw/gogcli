package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsCellStyleCmd struct {
	DocID           string `arg:"" name:"docId" help:"Doc ID"`
	TableIndex      int    `name:"table-index" help:"0-based table index in document order" default:"0"`
	Row             int    `name:"row" required:"" help:"0-based row number"`
	Col             int    `name:"col" required:"" help:"0-based column number"`
	RowSpan         int64  `name:"row-span" help:"Number of rows to style" default:"1"`
	ColSpan         int64  `name:"col-span" help:"Number of columns to style" default:"1"`
	BackgroundColor string `name:"background-color" aliases:"bg-color" help:"Cell background color as #RRGGBB or #RGB"`
	TextColor       string `name:"text-color" help:"Text color as #RRGGBB or #RGB"`
	Bold            bool   `name:"bold" help:"Set cell text bold"`
	Italic          bool   `name:"italic" help:"Set cell text italic"`
	Underline       bool   `name:"underline" help:"Set cell text underline"`
	Tab             string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Batch           string `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsCellStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.TableIndex < 0 {
		return usage("--table-index must be >= 0")
	}
	if c.Row < 0 {
		return usage("--row must be >= 0")
	}
	if c.Col < 0 {
		return usage("--col must be >= 0")
	}
	if c.RowSpan < 1 || c.ColSpan < 1 {
		return usage("--row-span and --col-span must be >= 1")
	}
	if !c.anyStyle() {
		return usage("no style flags provided")
	}
	if c.hasTextStyle() && (c.RowSpan != 1 || c.ColSpan != 1) {
		return usage("--row-span/--col-span can only be combined with --background-color; text style flags target one cell")
	}
	if err := dryRunExit(ctx, flags, "docs.cell-style", map[string]any{
		"documentId":      docID,
		"tableIndex":      c.TableIndex,
		"row":             c.Row,
		"col":             c.Col,
		"rowSpan":         c.RowSpan,
		"colSpan":         c.ColSpan,
		"backgroundColor": c.BackgroundColor,
		"textColor":       c.TextColor,
		"bold":            c.Bold,
		"italic":          c.Italic,
		"underline":       c.Underline,
		"tab":             c.Tab,
		"batch":           c.Batch,
	}); err != nil {
		return err
	}
	if err := validateDocsBatchTarget(flags, c.Batch, docID); err != nil {
		return err
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}
	loaded, err := loadDocsTargetDocument(ctx, svc, docID, c.Tab)
	if err != nil {
		return err
	}
	c.Tab = loaded.tabID

	tables := collectAllTablesWithIndex(loaded.target)
	if len(tables) == 0 {
		return usage("document has no tables")
	}
	if c.TableIndex >= len(tables) {
		return usagef("table %d out of range (document has %d tables)", c.TableIndex, len(tables))
	}
	table := tables[c.TableIndex]
	cell, err := findTableCell(loaded.target, &tableCellRef{
		tableIndex: c.TableIndex + 1,
		row:        c.Row + 1,
		col:        c.Col + 1,
	})
	if err != nil {
		return err
	}
	reqs, err := c.buildRequests(table.startIdx, cell, c.Tab)
	if err != nil {
		return err
	}
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.cell-style", loaded.full.RevisionId, reqs, false); queued || queueErr != nil {
		return queueErr
	}
	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{RequiredRevisionId: loaded.full.RevisionId},
		Requests:     reqs,
	}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return fmt.Errorf("cell style: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"tableIndex": c.TableIndex,
			"row":        c.Row,
			"col":        c.Col,
			"requests":   len(reqs),
			"updated":    true,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}
	u.Out().Linef("documentId\t%s", resp.DocumentId)
	u.Out().Linef("table_index\t%d", c.TableIndex)
	u.Out().Linef("row\t%d", c.Row)
	u.Out().Linef("col\t%d", c.Col)
	u.Out().Linef("requests\t%d", len(reqs))
	u.Out().Linef("updated\ttrue")
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsCellStyleCmd) anyStyle() bool {
	return strings.TrimSpace(c.BackgroundColor) != "" ||
		strings.TrimSpace(c.TextColor) != "" ||
		c.Bold || c.Italic || c.Underline
}

func (c *DocsCellStyleCmd) hasTextStyle() bool {
	return strings.TrimSpace(c.TextColor) != "" || c.Bold || c.Italic || c.Underline
}

func (c *DocsCellStyleCmd) buildRequests(tableStart int64, cell *docs.TableCell, tabID string) ([]*docs.Request, error) {
	reqs := make([]*docs.Request, 0, 2)
	if bg := strings.TrimSpace(c.BackgroundColor); bg != "" {
		color, err := docsFormatColor(bg, "--background-color")
		if err != nil {
			return nil, err
		}
		reqs = append(reqs, &docs.Request{UpdateTableCellStyle: &docs.UpdateTableCellStyleRequest{
			TableCellStyle: &docs.TableCellStyle{BackgroundColor: color},
			Fields:         "backgroundColor",
			TableRange: &docs.TableRange{
				RowSpan:    c.RowSpan,
				ColumnSpan: c.ColSpan,
				TableCellLocation: &docs.TableCellLocation{
					RowIndex:    int64(c.Row),
					ColumnIndex: int64(c.Col),
					TableStartLocation: &docs.Location{
						Index: tableStart,
						TabId: tabID,
					},
					ForceSendFields: []string{"RowIndex", "ColumnIndex"},
				},
			},
		}})
	}

	textReq, ok, err := c.buildTextStyleRequest(cell, tabID)
	if err != nil {
		return nil, err
	}
	if ok {
		reqs = append(reqs, textReq)
	}
	if len(reqs) == 0 {
		return nil, usage("no style flags provided")
	}
	return reqs, nil
}

func (c *DocsCellStyleCmd) buildTextStyleRequest(cell *docs.TableCell, tabID string) (*docs.Request, bool, error) {
	style := &docs.TextStyle{}
	fields := []string{}
	if color := strings.TrimSpace(c.TextColor); color != "" {
		optionalColor, err := docsFormatColor(color, "--text-color")
		if err != nil {
			return nil, false, err
		}
		style.ForegroundColor = optionalColor
		fields = append(fields, "foregroundColor")
	}
	if c.Bold {
		style.Bold = true
		fields = append(fields, "bold")
	}
	if c.Italic {
		style.Italic = true
		fields = append(fields, "italic")
	}
	if c.Underline {
		style.Underline = true
		fields = append(fields, "underline")
	}
	if len(fields) == 0 {
		return nil, false, nil
	}
	cellText, startIdx, endIdx := getCellText(cell)
	if startIdx <= 0 || endIdx <= startIdx {
		return nil, false, fmt.Errorf("target cell has no editable text range")
	}
	if strings.HasSuffix(cellText, "\n") {
		endIdx--
	}
	if endIdx <= startIdx {
		return nil, false, fmt.Errorf("target cell has no editable text")
	}
	return &docs.Request{UpdateTextStyle: &docs.UpdateTextStyleRequest{
		Range:     &docs.Range{StartIndex: startIdx, EndIndex: endIdx, TabId: tabID},
		TextStyle: style,
		Fields:    strings.Join(fields, ","),
	}}, true, nil
}
