package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

func copyDataValidation(ctx context.Context, svc *sheets.Service, spreadsheetID, sourceA1, destA1 string) error {
	sourceRange, err := parseA1Range(sourceA1)
	if err != nil {
		return fmt.Errorf("parse copy-validation-from: %w", err)
	}
	destRange, err := parseA1Range(destA1)
	if err != nil {
		return fmt.Errorf("parse updated range: %w", err)
	}

	if strings.TrimSpace(sourceRange.SheetName) == "" {
		return fmt.Errorf("copy-validation-from must include a sheet name")
	}
	if strings.TrimSpace(destRange.SheetName) == "" {
		return fmt.Errorf("updated range missing sheet name")
	}

	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}

	sourceSheetID, ok := sheetIDs[sourceRange.SheetName]
	if !ok {
		return fmt.Errorf("unknown sheet %q in copy-validation-from", sourceRange.SheetName)
	}
	destSheetID, ok := sheetIDs[destRange.SheetName]
	if !ok {
		return fmt.Errorf("unknown sheet %q in updated range", destRange.SheetName)
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				CopyPaste: &sheets.CopyPasteRequest{
					Source:      toGridRange(sourceRange, sourceSheetID),
					Destination: toGridRange(destRange, destSheetID),
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
	call := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title))")
	if ctx != nil {
		call = call.Context(ctx)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("get spreadsheet metadata: %w", err)
	}

	ids := make(map[string]int64, len(resp.Sheets))
	for _, sheet := range resp.Sheets {
		if sheet.Properties == nil {
			continue
		}
		ids[sheet.Properties.Title] = sheet.Properties.SheetId
	}
	return ids, nil
}

func toGridRange(r a1Range, sheetID int64) *sheets.GridRange {
	return &sheets.GridRange{
		SheetId:          sheetID,
		StartRowIndex:    int64(r.StartRow - 1),
		EndRowIndex:      int64(r.EndRow),
		StartColumnIndex: int64(r.StartCol - 1),
		EndColumnIndex:   int64(r.EndCol),
	}
}
