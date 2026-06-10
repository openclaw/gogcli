package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"
)

// pasteNormal is the default paste type / orientation keyword shared by the
// struct default, orientation fallback, and the type allow-list.
const pasteNormal = "NORMAL"

type SheetsCopyPasteCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Source        string `arg:"" name:"source" help:"Source range (eg. Sheet1!A2:H71)"`
	Dest          string `arg:"" name:"dest" help:"Destination range (eg. Sheet1!A2:H120). A destination larger than the source tiles the source to fill it — use this to fill formulas down or across with relative references adjusted."`
	Type          string `name:"type" help:"Paste type: NORMAL, VALUES, FORMAT, FORMULA, NO_BORDERS, DATA_VALIDATION, CONDITIONAL_FORMATTING" default:"NORMAL"`
	Transpose     bool   `name:"transpose" help:"Paste transposed (swap rows and columns)"`
}

func (c *SheetsCopyPasteCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	source := cleanRange(c.Source)
	dest := cleanRange(c.Dest)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(source) == "" {
		return usage("empty source range")
	}
	if strings.TrimSpace(dest) == "" {
		return usage("empty dest range")
	}

	pasteType, err := normalizePasteType(c.Type)
	if err != nil {
		return err
	}
	orientation := pasteNormal
	if c.Transpose {
		orientation = "TRANSPOSE"
	}

	srcInfo, err := parseSheetRange(source, "copy-paste source")
	if err != nil {
		return err
	}
	dstInfo, err := parseSheetRange(dest, "copy-paste dest")
	if err != nil {
		return err
	}

	return runSheetsMutation(ctx, flags, "sheets.copy-paste", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"source":         source,
		"dest":           dest,
		"type":           pasteType,
		"orientation":    orientation,
	}, func(ctx context.Context, svc *sheets.Service) (map[string]any, string, error) {
		catalog, err := fetchSpreadsheetRangeCatalog(ctx, svc, spreadsheetID)
		if err != nil {
			return nil, "", err
		}
		srcGrid, err := gridRangeFromMap(srcInfo, catalog.SheetIDsByTitle, "copy-paste source")
		if err != nil {
			return nil, "", err
		}
		dstGrid, err := gridRangeFromMap(dstInfo, catalog.SheetIDsByTitle, "copy-paste dest")
		if err != nil {
			return nil, "", err
		}
		srcGrid = boundGridRangeToSheet(srcGrid, catalog)
		dstGrid = boundGridRangeToSheet(dstGrid, catalog)
		requests := []*sheets.Request{{
			CopyPaste: &sheets.CopyPasteRequest{
				Source:           srcGrid,
				Destination:      dstGrid,
				PasteType:        pasteType,
				PasteOrientation: orientation,
			},
		}}
		tableManagedRules := 0
		if pasteCarriesDataValidation(pasteType) {
			spans, err := fetchTableValidationSpans(ctx, svc, spreadsheetID)
			if err != nil {
				return nil, "", err
			}
			copyOptions, err := resolveTableValidationCopyOptions(
				ctx,
				svc,
				spreadsheetID,
				srcGrid,
				dstGrid,
				spans,
				catalog,
				c.Transpose,
			)
			if err != nil {
				return nil, "", err
			}
			supplemental, err := buildTableValidationCopyRequests(
				srcGrid,
				dstGrid,
				c.Transpose,
				spans,
				copyOptions,
			)
			if err != nil {
				return nil, "", err
			}
			requests = append(requests, supplemental...)
			tableManagedRules = len(supplemental)
		}
		req := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: requests,
		}
		if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
			return nil, "", err
		}
		return map[string]any{
			"source":                         source,
			"dest":                           dest,
			"type":                           pasteType,
			"orientation":                    orientation,
			"tableManagedValidationRequests": tableManagedRules,
		}, fmt.Sprintf("Copied %s → %s (%s)", source, dest, pasteType), nil
	})
}

func pasteCarriesDataValidation(pasteType string) bool {
	switch pasteType {
	case "PASTE_NORMAL", "PASTE_FORMAT", "PASTE_NO_BORDERS", "PASTE_DATA_VALIDATION":
		return true
	default:
		return false
	}
}

func normalizePasteType(raw string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	if v == "" {
		v = pasteNormal
	}
	v = strings.TrimPrefix(v, "PASTE_")
	switch v {
	case pasteNormal, "VALUES", "FORMAT", "NO_BORDERS", "FORMULA", "DATA_VALIDATION", "CONDITIONAL_FORMATTING":
		return "PASTE_" + v, nil
	default:
		return "", usagef("invalid --type %q (expected NORMAL, VALUES, FORMAT, FORMULA, NO_BORDERS, DATA_VALIDATION, or CONDITIONAL_FORMATTING)", raw)
	}
}
