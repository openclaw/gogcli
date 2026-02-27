package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsConditionalFormatCmd struct {
	Add   SheetsConditionalFormatAddCmd   `cmd:"" name:"add" help:"Add a conditional formatting rule"`
	List  SheetsConditionalFormatListCmd  `cmd:"" name:"list" help:"List conditional formatting rules"`
	Clear SheetsConditionalFormatClearCmd `cmd:"" name:"clear" help:"Remove conditional formatting rules"`
}

type SheetsConditionalFormatAddCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `name:"range" help:"A1 range with sheet name (eg. Sheet1!A2:J)" required:""`
	Type          string `name:"type" help:"Rule type: text-eq|number-gt|number-gte|number-lt|number-lte|custom-formula" required:""`
	Expr          string `name:"expr" help:"Expression value or custom formula" required:""`
	StyleJSON     string `name:"style-json" help:"CellFormat JSON" required:""`
	StopIfTrue    bool   `name:"stop-if-true" help:"Stop evaluating lower-priority rules if this matches"`
}

func (c *SheetsConditionalFormatAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	rangeInfo, err := parseSheetRange(cleanRange(c.Range), "range")
	if err != nil {
		return err
	}

	var format sheets.CellFormat
	b, err := resolveInlineOrFileBytes(c.StyleJSON)
	if err != nil {
		return fmt.Errorf("read --style-json: %w", err)
	}
	if err := json.Unmarshal(b, &format); err != nil {
		return fmt.Errorf("invalid --style-json: %w", err)
	}

	conditionType, values, err := conditionalTypeAndValues(strings.TrimSpace(c.Type), strings.TrimSpace(c.Expr))
	if err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "sheets.conditional_format.add", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          c.Range,
		"type":           conditionType,
		"values":         values,
		"stop_if_true":   c.StopIfTrue,
		"format":         format,
	}); err != nil {
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
	sheetIDs, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := gridRangeFromMap(rangeInfo, sheetIDs, "range")
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
					Format:     &format,
					ForceSendFields: []string{"Format"},
				},
				Ranges: []*sheets.GridRange{gridRange},
			},
			Index: 0,
		},
	}}}

	if _, err := svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do(); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"ok": true})
	}
	ui.FromContext(ctx).Out().Printf("Added conditional format rule to %s", c.Range)
	return nil
}

type SheetsConditionalFormatListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

func (c *SheetsConditionalFormatListCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(title,sheetId),conditionalFormats)").Do()
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0)
	for _, sh := range resp.Sheets {
		for idx, rule := range sh.ConditionalFormats {
			entry := map[string]any{"sheet": sh.Properties.Title, "sheetId": sh.Properties.SheetId, "index": idx}
			if rule != nil && rule.BooleanRule != nil && rule.BooleanRule.Condition != nil {
				entry["type"] = rule.BooleanRule.Condition.Type
				entry["values"] = rule.BooleanRule.Condition.Values
			}
			rows = append(rows, entry)
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"rules": rows})
	}
	u := ui.FromContext(ctx)
	for _, r := range rows {
		u.Out().Printf("%s[#%v] %v", r["sheet"], r["index"], r["type"])
	}
	if len(rows) == 0 {
		u.Out().Println("No conditional format rules")
	}
	return nil
}

type SheetsConditionalFormatClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Sheet name" required:""`
	Index         string `name:"index" help:"Rule index to remove"`
	All           bool   `name:"all" help:"Remove all rules from the sheet"`
}

func (c *SheetsConditionalFormatClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if !c.All && strings.TrimSpace(c.Index) == "" {
		return fmt.Errorf("provide --index or --all")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	ids, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	sheetID, ok := ids[c.Sheet]
	if !ok {
		return fmt.Errorf("unknown sheet %q", c.Sheet)
	}

	requests := []*sheets.Request{}
	if c.All {
		meta, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(sheetId),conditionalFormats)").Do()
		if err != nil {
			return err
		}
		count := 0
		for _, sh := range meta.Sheets {
			if sh.Properties != nil && sh.Properties.SheetId == sheetID {
				count = len(sh.ConditionalFormats)
				break
			}
		}
		for i := count - 1; i >= 0; i-- {
			requests = append(requests, &sheets.Request{DeleteConditionalFormatRule: &sheets.DeleteConditionalFormatRuleRequest{SheetId: sheetID, Index: int64(i)}})
		}
	} else {
		idx, err := strconv.Atoi(strings.TrimSpace(c.Index))
		if err != nil || idx < 0 {
			return fmt.Errorf("invalid --index")
		}
		requests = append(requests, &sheets.Request{DeleteConditionalFormatRule: &sheets.DeleteConditionalFormatRuleRequest{SheetId: sheetID, Index: int64(idx)}})
	}
	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"removed": 0})
		}
		ui.FromContext(ctx).Out().Println("No rules to remove")
		return nil
	}
	_, err = svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"removed": len(requests)})
	}
	ui.FromContext(ctx).Out().Printf("Removed %d conditional rules", len(requests))
	return nil
}

type SheetsBandingCmd struct {
	Set   SheetsBandingSetCmd   `cmd:"" name:"set" help:"Apply alternating row colors"`
	Clear SheetsBandingClearCmd `cmd:"" name:"clear" help:"Clear banding"`
}

type SheetsBandingSetCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range         string `name:"range" help:"A1 range with sheet name" required:""`
}

func (c *SheetsBandingSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	rangeInfo, err := parseSheetRange(cleanRange(c.Range), "range")
	if err != nil {
		return err
	}
	if err := dryRunExit(ctx, flags, "sheets.banding.set", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          c.Range,
	}); err != nil {
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
	ids, err := fetchSheetIDMap(ctx, svc, spreadsheetID)
	if err != nil {
		return err
	}
	gridRange, err := gridRangeFromMap(rangeInfo, ids, "range")
	if err != nil {
		return err
	}
	req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{{
		AddBanding: &sheets.AddBandingRequest{BandedRange: &sheets.BandedRange{
			Range: gridRange,
			RowProperties: &sheets.BandingProperties{
				FirstBandColor:  &sheets.Color{Red: 0.98, Green: 0.98, Blue: 0.99},
				SecondBandColor: &sheets.Color{Red: 0.95, Green: 0.96, Blue: 0.98},
			},
		}},
	}}}
	_, err = svc.Spreadsheets.BatchUpdate(spreadsheetID, req).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"ok": true})
	}
	ui.FromContext(ctx).Out().Printf("Applied banding to %s", c.Range)
	return nil
}

type SheetsBandingClearCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Sheet name" required:""`
	All           bool   `name:"all" help:"Remove all banding on sheet"`
}

func (c *SheetsBandingClearCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	if err := dryRunExit(ctx, flags, "sheets.banding.clear", map[string]any{
		"spreadsheet_id": spreadsheetID,
		"sheet":         c.Sheet,
		"all":           c.All,
	}); err != nil {
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
	meta, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(title),bandedRanges(bandedRangeId))").Do()
	if err != nil {
		return err
	}
	requests := []*sheets.Request{}
	for _, sh := range meta.Sheets {
		if sh.Properties != nil && sh.Properties.Title == c.Sheet {
			for _, br := range sh.BandedRanges {
				requests = append(requests, &sheets.Request{DeleteBanding: &sheets.DeleteBandingRequest{BandedRangeId: br.BandedRangeId}})
			}
		}
	}
	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"removed": 0})
		}
		ui.FromContext(ctx).Out().Println("No banding found")
		return nil
	}
	_, err = svc.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"removed": len(requests)})
	}
	ui.FromContext(ctx).Out().Printf("Removed %d banded ranges", len(requests))
	return nil
}

type SheetsChartCmd struct {
	List SheetsChartListCmd `cmd:"" name:"list" help:"List embedded charts"`
}

type SheetsChartListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
}

func (c *SheetsChartListCmd) Run(ctx context.Context, flags *RootFlags) error {
	spreadsheetID := normalizeGoogleID(strings.TrimSpace(c.SpreadsheetID))
	if spreadsheetID == "" {
		return usage("empty spreadsheetId")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := newSheetsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spreadsheets.Get(spreadsheetID).Fields("sheets(properties(title),charts(chartId,spec(title)))").Do()
	if err != nil {
		return err
	}
	charts := make([]map[string]any, 0)
	for _, sh := range resp.Sheets {
		for _, ch := range sh.Charts {
			title := ""
			if ch.Spec != nil {
				title = ch.Spec.Title
			}
			charts = append(charts, map[string]any{"sheet": sh.Properties.Title, "chartId": ch.ChartId, "title": title})
		}
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"charts": charts})
	}
	u := ui.FromContext(ctx)
	for _, ch := range charts {
		u.Out().Printf("%s	%v	%s", ch["sheet"], ch["chartId"], ch["title"])
	}
	if len(charts) == 0 {
		u.Out().Println("No charts")
	}
	return nil
}

func conditionalTypeAndValues(kind, expr string) (string, []*sheets.ConditionValue, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "text-eq":
		return "TEXT_EQ", []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
	case "number-gt":
		return "NUMBER_GREATER", []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
	case "number-gte":
		return "NUMBER_GREATER_THAN_EQ", []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
	case "number-lt":
		return "NUMBER_LESS", []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
	case "number-lte":
		return "NUMBER_LESS_THAN_EQ", []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
	case "custom-formula":
		return "CUSTOM_FORMULA", []*sheets.ConditionValue{{UserEnteredValue: expr}}, nil
	default:
		return "", nil, fmt.Errorf("unsupported --type %q", kind)
	}
}
