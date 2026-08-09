package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/sheetsa1"
	"github.com/openclaw/gogcli/internal/ui"
)

type SheetsNamedRangesCmd struct {
	List   SheetsNamedRangesListCmd   `cmd:"" default:"withargs" help:"List named ranges"`
	Get    SheetsNamedRangesGetCmd    `cmd:"" name:"get" aliases:"show,info" help:"Get a named range"`
	Add    SheetsNamedRangesAddCmd    `cmd:"" name:"add" aliases:"create,new" help:"Add a named range"`
	Update SheetsNamedRangesUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a named range"`
	Delete SheetsNamedRangesDeleteCmd `cmd:"" name:"delete" aliases:"rm,remove,del" help:"Delete a named range"`
}

type SheetsNamedRangesListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

func (c *SheetsNamedRangesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	items := make([]namedRangeItem, 0, len(catalog.NamedRanges))
	for _, nr := range catalog.NamedRanges {
		if nr == nil {
			continue
		}
		items = append(items, namedRangeToItem(nr, catalog))
	}
	// Stable ordering: name, then id.
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].NamedRangeID < items[j].NamedRangeID
		}
		return items[i].Name < items[j].Name
	})

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"namedRanges": items})
	}

	if len(items) == 0 {
		u.Err().Println("No named ranges")
		return nil
	}

	return outfmt.WriteTable(ctx, stdoutWriter(ctx), items, sheetsNamedRangeColumns())
}

type SheetsNamedRangesGetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	NameOrID      string `arg:"" name:"nameOrId" help:"Named range name or ID"`
}

func (c *SheetsNamedRangesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.NameOrID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty nameOrId")
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	nr, found, err := resolveNamedRangeByNameOrID(in, catalog.NamedRanges)
	if err != nil {
		return err
	}
	if !found || nr == nil {
		return usagef("unknown named range %q", in)
	}

	it := namedRangeToItem(nr, catalog)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"namedRange": it})
	}

	u.Out().Linef("name\t%s", it.Name)
	u.Out().Linef("id\t%s", it.NamedRangeID)
	u.Out().Linef("sheet\t%s", it.SheetTitle)
	u.Out().Linef("a1\t%s", it.A1)
	return nil
}

type SheetsNamedRangesAddCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Name          string `arg:"" name:"name" help:"Named range name"`
	Range         string `arg:"" name:"range" help:"A1 range (must include sheet name; e.g. Sheet1!A1:B2 or Sheet1!A:C)"`
}

func (c *SheetsNamedRangesAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	name := strings.TrimSpace(c.Name)
	rangeSpec := cleanRange(strings.TrimSpace(c.Range))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if name == "" {
		return usage("empty name")
	}
	if rangeSpec == "" {
		return usage("empty range")
	}
	if _, err := parseSheetRange(rangeSpec, "range"); err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.named-ranges.add", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"name":           name,
		"range":          rangeSpec,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	r, err := parseSheetRange(rangeSpec, "range")
	if err != nil {
		return err
	}
	gridRange, err := gridRangeFromMap(r, catalog.SheetIDsByTitle, "range")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddNamedRange: &sheets.AddNamedRangeRequest{
					NamedRange: &sheets.NamedRange{
						Name:  name,
						Range: gridRange,
					},
				},
			},
		},
	}

	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}

	created := &sheets.NamedRange{Name: name, Range: gridRange}
	if resp != nil && len(resp.Replies) == 1 && resp.Replies[0] != nil && resp.Replies[0].AddNamedRange != nil && resp.Replies[0].AddNamedRange.NamedRange != nil {
		created = resp.Replies[0].AddNamedRange.NamedRange
	}

	it := namedRangeToItem(created, catalog)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"namedRange": it})
	}

	u.Out().Linef("name\t%s", it.Name)
	u.Out().Linef("id\t%s", it.NamedRangeID)
	u.Out().Linef("a1\t%s", it.A1)
	return nil
}

type SheetsNamedRangesUpdateCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	NameOrID      string `arg:"" name:"nameOrId" help:"Named range name or ID"`
	NewName       string `name:"name" help:"New name"`
	NewRange      string `name:"range" help:"New A1 range (must include sheet name; e.g. Sheet1!A1:B2 or Sheet1!A:C)"`
}

func (c *SheetsNamedRangesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.NameOrID)
	newName := strings.TrimSpace(c.NewName)
	newRangeSpec := cleanRange(strings.TrimSpace(c.NewRange))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty nameOrId")
	}
	if newName == "" && newRangeSpec == "" {
		return usage("provide --name and/or --range")
	}
	if newRangeSpec != "" {
		if _, err := parseSheetRange(newRangeSpec, "range"); err != nil {
			return err
		}
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.named-ranges.update", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"name_or_id":     in,
		"name":           newName,
		"range":          newRangeSpec,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	existing, found, err := resolveNamedRangeByNameOrID(in, catalog.NamedRanges)
	if err != nil {
		return err
	}
	if !found || existing == nil {
		return usagef("unknown named range %q", in)
	}
	id := strings.TrimSpace(existing.NamedRangeId)

	update := &sheets.NamedRange{NamedRangeId: id}
	fields := make([]string, 0, 2)
	if newName != "" {
		update.Name = newName
		fields = append(fields, "name")
	}
	if newRangeSpec != "" {
		parsedRange, parseErr := parseSheetRange(newRangeSpec, "range")
		if parseErr != nil {
			return parseErr
		}
		gridRange, gridErr := gridRangeFromMap(parsedRange, catalog.SheetIDsByTitle, "range")
		if gridErr != nil {
			return gridErr
		}
		update.Range = gridRange
		fields = append(fields, "range")
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				UpdateNamedRange: &sheets.UpdateNamedRangeRequest{
					NamedRange: update,
					Fields:     strings.Join(fields, ","),
				},
			},
		},
	}

	if _, batchErr := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); batchErr != nil {
		return batchErr
	}

	updatedCatalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	updated, found, err := resolveNamedRangeByNameOrID(id, updatedCatalog.NamedRanges)
	if err != nil {
		return err
	}
	if !found || updated == nil {
		return fmt.Errorf("updated named range not found (id=%q)", id)
	}

	it := namedRangeToItem(updated, updatedCatalog)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"namedRange": it})
	}

	u.Out().Linef("name\t%s", it.Name)
	u.Out().Linef("id\t%s", it.NamedRangeID)
	u.Out().Linef("a1\t%s", it.A1)
	return nil
}

type SheetsNamedRangesDeleteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	NameOrID      string `arg:"" name:"nameOrId" help:"Named range name or ID"`
}

func (c *SheetsNamedRangesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	in := strings.TrimSpace(c.NameOrID)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if in == "" {
		return usage("empty nameOrId")
	}

	if dryRunErr := dryRunExit(ctx, flags, "sheets.named-ranges.delete", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"name_or_id":     in,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := sheetsService(ctx, account)
	if err != nil {
		return err
	}

	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	existing, found, err := resolveNamedRangeByNameOrID(in, catalog.NamedRanges)
	if err != nil {
		return err
	}
	if !found || existing == nil {
		return usagef("unknown named range %q", in)
	}
	id := strings.TrimSpace(existing.NamedRangeId)

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				DeleteNamedRange: &sheets.DeleteNamedRangeRequest{NamedRangeId: id},
			},
		},
	}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"deleted": map[string]any{"namedRangeId": id, "name": strings.TrimSpace(existing.Name)},
		})
	}

	u.Out().Linef("deleted\t%s", id)
	return nil
}

type namedRangeItem struct {
	Name         string `json:"name"`
	NamedRangeID string `json:"namedRangeId"`
	SheetID      int64  `json:"sheetId"`
	SheetTitle   string `json:"sheetTitle"`

	StartRowIndex int64 `json:"startRowIndex"`
	EndRowIndex   int64 `json:"endRowIndex"`

	StartColIndex int64 `json:"startColumnIndex"`
	EndColIndex   int64 `json:"endColumnIndex"`

	A1 string `json:"a1"`
}

func namedRangeToItem(nr *sheets.NamedRange, catalog *spreadsheetRangeCatalog) namedRangeItem {
	if nr == nil {
		return namedRangeItem{}
	}
	out := namedRangeItem{
		Name:         strings.TrimSpace(nr.Name),
		NamedRangeID: strings.TrimSpace(nr.NamedRangeId),
	}
	if catalog == nil || nr.Range == nil {
		return out
	}
	out.SheetID = nr.Range.SheetId
	out.SheetTitle = catalog.SheetTitlesByID[nr.Range.SheetId]
	out.StartRowIndex = nr.Range.StartRowIndex
	out.EndRowIndex = nr.Range.EndRowIndex
	out.StartColIndex = nr.Range.StartColumnIndex
	out.EndColIndex = nr.Range.EndColumnIndex
	if out.SheetTitle != "" {
		out.A1 = sheetsa1.FormatGridRange(out.SheetTitle, nr.Range)
	}
	return out
}
