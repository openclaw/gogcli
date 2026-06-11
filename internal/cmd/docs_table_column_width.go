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

type DocsTableColumnWidthCmd struct {
	DocID             string  `arg:"" name:"docId" help:"Doc ID"`
	TableIndex        int     `name:"table-index" help:"1-based table index in document order; negative indexes count from the end" default:"1"`
	Col               int     `name:"col" help:"1-based column number. Omit with --evenly-distributed to reset all columns."`
	Width             float64 `name:"width" help:"Fixed column width in points (minimum 5pt)"`
	EvenlyDistributed bool    `name:"evenly-distributed" aliases:"even" help:"Reset selected column, or all columns when --col is omitted, to Docs-managed equal width"`
	Tab               string  `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	Batch             string  `name:"batch" help:"Append requests to a persisted Docs batch instead of submitting"`
}

func (c *DocsTableColumnWidthCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if err := c.validate(); err != nil {
		return err
	}
	dryRunPayload := map[string]any{
		"documentId":        docID,
		"tableIndex":        c.TableIndex,
		"evenlyDistributed": c.EvenlyDistributed,
		"tab":               c.Tab,
		"batch":             c.Batch,
	}
	if c.Col > 0 {
		dryRunPayload["col"] = c.Col
	} else {
		dryRunPayload["allColumns"] = true
	}
	if c.Width > 0 {
		dryRunPayload["width"] = c.Width
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.table-column-width", dryRunPayload); dryRunErr != nil {
		return dryRunErr
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

	table, resolvedTableIndex, err := resolveDocsTableWithIndex(loaded.target, c.TableIndex)
	if err != nil {
		return err
	}
	req, err := c.buildRequest(table.startIdx, docsTableColumnCount(table.table), c.Tab)
	if err != nil {
		return err
	}
	if queued, queueErr := queueDocsBatchRequests(ctx, flags, c.Batch, docID, "docs.table-column-width", loaded.full.RevisionId, []*docs.Request{req}, false); queued || queueErr != nil {
		return queueErr
	}
	resp, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		WriteControl: &docs.WriteControl{RequiredRevisionId: loaded.full.RevisionId},
		Requests:     []*docs.Request{req},
	}).Context(ctx).Do()
	if err != nil {
		if isDocsNotFound(err) {
			return fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return fmt.Errorf("table column width: %w", err)
	}

	widthType := req.UpdateTableColumnProperties.TableColumnProperties.WidthType
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": resp.DocumentId,
			"tableIndex": resolvedTableIndex,
			"widthType":  widthType,
			"updated":    true,
		}
		if c.Col > 0 {
			payload["col"] = c.Col
		} else {
			payload["allColumns"] = true
		}
		if c.Width > 0 {
			payload["width"] = c.Width
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, os.Stdout, payload)
	}
	u.Out().Linef("documentId\t%s", resp.DocumentId)
	u.Out().Linef("table_index\t%d", resolvedTableIndex)
	if c.Col > 0 {
		u.Out().Linef("col\t%d", c.Col)
	} else {
		u.Out().Linef("all_columns\ttrue")
	}
	u.Out().Linef("width_type\t%s", widthType)
	if c.Width > 0 {
		u.Out().Linef("width\t%.3f", c.Width)
	}
	u.Out().Linef("updated\ttrue")
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

func (c *DocsTableColumnWidthCmd) validate() error {
	if c.TableIndex == 0 {
		return usage("--table-index cannot be 0")
	}
	if c.Col < 0 {
		return usage("--col must be >= 1")
	}
	if c.EvenlyDistributed {
		if c.Width != 0 {
			return usage("--width and --evenly-distributed are mutually exclusive")
		}
		return nil
	}
	if c.Width == 0 {
		return usage("set --width or --evenly-distributed")
	}
	if c.Width < 5 {
		return usage("--width must be >= 5pt")
	}
	if c.Col < 1 {
		return usage("--col is required when setting --width")
	}
	return nil
}

func (c *DocsTableColumnWidthCmd) buildRequest(tableStart int64, colCount int, tabID string) (*docs.Request, error) {
	if colCount < 1 {
		return nil, usage("target table has no columns")
	}
	var columnIndices []int64
	if c.Col > 0 {
		if c.Col > colCount {
			return nil, usagef("col %d out of range (table has %d columns)", c.Col, colCount)
		}
		columnIndices = []int64{int64(c.Col - 1)}
	}

	props := &docs.TableColumnProperties{}
	fields := "widthType"
	if c.EvenlyDistributed {
		props.WidthType = "EVENLY_DISTRIBUTED"
	} else {
		props.WidthType = "FIXED_WIDTH"
		props.Width = &docs.Dimension{Magnitude: c.Width, Unit: "PT"}
		fields = "width,widthType"
	}

	return &docs.Request{UpdateTableColumnProperties: &docs.UpdateTableColumnPropertiesRequest{
		TableStartLocation:    &docs.Location{Index: tableStart, TabId: tabID},
		ColumnIndices:         columnIndices,
		TableColumnProperties: props,
		Fields:                fields,
	}}, nil
}

func resolveDocsTableWithIndex(doc *docs.Document, requested int) (tableWithIndex, int, error) {
	tables := collectAllTablesWithIndex(doc)
	if len(tables) == 0 {
		return tableWithIndex{}, 0, usage("document has no tables")
	}
	idx := requested
	if idx < 0 {
		idx = len(tables) + idx + 1
	}
	if idx < 1 || idx > len(tables) {
		return tableWithIndex{}, 0, usagef("table %d out of range (document has %d tables)", requested, len(tables))
	}
	return tables[idx-1], idx, nil
}

func docsTableColumnCount(table *docs.Table) int {
	if table == nil {
		return 0
	}
	if table.Columns > 0 {
		return int(table.Columns)
	}
	if len(table.TableRows) > 0 {
		return len(table.TableRows[0].TableCells)
	}
	return 0
}
