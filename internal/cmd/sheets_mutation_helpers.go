package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

func runSheetsMutation(
	ctx context.Context,
	flags *RootFlags,
	op string,
	dryRunPayload map[string]any,
	run func(context.Context, *sheets.Service) (map[string]any, string, error),
) error {
	u := ui.FromContext(ctx)
	if dryRunErr := dryRunExit(ctx, flags, op, dryRunPayload); dryRunErr != nil {
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

	jsonPayload, text, err := run(ctx, svc)
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), jsonPayload)
	}
	u.Out().Linef("%s", text)
	return nil
}

func runSheetsSpreadsheetList[T any](
	ctx context.Context,
	flags *RootFlags,
	rawSpreadsheetID string,
	sheetName string,
	fields string,
	jsonKey string,
	emptyMessage string,
	extract func(*sheets.Spreadsheet, string) []T,
	columns []outfmt.Column[T],
) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(rawSpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}

	_, svc, err := requireSheetsService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields(googleapi.Field(fields)).Context(ctx).Do()
	if err != nil {
		return err
	}

	items := extract(resp, strings.TrimSpace(sheetName))
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{jsonKey: items})
	}
	if len(items) == 0 {
		ui.FromContext(ctx).Err().Println(emptyMessage)
		return nil
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), items, columns)
}

func resolveSheetIDByNameOrFirst(ctx context.Context, svc *sheets.Service, spreadsheetID, sheetName string) (int64, string, error) {
	catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
	if err != nil {
		return 0, "", err
	}
	return resolveSheetIDByNameOrFirstWithCatalog(catalog, sheetName)
}

func resolveSheetIDByNameOrFirstWithCatalog(catalog *spreadsheetRangeCatalog, sheetName string) (int64, string, error) {
	if catalog == nil {
		return 0, "", fmt.Errorf("missing spreadsheet range catalog")
	}

	firstTitle := ""
	var firstID int64
	wanted := strings.TrimSpace(sheetName)
	for _, props := range catalog.Sheets {
		if props == nil {
			continue
		}
		if firstTitle == "" {
			firstTitle = props.Title
			firstID = props.SheetId
		}
		if wanted != "" && props.Title == wanted {
			return props.SheetId, props.Title, nil
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

func applySheetsBatchUpdate(ctx context.Context, svc *sheets.Service, spreadsheetID string, req *sheets.BatchUpdateSpreadsheetRequest) error {
	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Context(ctx).Do(); err != nil {
		return err
	}
	return nil
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
