package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

func copyDataValidation(ctx context.Context, svc *sheets.Service, spreadsheetID, sourceA1, destA1 string) error {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	sourceGrid, err := resolveGridRangeWithCatalog(sourceA1, catalog, "copy-validation-from")
	if err != nil {
		return err
	}

	destRange, err := parseSheetRange(destA1, "updated")
	if err != nil {
		return err
	}
	destGrid, err := gridRangeFromMap(destRange, catalog.SheetIDsByTitle, "updated")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				CopyPaste: &sheets.CopyPasteRequest{
					Source:      sourceGrid,
					Destination: destGrid,
					PasteType:   "PASTE_DATA_VALIDATION",
				},
			},
		},
	}

	_, err = svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return fmt.Errorf("apply data validation: %w", err)
	}
	return nil
}

func fetchSheetIDMap(ctx context.Context, svc *sheets.Service, spreadsheetID string) (map[string]int64, error) {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return nil, err
	}
	return catalog.SheetIDsByTitle, nil
}

func toGridRange(r a1Range, sheetID int64) *sheets.GridRange {
	gr := &sheets.GridRange{
		SheetId:          sheetID,
		ForceSendFields:  []string{"SheetId"}, // sheetId can be 0 for the first sheet, but still must be sent.
		StartRowIndex:    0,
		EndRowIndex:      0,
		StartColumnIndex: 0,
		EndColumnIndex:   0,
	}
	if r.StartRow > 0 {
		gr.StartRowIndex = int64(r.StartRow - 1)
	}
	if r.EndRow > 0 {
		gr.EndRowIndex = int64(r.EndRow)
	}
	if r.StartCol > 0 {
		gr.StartColumnIndex = int64(r.StartCol - 1)
	}
	if r.EndCol > 0 {
		gr.EndColumnIndex = int64(r.EndCol)
	}
	return gr
}

func parseSheetRange(a1, label string) (a1Range, error) {
	r, err := parseA1Range(a1)
	if err != nil {
		return a1Range{}, fmt.Errorf("parse %s range: %w", label, err)
	}
	if strings.TrimSpace(r.SheetName) == "" {
		return a1Range{}, fmt.Errorf("%s range must include a sheet name", label)
	}
	return r, nil
}

func gridRangeFromMap(r a1Range, sheetIDs map[string]int64, label string) (*sheets.GridRange, error) {
	sheetID, ok := sheetIDs[r.SheetName]
	if !ok {
		return nil, fmt.Errorf("unknown sheet %q in %s range", r.SheetName, label)
	}
	return toGridRange(r, sheetID), nil
}
