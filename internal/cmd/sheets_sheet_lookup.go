package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

func resolveSheetIDByNameOrFirst(ctx context.Context, svc *sheets.Service, spreadsheetID, sheetName string) (int64, string, error) {
	call := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(sheetId,title))")
	if ctx != nil {
		call = call.Context(ctx)
	}
	resp, err := call.Do()
	if err != nil {
		return 0, "", fmt.Errorf("get spreadsheet metadata: %w", err)
	}

	firstTitle := ""
	var firstID int64
	wanted := strings.TrimSpace(sheetName)
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		if firstTitle == "" {
			firstTitle = sh.Properties.Title
			firstID = sh.Properties.SheetId
		}
		if wanted != "" && sh.Properties.Title == wanted {
			return sh.Properties.SheetId, sh.Properties.Title, nil
		}
	}

	if wanted != "" {
		return 0, "", usagef("unknown sheet %q", wanted)
	}
	if firstTitle == "" {
		return 0, "", fmt.Errorf("spreadsheet has no sheets")
	}
	return firstID, firstTitle, nil
}

func forceSendSheetPropertiesSheetID(props *sheets.SheetProperties) {
	if props == nil || props.SheetId != 0 {
		return
	}
	for _, field := range props.ForceSendFields {
		if field == "SheetId" {
			return
		}
	}
	props.ForceSendFields = append(props.ForceSendFields, "SheetId")
}

func forceSendDimensionRangeZeroes(dimRange *sheets.DimensionRange) {
	if dimRange == nil {
		return
	}
	if dimRange.SheetId == 0 {
		dimRange.ForceSendFields = append(dimRange.ForceSendFields, "SheetId")
	}
	if dimRange.StartIndex == 0 {
		dimRange.ForceSendFields = append(dimRange.ForceSendFields, "StartIndex")
	}
}
