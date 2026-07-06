package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/sheetsconditional"
	"github.com/steipete/gogcli/internal/sheetsformat"
	"github.com/steipete/gogcli/internal/ui"
)

type SheetsConditionalCmd struct {
	List  SheetsConditionalListCmd  `cmd:"" default:"withargs" help:"List conditional formatting rules"`
	Add   SheetsConditionalAddCmd   `cmd:"" name:"add" aliases:"create,new" help:"Add a conditional formatting rule"`
	Clear SheetsConditionalClearCmd `cmd:"" name:"clear" aliases:"delete,rm,remove" help:"Remove conditional formatting rules"`
}

type SheetsConditionalAddCmd struct {
	SpreadsheetID    string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Range            string `arg:"" name:"range" help:"A1 range with sheet name (e.g. Sheet1!A2:J)"`
	Type             string `name:"type" help:"Boolean rule type: text-eq|text-contains|text-starts-with|text-ends-with|number-eq|number-gt|number-gte|number-lt|number-lte|blank|not-blank|custom-formula"`
	Expr             string `name:"expr" help:"Expression value or custom formula for boolean rules (omit for blank/not-blank)"`
	FormatJSON       string `name:"format-json" help:"CellFormat JSON for boolean rules (inline or @file)"`
	FormatFields     string `name:"format-fields" help:"Format field mask for force-sending zero/false fields in boolean rule formats (e.g. backgroundColor,textFormat.bold)"`
	GradientRuleJSON string `name:"gradient-rule-json" help:"GradientRule JSON for gradient conditional formats (inline or @file)"`
	Index            int64  `name:"index" help:"Insert rule at this priority index" default:"0"`
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

	input := stdinReader(ctx)
	useGradient := strings.TrimSpace(c.GradientRuleJSON) != ""
	var (
		conditionType string
		values        []*sheets.ConditionValue
		format        *sheets.CellFormat
		formatFields  string
		gradientRule  *sheets.GradientRule
	)

	if useGradient {
		if strings.TrimSpace(c.Type) != "" ||
			strings.TrimSpace(c.Expr) != "" ||
			strings.TrimSpace(c.FormatJSON) != "" ||
			strings.TrimSpace(c.FormatFields) != "" {
			return usage("use either --gradient-rule-json or boolean rule flags (--type, --expr, --format-json, --format-fields), not both")
		}
		gradientRule, err = parseConditionalGradientRule(c.GradientRuleJSON, input)
		if err != nil {
			return err
		}
	} else {
		if strings.TrimSpace(c.Type) == "" {
			return usage("provide --type or --gradient-rule-json")
		}
		if strings.TrimSpace(c.FormatJSON) == "" {
			return usage("provide --format-json for boolean conditional format rules")
		}
		format, formatFields, err = parseConditionalFormat(c.FormatJSON, c.FormatFields, input)
		if err != nil {
			return err
		}
		conditionType, values, err = sheetsconditional.BuildCondition(strings.TrimSpace(c.Type), strings.TrimSpace(c.Expr))
		if err != nil {
			return sheetsConditionalPlannerError(err)
		}
	}

	dryRunRequest := map[string]any{
		"spreadsheet_id": spreadsheetID,
		"range":          rangeSpec,
		"index":          c.Index,
	}
	if useGradient {
		dryRunRequest["type"] = "GRADIENT_RULE"
		dryRunRequest["gradient_rule"] = gradientRule
	} else {
		dryRunRequest["type"] = conditionType
		dryRunRequest["values"] = values
		dryRunRequest["format_fields"] = formatFields
	}

	if dryErr := dryRunExit(ctx, flags, "sheets.conditional-format.add", dryRunRequest); dryErr != nil {
		return dryErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
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

	req := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{nil},
	}
	if useGradient {
		req.Requests[0] = sheetsconditional.BuildGradientAddRequest(gridRange, gradientRule, c.Index)
	} else {
		req.Requests[0] = sheetsconditional.BuildAddRequest(gridRange, conditionType, values, format, c.Index)
	}

	if err := applySheetsBatchUpdate(ctx, svc, spreadsheetID, req); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"range":         rangeSpec,
			"type":          conditionalFormatRuleType(useGradient, conditionType),
			"index":         c.Index,
		})
	}
	u.Out().Linef("Added conditional format rule to %s", rangeSpec)
	return nil
}

type SheetsConditionalListCmd struct {
	SpreadsheetID string `arg:"" name:"spreadsheetId" help:"Spreadsheet ID"`
	Sheet         string `name:"sheet" help:"Only list rules from this sheet"`
}

func (c *SheetsConditionalListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runSheetsSpreadsheetList(
		ctx,
		flags,
		c.SpreadsheetID,
		c.Sheet,
		"sheets(properties(sheetId,title),conditionalFormats)",
		"rules",
		"No conditional format rules",
		sheetsconditional.RuleItems,
		sheetsConditionalColumns(),
	)
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
	if err := sheetsconditional.ValidateClearIndex(strings.TrimSpace(c.Index)); err != nil {
		return sheetsConditionalPlannerError(err)
	}

	if flags != nil && flags.DryRun {
		return dryRunAndConfirmDestructive(ctx, flags, "sheets.conditional-format.clear", map[string]any{
			"spreadsheet_id": spreadsheetID,
			"sheet":          sheetName,
			"index":          strings.TrimSpace(c.Index),
			"all":            c.All,
		}, "remove conditional format rules from "+sheetName)
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := sheetsService(ctx, account)
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
	sheetID, count, err := sheetsconditional.SheetRuleCount(resp, sheetName)
	if err != nil {
		return sheetsConditionalPlannerError(err)
	}

	requests, err := sheetsconditional.BuildDeleteRequests(sheetID, count, strings.TrimSpace(c.Index), c.All)
	if err != nil {
		return sheetsConditionalPlannerError(err)
	}
	if len(requests) == 0 {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"removed": 0})
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
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"spreadsheetId": spreadsheetID,
			"sheet":         sheetName,
			"removed":       len(requests),
		})
	}
	ui.FromContext(ctx).Out().Linef("Removed %d conditional format rules from %s", len(requests), sheetName)
	return nil
}

func parseConditionalFormat(formatJSON, formatMask string, input io.Reader) (*sheets.CellFormat, string, error) {
	b, err := resolveInlineOrFileBytes(formatJSON, input)
	if err != nil {
		return nil, "", usagef("read --format-json: %v", err)
	}
	var format sheets.CellFormat
	if err := sheetsformat.Decode(b, &format); err != nil {
		return nil, "", usagef("invalid --format-json: %v", err)
	}
	formatFields := strings.TrimSpace(formatMask)
	if formatFields != "" {
		if sheetsformat.HasBordersTypo(formatFields) {
			return nil, "", usage(`invalid --format-fields: found "boarders"; use "borders"`)
		}
		normalized, formatPaths := sheetsformat.NormalizeMask(formatFields)
		formatFields = strings.TrimPrefix(normalized, sheetsformat.UserEnteredFormatPrefix+".")
		formatFields = strings.ReplaceAll(formatFields, ","+sheetsformat.UserEnteredFormatPrefix+".", ",")
		if err := sheetsformat.ApplyForceSendFields(&format, formatPaths); err != nil {
			return nil, "", usage(err.Error())
		}
	}
	return &format, formatFields, nil
}

func parseConditionalGradientRule(raw string, input io.Reader) (*sheets.GradientRule, error) {
	b, err := resolveInlineOrFileBytes(raw, input)
	if err != nil {
		return nil, usagef("read --gradient-rule-json: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, usage("empty --gradient-rule-json")
	}

	rule, err := decodeConditionalGradientRule(b)
	if err != nil {
		return nil, usagef("invalid --gradient-rule-json: %v", err)
	}
	if rule.Minpoint == nil || rule.Maxpoint == nil {
		return nil, usage("--gradient-rule-json must include minpoint and maxpoint")
	}

	return rule, nil
}

type conditionalGradientRuleJSON struct {
	Minpoint *conditionalGradientPointJSON `json:"minpoint,omitempty"`
	Midpoint *conditionalGradientPointJSON `json:"midpoint,omitempty"`
	Maxpoint *conditionalGradientPointJSON `json:"maxpoint,omitempty"`
}

type conditionalGradientPointJSON struct {
	Color      *conditionalGradientColorJSON      `json:"color,omitempty"`
	ColorStyle *conditionalGradientColorStyleJSON `json:"colorStyle,omitempty"`
	Type       string                             `json:"type,omitempty"`
	Value      string                             `json:"value,omitempty"`
}

type conditionalGradientColorStyleJSON struct {
	RGBColor   *conditionalGradientColorJSON `json:"rgbColor,omitempty"`
	ThemeColor string                        `json:"themeColor,omitempty"`
}

type conditionalGradientColorJSON struct {
	Alpha *float64 `json:"alpha,omitempty"`
	Blue  *float64 `json:"blue,omitempty"`
	Green *float64 `json:"green,omitempty"`
	Red   *float64 `json:"red,omitempty"`
}

func decodeConditionalGradientRule(data []byte) (*sheets.GradientRule, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var wire conditionalGradientRuleJSON
	if err := dec.Decode(&wire); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	if conditionalGradientHasAlpha(&wire) {
		return nil, errors.New("sheets gradient colors do not support alpha")
	}

	var rule sheets.GradientRule
	if err := json.Unmarshal(data, &rule); err != nil {
		return nil, err
	}
	preserveConditionalGradientColorFields(wire.Minpoint, rule.Minpoint)
	preserveConditionalGradientColorFields(wire.Midpoint, rule.Midpoint)
	preserveConditionalGradientColorFields(wire.Maxpoint, rule.Maxpoint)
	return &rule, nil
}

func preserveConditionalGradientColorFields(wire *conditionalGradientPointJSON, point *sheets.InterpolationPoint) {
	if wire == nil || point == nil {
		return
	}
	preserveConditionalGradientRGBFields(wire.Color, point.Color)
	if wire.ColorStyle != nil && point.ColorStyle != nil {
		preserveConditionalGradientRGBFields(wire.ColorStyle.RGBColor, point.ColorStyle.RgbColor)
	}
}

func preserveConditionalGradientRGBFields(wire *conditionalGradientColorJSON, color *sheets.Color) {
	if wire == nil || color == nil {
		return
	}
	if wire.Blue != nil {
		color.ForceSendFields = append(color.ForceSendFields, "Blue")
	}
	if wire.Green != nil {
		color.ForceSendFields = append(color.ForceSendFields, "Green")
	}
	if wire.Red != nil {
		color.ForceSendFields = append(color.ForceSendFields, "Red")
	}
}

func conditionalGradientHasAlpha(rule *conditionalGradientRuleJSON) bool {
	if rule == nil {
		return false
	}
	return conditionalGradientPointHasAlpha(rule.Minpoint) ||
		conditionalGradientPointHasAlpha(rule.Midpoint) ||
		conditionalGradientPointHasAlpha(rule.Maxpoint)
}

func conditionalGradientPointHasAlpha(point *conditionalGradientPointJSON) bool {
	if point == nil {
		return false
	}
	if point.Color != nil && point.Color.Alpha != nil {
		return true
	}
	return point.ColorStyle != nil && point.ColorStyle.RGBColor != nil && point.ColorStyle.RGBColor.Alpha != nil
}

func conditionalFormatRuleType(useGradient bool, conditionType string) string {
	if useGradient {
		return "GRADIENT_RULE"
	}
	return conditionType
}

func sheetsConditionalPlannerError(err error) error {
	var validationErr sheetsconditional.ValidationError
	if errors.As(err, &validationErr) {
		return usage(validationErr.Error())
	}

	return err
}
