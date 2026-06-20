package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	docsTableContentAlignTop    = "TOP"
	docsTableContentAlignMiddle = "MIDDLE"
	docsTableContentAlignBottom = "BOTTOM"
	docsTableBorderDashSolid    = "SOLID"
	docsTableBorderDashDot      = "DOT"
	docsTableBorderDashDash     = "DASH"
)

type DocsCellStyleCmd struct {
	DocID           string `arg:"" name:"docId" help:"Doc ID"`
	TableIndex      int    `name:"table-index" help:"1-based table index in document order; negative indexes count from the end" default:"1"`
	Row             int    `name:"row" required:"" help:"1-based row number"`
	Col             int    `name:"col" required:"" help:"1-based column number"`
	RowSpan         int64  `name:"row-span" help:"Number of rows to style" default:"1"`
	ColSpan         int64  `name:"col-span" help:"Number of columns to style" default:"1"`
	BackgroundColor string `name:"background-color" aliases:"bg-color" help:"Cell background color as #RRGGBB or #RGB"`
	BorderAll       string `name:"border-all" help:"All borders as WIDTH[,COLOR[,SOLID|DOT|DASH]] (e.g. 1pt,#000,DASH)"`
	BorderTop       string `name:"border-top" help:"Top border; overrides --border-all"`
	BorderBottom    string `name:"border-bottom" help:"Bottom border; overrides --border-all"`
	BorderLeft      string `name:"border-left" help:"Left border; overrides --border-all"`
	BorderRight     string `name:"border-right" help:"Right border; overrides --border-all"`
	PaddingAll      string `name:"padding-all" help:"All cell padding (points by default; supports pt, in, cm, mm)"`
	PaddingTop      string `name:"padding-top" help:"Top cell padding; overrides --padding-all"`
	PaddingBottom   string `name:"padding-bottom" help:"Bottom cell padding; overrides --padding-all"`
	PaddingLeft     string `name:"padding-left" help:"Left cell padding; overrides --padding-all"`
	PaddingRight    string `name:"padding-right" help:"Right cell padding; overrides --padding-all"`
	ContentAlign    string `name:"content-align" help:"Vertical content alignment: top, middle, or bottom"`
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
	if c.TableIndex == 0 {
		return usage("--table-index cannot be 0")
	}
	if c.Row < 1 {
		return usage("--row must be >= 1")
	}
	if c.Col < 1 {
		return usage("--col must be >= 1")
	}
	if c.RowSpan < 1 || c.ColSpan < 1 {
		return usage("--row-span and --col-span must be >= 1")
	}
	if !c.anyStyle() {
		return usage("no style flags provided")
	}
	if c.hasTextStyle() && (c.RowSpan != 1 || c.ColSpan != 1) {
		return usage("text style flags target one cell; use cell style flags with --row-span/--col-span")
	}
	if err := dryRunExit(ctx, flags, "docs.cell-style", map[string]any{
		"documentId":      docID,
		"tableIndex":      c.TableIndex,
		"row":             c.Row,
		"col":             c.Col,
		"rowSpan":         c.RowSpan,
		"colSpan":         c.ColSpan,
		"backgroundColor": c.BackgroundColor,
		"borderAll":       c.BorderAll,
		"borderTop":       c.BorderTop,
		"borderBottom":    c.BorderBottom,
		"borderLeft":      c.BorderLeft,
		"borderRight":     c.BorderRight,
		"paddingAll":      c.PaddingAll,
		"paddingTop":      c.PaddingTop,
		"paddingBottom":   c.PaddingBottom,
		"paddingLeft":     c.PaddingLeft,
		"paddingRight":    c.PaddingRight,
		"contentAlign":    c.ContentAlign,
		"textColor":       c.TextColor,
		"bold":            c.Bold,
		"italic":          c.Italic,
		"underline":       c.Underline,
		"tab":             c.Tab,
		"batch":           c.Batch,
	}); err != nil {
		return err
	}
	if err := validateDocsBatchTarget(ctx, flags, c.Batch, docID); err != nil {
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

	table, resolvedTableIndex, err := resolveDocsTableWithIndex(loaded.target, c.TableIndex)
	if err != nil {
		return err
	}
	cell, err := findTableCell(loaded.target, &tableCellRef{
		tableIndex: resolvedTableIndex,
		row:        c.Row,
		col:        c.Col,
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
			"tableIndex": resolvedTableIndex,
			"row":        c.Row,
			"col":        c.Col,
			"requests":   len(reqs),
			"updated":    true,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}
	u.Out().Linef("documentId\t%s", resp.DocumentId)
	u.Out().Linef("table_index\t%d", resolvedTableIndex)
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
		strings.TrimSpace(c.BorderAll) != "" ||
		strings.TrimSpace(c.BorderTop) != "" ||
		strings.TrimSpace(c.BorderBottom) != "" ||
		strings.TrimSpace(c.BorderLeft) != "" ||
		strings.TrimSpace(c.BorderRight) != "" ||
		strings.TrimSpace(c.PaddingAll) != "" ||
		strings.TrimSpace(c.PaddingTop) != "" ||
		strings.TrimSpace(c.PaddingBottom) != "" ||
		strings.TrimSpace(c.PaddingLeft) != "" ||
		strings.TrimSpace(c.PaddingRight) != "" ||
		strings.TrimSpace(c.ContentAlign) != "" ||
		strings.TrimSpace(c.TextColor) != "" ||
		c.Bold || c.Italic || c.Underline
}

func (c *DocsCellStyleCmd) hasTextStyle() bool {
	return strings.TrimSpace(c.TextColor) != "" || c.Bold || c.Italic || c.Underline
}

func (c *DocsCellStyleCmd) buildRequests(tableStart int64, cell *docs.TableCell, tabID string) ([]*docs.Request, error) {
	reqs := make([]*docs.Request, 0, 2)
	cellStyle, fields, err := c.buildCellStyle()
	if err != nil {
		return nil, err
	}
	if len(fields) > 0 {
		reqs = append(reqs, &docs.Request{UpdateTableCellStyle: &docs.UpdateTableCellStyleRequest{
			TableCellStyle: cellStyle,
			Fields:         strings.Join(fields, ","),
			TableRange: &docs.TableRange{
				RowSpan:    c.RowSpan,
				ColumnSpan: c.ColSpan,
				TableCellLocation: &docs.TableCellLocation{
					RowIndex:    int64(c.Row - 1),
					ColumnIndex: int64(c.Col - 1),
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

func (c *DocsCellStyleCmd) buildCellStyle() (*docs.TableCellStyle, []string, error) {
	style := &docs.TableCellStyle{}
	fields := []string{}
	if bg := strings.TrimSpace(c.BackgroundColor); bg != "" {
		color, err := docsFormatColor(bg, "--background-color")
		if err != nil {
			return nil, nil, err
		}
		style.BackgroundColor = color
		fields = append(fields, "backgroundColor")
	}

	// Docs resolves shared-border conflicts in this order. Match it so the field
	// mask and request payload make the final edge precedence explicit.
	borderSides := []struct {
		field string
		flag  string
		value string
		set   func(*docs.TableCellBorder)
	}{
		{field: "borderRight", flag: "border-right", value: c.BorderRight, set: func(v *docs.TableCellBorder) { style.BorderRight = v }},
		{field: "borderLeft", flag: "border-left", value: c.BorderLeft, set: func(v *docs.TableCellBorder) { style.BorderLeft = v }},
		{field: "borderBottom", flag: "border-bottom", value: c.BorderBottom, set: func(v *docs.TableCellBorder) { style.BorderBottom = v }},
		{field: "borderTop", flag: "border-top", value: c.BorderTop, set: func(v *docs.TableCellBorder) { style.BorderTop = v }},
	}
	for _, side := range borderSides {
		raw, flag := strings.TrimSpace(side.value), side.flag
		if raw == "" {
			raw, flag = strings.TrimSpace(c.BorderAll), "border-all"
		}
		if raw == "" {
			continue
		}
		border, err := parseDocsTableCellBorder(flag, raw)
		if err != nil {
			return nil, nil, err
		}
		side.set(border)
		fields = append(fields, side.field)
	}

	paddingSides := []struct {
		field string
		flag  string
		value string
		set   func(*docs.Dimension)
	}{
		{field: "paddingTop", flag: "padding-top", value: c.PaddingTop, set: func(v *docs.Dimension) { style.PaddingTop = v }},
		{field: "paddingBottom", flag: "padding-bottom", value: c.PaddingBottom, set: func(v *docs.Dimension) { style.PaddingBottom = v }},
		{field: "paddingLeft", flag: "padding-left", value: c.PaddingLeft, set: func(v *docs.Dimension) { style.PaddingLeft = v }},
		{field: "paddingRight", flag: "padding-right", value: c.PaddingRight, set: func(v *docs.Dimension) { style.PaddingRight = v }},
	}
	for _, side := range paddingSides {
		raw, flag := strings.TrimSpace(side.value), side.flag
		if raw == "" {
			raw, flag = strings.TrimSpace(c.PaddingAll), "padding-all"
		}
		if raw == "" {
			continue
		}
		dimension, _, err := parseDocsDimension(flag, raw, true)
		if err != nil {
			return nil, nil, err
		}
		side.set(dimension)
		fields = append(fields, side.field)
	}

	if align := strings.ToUpper(strings.TrimSpace(c.ContentAlign)); align != "" {
		switch align {
		case docsTableContentAlignTop, docsTableContentAlignMiddle, docsTableContentAlignBottom:
			style.ContentAlignment = align
			fields = append(fields, "contentAlignment")
		default:
			return nil, nil, usage("--content-align must be top, middle, or bottom")
		}
	}
	return style, fields, nil
}

func parseDocsTableCellBorder(flag, raw string) (*docs.TableCellBorder, error) {
	parts := strings.Split(raw, ",")
	if len(parts) > 3 || strings.TrimSpace(parts[0]) == "" {
		return nil, usagef("invalid --%s %q (expected WIDTH[,COLOR[,SOLID|DOT|DASH]])", flag, raw)
	}
	width, _, err := parseDocsDimension(flag, strings.TrimSpace(parts[0]), true)
	if err != nil {
		return nil, err
	}
	colorRaw := "#000000"
	if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
		colorRaw = strings.TrimSpace(parts[1])
	}
	color, err := docsFormatColor(colorRaw, "--"+flag)
	if err != nil {
		return nil, err
	}
	dash := docsTableBorderDashSolid
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		dash = strings.ToUpper(strings.TrimSpace(parts[2]))
	}
	switch dash {
	case docsTableBorderDashSolid, docsTableBorderDashDot, docsTableBorderDashDash:
	default:
		return nil, usagef("invalid --%s dash style %q (expected SOLID, DOT, or DASH)", flag, strings.TrimSpace(parts[2]))
	}
	return &docs.TableCellBorder{Width: width, Color: color, DashStyle: dash}, nil
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
