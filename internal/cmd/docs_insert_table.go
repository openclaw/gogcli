package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/docsedit"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// DocsInsertTableCmd inserts a native Google Docs table at a specific
// character index (or at end-of-doc) and optionally populates it from a JSON
// 2D string array. The markdown writer can already render tables, but it
// drops them mid-insert in some scenarios — see #592/#607/#608/#609 — and
// agents needed a path that bypasses the markdown converter entirely (#602).
type DocsInsertTableCmd struct {
	DocID      string `arg:"" name:"docId" help:"Doc ID"`
	Rows       int    `name:"rows" required:"" help:"Number of rows (>=1)"`
	Cols       int    `name:"cols" required:"" help:"Number of columns (>=1)"`
	Index      *int64 `name:"index" help:"Character index to insert at (1 = beginning). Omit or use --at-end for end-of-doc."`
	AtEnd      bool   `name:"at-end" help:"Insert at end-of-doc/tab (mutually exclusive with --index)"`
	ValuesJSON string `name:"values-json" help:"Cell values as a JSON 2D string array; dimensions must match --rows x --cols when supplied"`
	Tab        string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
	TabID      string `name:"tab-id" hidden:"" help:"(deprecated) Use --tab"`
}

func (c *DocsInsertTableCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	docID := strings.TrimSpace(c.DocID)
	if docID == "" {
		return usage("empty docId")
	}
	if c.Rows < 1 {
		return usage("--rows must be >= 1")
	}
	if c.Cols < 1 {
		return usage("--cols must be >= 1")
	}
	if c.AtEnd && c.Index != nil {
		return usage("--at-end and --index are mutually exclusive")
	}
	if c.Index != nil && *c.Index < 1 {
		return usage("--index must be >= 1 (index 0 is reserved)")
	}

	cells, err := parseTableValuesJSON(c.ValuesJSON, c.Rows, c.Cols)
	if err != nil {
		return err
	}

	tab, tabErr := resolveTabArg(ctx, c.Tab, c.TabID)
	if tabErr != nil {
		return tabErr
	}
	c.Tab = tab

	resolveEnd := c.AtEnd || c.Index == nil

	dryRunPayload := map[string]any{
		"documentId": docID,
		"rows":       c.Rows,
		"cols":       c.Cols,
		"tab":        c.Tab,
	}
	if resolveEnd {
		dryRunPayload["atIndex"] = docsAtIndexEnd
	} else {
		dryRunPayload["atIndex"] = *c.Index
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.insert-table", dryRunPayload); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := requireDocsService(ctx, flags)
	if err != nil {
		return err
	}

	var insertIndex int64
	if resolveEnd {
		endIndex, tabID, endErr := docsTargetEndIndexAndTabID(ctx, svc, docID, c.Tab)
		if endErr != nil {
			return endErr
		}
		c.Tab = tabID
		insertIndex = docsedit.AppendIndex(endIndex)
	} else {
		insertIndex = *c.Index
		if c.Tab != "" {
			tabID, tabErr := resolveDocsTabID(ctx, svc, docID, c.Tab)
			if tabErr != nil {
				return tabErr
			}
			c.Tab = tabID
		}
	}

	inserter := NewTableInserter(svc, docID)
	tableEnd, err := inserter.InsertNativeTable(ctx, insertIndex, cells, c.Tab)
	if err != nil {
		return fmt.Errorf("insert table: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"atIndex":    insertIndex,
			"rows":       c.Rows,
			"cols":       c.Cols,
			"tableEnd":   tableEnd,
		}
		if c.Tab != "" {
			payload["tabId"] = c.Tab
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("atIndex\t%d", insertIndex)
	u.Out().Linef("rows\t%d", c.Rows)
	u.Out().Linef("cols\t%d", c.Cols)
	u.Out().Linef("tableEnd\t%d", tableEnd)
	if c.Tab != "" {
		u.Out().Linef("tabId\t%s", c.Tab)
	}
	return nil
}

// parseTableValuesJSON converts a JSON 2D string array into a rows x cols cell
// matrix. When the input is empty, returns an all-empty rows x cols matrix
// suitable for inserting an empty table structure. Validates that the supplied
// JSON exactly matches the requested dimensions.
func parseTableValuesJSON(raw string, rows, cols int) ([][]string, error) {
	if strings.TrimSpace(raw) == "" {
		cells := make([][]string, rows)
		for i := range cells {
			cells[i] = make([]string, cols)
		}
		return cells, nil
	}

	var parsed [][]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, usagef("--values-json must be a JSON 2D string array: %v", err)
	}
	if len(parsed) != rows {
		return nil, usagef("--values-json row count %d does not match --rows %d", len(parsed), rows)
	}
	for i, row := range parsed {
		if len(row) != cols {
			return nil, usagef("--values-json row %d has %d columns, want %d", i, len(row), cols)
		}
	}
	return parsed, nil
}
