package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type DocsTableRowStyleCmd struct {
	DocID           string `arg:"" name:"docId" help:"Doc ID"`
	Table           string `name:"table" help:"Table selector: index, exact first-cell text, *, or text:VALUE for numeric/syntax-looking text" default:"1"`
	Row             *int   `name:"row" help:"1-based row number; negative indexes count from the end; omit to style all rows"`
	MinHeight       string `name:"min-height" help:"Minimum row height (points by default; supports pt, in, cm, mm)"`
	PreventOverflow *bool  `name:"prevent-overflow" negatable:"" help:"Keep the row within one page or column; use --no-prevent-overflow to clear"`
	Tab             string `name:"tab" help:"Target a specific tab by title or ID (see docs list-tabs)"`
}

type docsTableRowActionResult struct {
	TableIndex int
	Value      any
}

func (c *DocsTableRowStyleCmd) Run(ctx context.Context, flags *RootFlags) error {
	docID, err := validateDocsTableMutationArgs(c.DocID, c.Table)
	if err != nil {
		return err
	}
	style, fields, err := c.buildStyle()
	if err != nil {
		return err
	}
	if c.Row != nil && *c.Row == 0 {
		return usage("--row cannot be 0")
	}
	row := any(literalAll)
	if c.Row != nil {
		row = *c.Row
	}
	if dryRunErr := dryRunExit(ctx, flags, "docs.table-row.style", map[string]any{
		"documentId":      docID,
		"table":           c.Table,
		"row":             row,
		"minHeight":       c.MinHeight,
		"preventOverflow": c.PreventOverflow,
		"tab":             c.Tab,
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
		rowIndices, resolvedRow, resolveErr := resolveDocsTableStyleRow(target.table, c.Row)
		if resolveErr != nil {
			return resolveErr
		}
		requests = append(requests, &docs.Request{UpdateTableRowStyle: &docs.UpdateTableRowStyleRequest{
			TableStartLocation: &docs.Location{Index: target.startIdx, TabId: loaded.tabID},
			RowIndices:         rowIndices,
			TableRowStyle:      style,
			Fields:             strings.Join(fields, ","),
		}})
		results = append(results, docsTableRowActionResult{TableIndex: target.index, Value: resolvedRow})
	}
	if _, err := executeDocsTableRequests(ctx, svc, docID, loaded.full.RevisionId, requests); err != nil {
		return fmt.Errorf("style table row: %w", err)
	}
	return writeDocsTableRowActionResult(ctx, docID, loaded.tabID, "style", "row", results)
}

func (c *DocsTableRowStyleCmd) buildStyle() (*docs.TableRowStyle, []string, error) {
	style := &docs.TableRowStyle{}
	fields := []string{}
	if raw := strings.TrimSpace(c.MinHeight); raw != "" {
		dimension, _, err := parseDocsDimension("min-height", raw, true)
		if err != nil {
			return nil, nil, err
		}
		style.MinRowHeight = dimension
		fields = append(fields, "minRowHeight")
	}
	if c.PreventOverflow != nil {
		style.PreventOverflow = *c.PreventOverflow
		style.ForceSendFields = append(style.ForceSendFields, "PreventOverflow")
		fields = append(fields, "preventOverflow")
	}
	if len(fields) == 0 {
		return nil, nil, usage("no row style flags provided")
	}
	return style, fields, nil
}

func resolveDocsTableStyleRow(table *docs.Table, requested *int) ([]int64, any, error) {
	if table == nil || len(table.TableRows) == 0 {
		return nil, nil, usage("target table has no rows")
	}
	if requested == nil {
		return nil, literalAll, nil
	}
	resolved := *requested
	if resolved < 0 {
		resolved = len(table.TableRows) + resolved + 1
	}
	if resolved < 1 || resolved > len(table.TableRows) {
		return nil, nil, usagef("row %d out of range (table has %d rows)", *requested, len(table.TableRows))
	}
	return []int64{int64(resolved - 1)}, resolved, nil
}

func writeDocsTableRowActionResult(
	ctx context.Context,
	docID, tabID, action, valueName string,
	results []docsTableRowActionResult,
) error {
	sort.Slice(results, func(i, j int) bool {
		return results[i].TableIndex < results[j].TableIndex
	})
	tables := make([]map[string]any, 0, len(results))
	for _, result := range results {
		tables = append(tables, map[string]any{"tableIndex": result.TableIndex, valueName: result.Value})
	}
	if outfmt.IsJSON(ctx) {
		payload := map[string]any{
			"documentId": docID,
			"action":     action,
			"target":     "row",
			"updated":    true,
			"tables":     tables,
		}
		if len(results) == 1 {
			payload["tableIndex"] = results[0].TableIndex
			payload[valueName] = results[0].Value
		}
		if tabID != "" {
			payload["tabId"] = tabID
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), payload)
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("documentId\t%s", docID)
	u.Out().Linef("action\t%s", action)
	u.Out().Linef("target\trow")
	for _, result := range results {
		u.Out().Linef("table\t%d\t%s\t%v", result.TableIndex, valueName, result.Value)
	}
	u.Out().Linef("updated\ttrue")
	if tabID != "" {
		u.Out().Linef("tabId\t%s", tabID)
	}
	return nil
}
