package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsConditionalCmd struct {
	List  SheetsConditionalListCmd  `cmd:"" default:"withargs" help:"List conditional formatting rules"`
	Add   SheetsConditionalAddCmd   `cmd:"" name:"add" aliases:"create,new" help:"Add a conditional formatting rule"`
	Clear SheetsConditionalClearCmd `cmd:"" name:"clear" aliases:"delete,rm,remove" help:"Remove conditional formatting rules"`
}

type SheetsConditionalAddCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `arg:"" name:"range" help:"A1 range with sheet name (e.g. Sheet1!A2:J)"`
	Type          string `name:"type" required:"" help:"Rule type: text-eq|text-contains|text-starts-with|text-ends-with|number-eq|number-gt|number-gte|number-lt|number-lte|blank|not-blank|custom-formula"`
	Expr          string `name:"expr" help:"Expression value or custom formula (omit for blank/not-blank)"`
	FormatJSON    string `name:"format-json" required:"" help:"CellFormat JSON (inline or @file)"`
	FormatFields  string `name:"format-fields" help:"Format field mask for force-sending zero/false fields (e.g. backgroundColor,textFormat.bold)"`
	Index         int64  `name:"index" help:"Insert rule at this priority index" default:"0"`
}

func (c *SheetsConditionalAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	rangeSpec := cleanRange(c.Range)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if strings.TrimSpace(rangeSpec) == "" {
		return usage("empty range")
	}
	if c.Index < 0 {
		return usage("--index must be zero or greater")
	}

	parsedRange, err := parseSheetRange(rangeSpec, "conditional-format")
	if err != nil {
		return err
	}
	format, formatFields, err := parseConditionalFormat(c.FormatJSON, c.FormatFields)
	if err != nil {
		return err
	}
	conditionType, values, err := conditionalCondition(strings.TrimSpace(c.Type), strings.TrimSpace(c.Expr))
	if err != nil {
		return err
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.conditional-format.add", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"type":           conditionType,
		"values":         values,
		"format_fields":  formatFields,
		"index":          c.Index,
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
	gridRange, err := gridRangeFromMap(parsedRange, sheetIDs, "conditional-format")
	if err != nil {
		return err
	}

	req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{{
		AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
			Rule: &sheets.ConditionalFormatRule{
				BooleanRule: &sheets.BooleanRule{
					Condition: &sheets.BooleanCondition{
						Type:   conditionType,
						Values: values,
					},
					Format: format,
				},
				Ranges: []*sheets.GridRange{gridRange},
			},
			Index: c.Index,
		},
	}}}

	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"type":          conditionType,
			"index":         c.Index,
		})
	}
	u.Out().Printf("Added conditional format rule to %s", rangeSpec)
	return nil
}

type SheetsConditionalListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Only list rules from this sheet"`
}

func (c *SheetsConditionalListCmd) Run(ctx context.Context, flags *RootFlags) error {
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
		Fields("sheets(properties(sheetId,title),conditionalFormats)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}

	items := conditionalRuleItems(resp, strings.TrimSpace(c.Sheet))
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"rules": items})
	}
	if len(items) == 0 {
		u.Err().Println("No conditional format rules")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "SHEET\tINDEX\tTYPE\tRANGES")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", item.SheetTitle, item.Index, item.Type, strings.Join(item.Ranges, ","))
	}
	return nil
}

type SheetsConditionalClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" required:"" help:"Sheet name"`
	Index         string `name:"index" help:"Rule index to remove"`
	All           bool   `name:"all" help:"Remove all conditional formatting rules from the sheet"`
}

func (c *SheetsConditionalClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	sheetName := strings.TrimSpace(c.Sheet)
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if sheetName == "" {
		return usage("empty --sheet")
	}
	if !c.All && strings.TrimSpace(c.Index) == "" {
		return usage("provide --index or --all")
	}
	if c.All && strings.TrimSpace(c.Index) != "" {
		return usage("use either --index or --all, not both")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title),conditionalFormats)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	sheetID, count, err := conditionalSheetRuleCount(resp, sheetName)
	if err != nil {
		return err
	}

	requests, err := conditionalDeleteRequests(sheetID, count, strings.TrimSpace(c.Index), c.All)
	if err != nil {
		return err
	}
	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"removed": 0})
		}
		ui.FromContext(ctx).Out().Println("No conditional format rules to remove")
		return nil
	}

	if err := dryRunAndConfirmDestructive(ctx, flags, "sheets.conditional-format.clear", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":          sheetName,
		"index":          strings.TrimSpace(c.Index),
		"all":            c.All,
		"removed":        len(requests),
	}, "remove conditional format rules from "+sheetName); err != nil {
		return err
	}

	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"spreadsheetId": spreadsheetID,
			"sheet":         sheetName,
			"removed":       len(requests),
		})
	}
	ui.FromContext(ctx).Out().Printf("Removed %d conditional format rules from %s", len(requests), sheetName)
	return nil
}

type conditionalRuleItem struct {
	SheetID    int64    `json:"sheetId"`
	SheetTitle string   `json:"sheetTitle"`
	Index      int      `json:"index"`
	Type       string   `json:"type,omitempty"`
	Values     []string `json:"values,omitempty"`
	Ranges     []string `json:"ranges,omitempty"`
	Rule       any      `json:"rule,omitempty"`
}

func parseConditionalFormat(formatJSON, formatMask string) (*sheets.CellFormat, string, error) {
	b, err := resolveInlineOrFileBytes(formatJSON)
	if err != nil {
		return nil, "", fmt.Errorf("read --format-json: %w", err)
	}
	var format sheets.CellFormat
	if err := decodeCellFormatJSON(b, &format); err != nil {
		return nil, "", fmt.Errorf("invalid --format-json: %w", err)
	}
	formatFields := strings.TrimSpace(formatMask)
	if formatFields != "" {
		if hasBoardersTypo(formatFields) {
			return nil, "", fmt.Errorf(`invalid --format-fields: found "boarders"; use "borders"`)
		}
		normalized, formatPaths := normalizeFormatMask(formatFields)
		formatFields = strings.TrimPrefix(normalized, sheetsUserEnteredFormatPrefix+".")
		formatFields = strings.ReplaceAll(formatFields, ","+sheetsUserEnteredFormatPrefix+".", ",")
		if err := applyForceSendFields(&format, formatPaths); err != nil {
			return nil, "", err
		}
	}
	return &format, formatFields, nil
}

func conditionalCondition(kind, expr string) (string, []*sheets.ConditionValue, error) {
	conditionType, valueCount, err := conditionalConditionType(kind)
	if err != nil {
		return "", nil, err
	}
	if valueCount == 0 {
		if expr != "" {
			return "", nil, usagef("--expr is not used with --type %s", kind)
		}
		return conditionType, nil, nil
	}
	if expr == "" {
		return "", nil, usage("--expr is required for this conditional format type")
	}
	return conditionType, []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
}

func conditionalConditionType(kind string) (string, int, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "text-eq":
		return "TEXT_EQ", 1, nil
	case "text-contains":
		return "TEXT_CONTAINS", 1, nil
	case "text-starts-with":
		return "TEXT_STARTS_WITH", 1, nil
	case "text-ends-with":
		return "TEXT_ENDS_WITH", 1, nil
	case "number-eq":
		return "NUMBER_EQ", 1, nil
	case "number-gt":
		return "NUMBER_GREATER", 1, nil
	case "number-gte":
		return "NUMBER_GREATER_THAN_EQ", 1, nil
	case "number-lt":
		return "NUMBER_LESS", 1, nil
	case "number-lte":
		return "NUMBER_LESS_THAN_EQ", 1, nil
	case "blank":
		return "BLANK", 0, nil
	case "not-blank":
		return "NOT_BLANK", 0, nil
	case "custom-formula":
		return "CUSTOM_FORMULA", 1, nil
	default:
		return "", 0, usagef("unsupported --type %q", kind)
	}
}

func conditionalRuleItems(resp *sheets.Spreadsheet, onlySheet string) []conditionalRuleItem {
	items := make([]conditionalRuleItem, 0)
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
		for idx, rule := range sheet.ConditionalFormats {
			item := conditionalRuleItem{
				SheetID:    sheet.Properties.SheetId,
				SheetTitle: sheetTitle,
				Index:      idx,
				Rule:       rule,
			}
			if rule != nil {
				for _, gr := range rule.Ranges {
					item.Ranges = append(item.Ranges, gridRangeToA1(sheetTitle, gr))
				}
				if rule.BooleanRule != nil && rule.BooleanRule.Condition != nil {
					item.Type = rule.BooleanRule.Condition.Type
					for _, value := range rule.BooleanRule.Condition.Values {
						if value != nil {
							item.Values = append(item.Values, value.UserEnteredValue)
						}
					}
				}
			}
			items = append(items, item)
		}
	}
	return items
}

func conditionalSheetRuleCount(resp *sheets.Spreadsheet, sheetName string) (int64, int, error) {
	if resp == nil {
		return 0, 0, fmt.Errorf("empty spreadsheet metadata")
	}
	for _, sheet := range resp.Sheets {
		if sheet == nil || sheet.Properties == nil || sheet.Properties.Title != sheetName {
			continue
		}
		return sheet.Properties.SheetId, len(sheet.ConditionalFormats), nil
	}
	return 0, 0, usagef("unknown sheet %q", sheetName)
}

func conditionalDeleteRequests(sheetID int64, count int, indexRaw string, all bool) ([]*sheets.Request, error) {
	if all {
		requests := make([]*sheets.Request, 0, count)
		for i := count - 1; i >= 0; i-- {
			requests = append(requests, conditionalDeleteRequest(sheetID, int64(i)))
		}
		return requests, nil
	}
	idx, err := strconv.Atoi(indexRaw)
	if err != nil || idx < 0 {
		return nil, usage("invalid --index")
	}
	if idx >= count {
		return nil, usagef("--index %d out of range; sheet has %d rules", idx, count)
	}
	return []*sheets.Request{conditionalDeleteRequest(sheetID, int64(idx))}, nil
}

func conditionalDeleteRequest(sheetID, index int64) *sheets.Request {
	return &sheets.Request{DeleteConditionalFormatRule: &sheets.DeleteConditionalFormatRuleRequest{SheetId: sheetID, Index: index}}
}
