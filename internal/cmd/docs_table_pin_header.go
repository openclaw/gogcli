package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/docs/v1"
)

type DocsTablePinHeaderCmd struct {
	DocID string `arg:"" name:"docId" help:"Doc ID"`
	Table string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	Rows  int64  `name:"rows" required:"" help:"Number of leading rows to pin; 0 unpins all header rows"`
	Tab   string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

func (c *DocsTablePinHeaderCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID, err := validateDocsTableMutationArgs(c.DocID, c.Table)
	if err != nil {
		return err
	}
	if c.Rows < 0 {
		return usage("--rows must be >= 0")
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.table-row.pin-header", map[string]any{
		"documentId": docID,
		"table":      c.Table,
		"rows":       c.Rows,
		"tab":        c.Tab,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, loaded, selected, err := loadDocsSelectedTables(ctx, flags, docID, c.Tab, c.Table)
	if err != nil {
		return err
	}
	requests := make([]*docs.Request, 0, len(selected))
	results := make([]docsTableRowActionResult, 0, len(selected))
	for _, target := range selected {
		rowCount := int64(len(target.table.TableRows))
		if c.Rows > rowCount {
			return usagef("cannot pin %d header rows in table %d (table has %d rows)", c.Rows, target.index, rowCount)
		}
		requests = append(requests, &docs.Request{PinTableHeaderRows: &docs.PinTableHeaderRowsRequest{
			TableStartLocation:    &docs.Location{Index: target.startIdx, TabId: loaded.tabID},
			PinnedHeaderRowsCount: c.Rows,
			ForceSendFields:       []string{"PinnedHeaderRowsCount"},
		}})
		results = append(results, docsTableRowActionResult{TableIndex: target.index, Value: c.Rows})
	}
	if _, err := executeDocsTableRequests(ctx, svc, docID, loaded.full.RevisionId, requests); err != nil {
		return fmt.Errorf("pin table header rows: %w", err)
	}
	return writeDocsTableRowActionResult(ctx, docID, loaded.tabID, "pin-header", "rows", results)
}
