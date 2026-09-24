package cmd

import (
	"context"

	"google.golang.org/api/sheets/v4"
)

type chartSheetResolution struct {
	SheetID        int64
	HasSheetIDZero bool
}

func firstSheetResolution(ctx context.Context, svc *sheets.Service, spreadsheetID string) (chartSheetResolution, error) {
	resp, err := fetchSheetsMutationMetadata(ctx, svc, spreadsheetID, "sheets(properties(sheetId,title))")
	if err != nil {
		return chartSheetResolution{}, err
	}

	var res chartSheetResolution
	var found bool
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if !found {
			res.SheetID = sheet.Properties.SheetId
			found = true
		}
		if sheet.Properties.SheetId == 0 {
			res.HasSheetIDZero = true
		}
	}
	if found {
		return res, nil
	}
	return chartSheetResolution{}, usage("spreadsheet has no sheets")
}

func findChartSheetResolution(ctx context.Context, svc *sheets.Service, spreadsheetID string, chartID int64) (chartSheetResolution, error) {
	resp, err := fetchSheetsMutationMetadata(ctx, svc, spreadsheetID, "sheets(properties(sheetId,title),charts(chartId))")
	if err != nil {
		return chartSheetResolution{}, err
	}

	var res chartSheetResolution
	var found bool
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if sheet.Properties.SheetId == 0 {
			res.HasSheetIDZero = true
		}
		for _, chart := range sheet.Charts {
			if chart != nil && chart.ChartId == chartID {
				res.SheetID = sheet.Properties.SheetId
				found = true
			}
		}
	}
	if found {
		return res, nil
	}
	return chartSheetResolution{}, usagef("chart %d not found", chartID)
}

func resolveChartSheetResolution(ctx context.Context, svc *sheets.Service, spreadsheetID, sheetName string) (chartSheetResolution, error) {
	if sheetName == "" {
		return firstSheetResolution(ctx, svc, spreadsheetID)
	}

	resp, err := fetchSheetsMutationMetadata(ctx, svc, spreadsheetID, "sheets(properties(sheetId,title))")
	if err != nil {
		return chartSheetResolution{}, err
	}

	var res chartSheetResolution
	var found bool
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}
		if sheet.Properties.SheetId == 0 {
			res.HasSheetIDZero = true
		}
		if sheet.Properties.Title == sheetName {
			res.SheetID = sheet.Properties.SheetId
			found = true
		}
	}
	if !found {
		return chartSheetResolution{}, usagef("unknown sheet %q", sheetName)
	}
	return res, nil
}
