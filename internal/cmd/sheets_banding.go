package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsBandingCmd struct {
	List  SheetsBandingListCmd  `cmd:"" default:"withargs" help:"List alternating color banded ranges"`
	Set   SheetsBandingSetCmd   `cmd:"" name:"set" aliases:"add,create" help:"Apply alternating colors to a range"`
	Clear SheetsBandingClearCmd `cmd:"" name:"clear" aliases:"delete,rm,remove" help:"Remove alternating color banding"`
}

type SheetsBandingSetCmd struct {
	SpreadsheetID        string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range                string `arg:"" name:"range" help:"A1 range with sheet name (e.g. Sheet1!A1:H20)"`
	RowPropertiesJSON    string `name:"row-properties-json" help:"Sheets API BandingProperties JSON for row colors"`
	ColumnPropertiesJSON string `name:"column-properties-json" help:"Sheets API BandingProperties JSON for column colors"`
}

func (c *SheetsBandingSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}
	parsedRange, err := parseSheetRange(rangeSpec, "banding")
	if err != nil {
		return err
	}
	rowProps, colProps, err := bandingProperties(c.RowPropertiesJSON, c.ColumnPropertiesJSON)
	if err != nil {
		return err
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.banding.set", map[string]any{
		"spreadsheet_id":    spreadsheetID,
		"range":             rangeSpec,
		"row_properties":    rowProps,
		"column_properties": colProps,
	}); dryErr != nil {
		return dryErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := gridRangeFromMap(parsedRange, sheetIDs, "banding")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{{
		AddBanding: &sheets.AddBandingRequest{BandedRange: &sheets.BandedRange{
			Range:            gridRange,
			RowProperties:    rowProps,
			ColumnProperties: colProps,
		}},
	}}}
	resp, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do()
	if err != nil {
		return err
	}
	var bandedRangeID int64
	if len(resp.Replies) > 0 && resp.Replies[0].AddBanding != nil && resp.Replies[0].AddBanding.BandedRange != nil {
		bandedRangeID = resp.Replies[0].AddBanding.BandedRange.BandedRangeId
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"bandedRangeId": bandedRangeID,
			"range":         rangeSpec,
		})
	}
	u.Out().Printf("Applied banding %d to %s", bandedRangeID, rangeSpec)
	return nil
}

type SheetsBandingListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Only list banding from this sheet"`
}

func (c *SheetsBandingListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title),bandedRanges)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	items := bandingItems(resp, strings.TrimSpace(c.Sheet))
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"bandedRanges": items})
	}
	if len(items) == 0 {
		u.Err().Println("No banded ranges")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "BANDED_RANGE_ID\tSHEET\tRANGE")
	for _, item := range items {
		fmt.Fprintf(w, "%d\t%s\t%s\n", item.BandedRangeID, item.SheetTitle, item.A1)
	}
	return nil
}

type SheetsBandingClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	BandedRangeID int64  `name:"id" help:"Banded range ID to remove"`
	Sheet         string `name:"sheet" help:"Sheet name for --all"`
	All           bool   `name:"all" help:"Remove all banding from the sheet"`
}

func (c *SheetsBandingClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sheetName := strings.TrimSpace(c.Sheet)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if c.BandedRangeID > 0 && c.All {
		return usage("use either --id or --all, not both")
	}
	if c.BandedRangeID <= 0 && !c.All {
		return usage("provide --id or --all")
	}
	if c.All && sheetName == "" {
		return usage("--sheet is required with --all")
	}

	requests := []*sheets.Request{}
	var removed int
	if c.BandedRangeID > 0 {
		requests = append(requests, bandingDeleteRequest(c.BandedRangeID))
		removed = 1
	} else {
		account, err := requireAccount(flags)
		if err != nil {
			return err
		}
		svc, err := newSheetsService(ctx, account)
		if err != nil {
			return err
		}
		resp, err := svc.Spreadsheets.Get(spreadsheetID).
			Fields("sheets(properties(title),bandedRanges(bandedRangeId))").
			Context(ctx).
			Do()
		if err != nil {
			return err
		}
		ids, found := bandingIDsForSheet(resp, sheetName)
		if !found {
			return usagef("unknown sheet %q", sheetName)
		}
		for _, id := range ids {
			requests = append(requests, bandingDeleteRequest(id))
		}
		removed = len(requests)
	}

	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"removed": 0})
		}
		ui.FromContext(ctx).Out().Println("No banded ranges to remove")
		return nil
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "sheets.banding.clear", map[string]any{
		"spreadsheet_id":  spreadsheetID,
		"banded_range_id": c.BandedRangeID,
		"sheet":           sheetName,
		"all":             c.All,
		"removed":         removed,
	}, "remove banding"); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"removed":       removed,
		})
	}
	ui.FromContext(ctx).Out().Printf("Removed %d banded ranges", removed)
	return nil
}

type bandingItem struct {
	BandedRangeID    int64                     `json:"bandedRangeId"`
	SheetID          int64                     `json:"sheetId"`
	SheetTitle       string                    `json:"sheetTitle"`
	A1               string                    `json:"a1,omitempty"`
	Range            *sheets.GridRange         `json:"range,omitempty"`
	RowProperties    *sheets.BandingProperties `json:"rowProperties,omitempty"`
	ColumnProperties *sheets.BandingProperties `json:"columnProperties,omitempty"`
}

func bandingProperties(rowJSON, columnJSON string) (*sheets.BandingProperties, *sheets.BandingProperties, error) {
	rowProps, err := parseBandingProperties(rowJSON, defaultRowBandingProperties())
	if err != nil {
		return nil, nil, fmt.Errorf("invalid --row-properties-json: %w", err)
	}
	colProps, err := parseBandingProperties(columnJSON, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid --column-properties-json: %w", err)
	}
	if rowProps == nil && colProps == nil {
		return nil, nil, usage("provide row or column banding properties")
	}
	return rowProps, colProps, nil
}

func parseBandingProperties(raw string, fallback *sheets.BandingProperties) (*sheets.BandingProperties, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	b, err := resolveInlineOrFileBytes(raw)
	if err != nil {
		return nil, err
	}
	var props sheets.BandingProperties
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&props); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return &props, nil
}

func defaultRowBandingProperties() *sheets.BandingProperties {
	return &sheets.BandingProperties{
		HeaderColorStyle:     &sheets.ColorStyle{RgbColor: &sheets.Color{Red: 0.88, Green: 0.93, Blue: 1}},
		FirstBandColorStyle:  &sheets.ColorStyle{RgbColor: &sheets.Color{Red: 1, Green: 1, Blue: 1}},
		SecondBandColorStyle: &sheets.ColorStyle{RgbColor: &sheets.Color{Red: 0.96, Green: 0.98, Blue: 1}},
	}
}

func bandingItems(resp *sheets.Spreadsheet, onlySheet string) []bandingItem {
	items := make([]bandingItem, 0)
	if resp == nil {
		return items
	}
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		sheetTitle := sheet.Properties.Title
		if onlySheet != "" && sheetTitle != onlySheet {
			continue
		}
		for _, br := range sheet.BandedRanges {
			if br == nil {
				continue
			}
			items = append(items, bandingItem{
				BandedRangeID:    br.BandedRangeId,
				SheetID:          sheet.Properties.SheetId,
				SheetTitle:       sheetTitle,
				A1:               gridRangeToA1(sheetTitle, br.Range),
				Range:            br.Range,
				RowProperties:    br.RowProperties,
				ColumnProperties: br.ColumnProperties,
			})
		}
	}
	return items
}

func bandingIDsForSheet(resp *sheets.Spreadsheet, sheetName string) ([]int64, bool) {
	if resp == nil {
		return nil, false
	}
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil || sheet.Properties.Title != sheetName {
			continue
		}
		ids := make([]int64, 0, len(sheet.BandedRanges))
		for _, br := range sheet.BandedRanges {
			if br != nil {
				ids = append(ids, br.BandedRangeId)
			}
		}
		return ids, true
	}
	return nil, false
}

func bandingDeleteRequest(id int64) *sheets.Request {
	return &sheets.Request{DeleteBanding: &sheets.DeleteBandingRequest{BandedRangeId: id}}
}
