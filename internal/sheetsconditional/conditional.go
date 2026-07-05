package sheetsconditional

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/sheetsa1"
)

var errEmptySpreadsheetMetadata = errors.New("empty spreadsheet metadata")

type ValidationError string

func (e ValidationError) Error() string {
	return string(e)
}

//nolint:tagliatelle // Preserve the existing Sheets CLI JSON contract.
type RuleItem struct {
	SheetID    int64    `json:"sheetId"`
	SheetTitle string   `json:"sheetTitle"`
	Index      int      `json:"index"`
	Type       string   `json:"type,omitempty"`
	Values     []string `json:"values,omitempty"`
	Ranges     []string `json:"ranges,omitempty"`
	Rule       any      `json:"rule,omitempty"`
}

func BuildCondition(kind, expression string) (string, []*sheets.ConditionValue, error) {
	conditionType, valueCount, err := conditionType(kind)
	if err != nil {
		return "", nil, err
	}

	if valueCount == 0 {
		if expression != "" {
			return "", nil, invalidf("--expr is not used with --type %s", kind)
		}

		return conditionType, nil, nil
	}

	if expression == "" {
		return "", nil, invalidf("--expr is required for this conditional format type")
	}

	return conditionType, []*sheets.ConditionValue{{UserEnteredValue: expression}}, nil
}

func BuildAddRequest(
	gridRange *sheets.GridRange,
	conditionType string,
	values []*sheets.ConditionValue,
	format *sheets.CellFormat,
	index int64,
) *sheets.Request {
	return &sheets.Request{
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
			Index: index,
		},
	}
}

func BuildGradientAddRequest(
	gridRange *sheets.GridRange,
	gradientRule *sheets.GradientRule,
	index int64,
) *sheets.Request {
	return &sheets.Request{
		AddConditionalFormatRule: &sheets.AddConditionalFormatRuleRequest{
			Rule: &sheets.ConditionalFormatRule{
				GradientRule: gradientRule,
				Ranges:       []*sheets.GridRange{gridRange},
			},
			Index: index,
		},
	}
}

func RuleItems(spreadsheet *sheets.Spreadsheet, onlySheet string) []RuleItem {
	items := make([]RuleItem, 0)
	if spreadsheet == nil {
		return items
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet == nil || sheet.Properties == nil {
			continue
		}

		sheetTitle := sheet.Properties.Title
		if onlySheet != "" && sheetTitle != onlySheet {
			continue
		}

		for index, rule := range sheet.ConditionalFormats {
			item := RuleItem{
				SheetID:    sheet.Properties.SheetId,
				SheetTitle: sheetTitle,
				Index:      index,
				Rule:       rule,
			}

			if rule != nil {
				for _, gridRange := range rule.Ranges {
					item.Ranges = append(item.Ranges, sheetsa1.FormatGridRange(sheetTitle, gridRange))
				}

				if rule.BooleanRule != nil && rule.BooleanRule.Condition != nil {
					item.Type = rule.BooleanRule.Condition.Type
					for _, value := range rule.BooleanRule.Condition.Values {
						if value != nil {
							item.Values = append(item.Values, value.UserEnteredValue)
						}
					}
				} else if rule.GradientRule != nil {
					item.Type = "GRADIENT_RULE"
				}
			}

			items = append(items, item)
		}
	}

	return items
}

func SheetRuleCount(spreadsheet *sheets.Spreadsheet, sheetName string) (int64, int, error) {
	if spreadsheet == nil {
		return 0, 0, errEmptySpreadsheetMetadata
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet == nil || sheet.Properties == nil || sheet.Properties.Title != sheetName {
			continue
		}

		return sheet.Properties.SheetId, len(sheet.ConditionalFormats), nil
	}

	return 0, 0, invalidf("unknown sheet %q", sheetName)
}

func BuildDeleteRequests(sheetID int64, count int, indexRaw string, all bool) ([]*sheets.Request, error) {
	if all {
		requests := make([]*sheets.Request, 0, count)
		for index := count - 1; index >= 0; index-- {
			requests = append(requests, deleteRequest(sheetID, int64(index)))
		}

		return requests, nil
	}

	index, err := strconv.Atoi(indexRaw)
	if err != nil || index < 0 {
		return nil, invalidf("invalid --index")
	}

	if index >= count {
		return nil, invalidf("--index %d out of range; sheet has %d rules", index, count)
	}

	return []*sheets.Request{deleteRequest(sheetID, int64(index))}, nil
}

func ValidateClearIndex(indexRaw string) error {
	indexRaw = strings.TrimSpace(indexRaw)
	if indexRaw == "" {
		return nil
	}

	index, err := strconv.Atoi(indexRaw)
	if err != nil || index < 0 {
		return invalidf("--index must be a non-negative integer")
	}

	return nil
}

func conditionType(kind string) (string, int, error) {
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
		return "", 0, invalidf("unsupported --type %q", kind)
	}
}

func deleteRequest(sheetID, index int64) *sheets.Request {
	return &sheets.Request{
		DeleteConditionalFormatRule: &sheets.DeleteConditionalFormatRuleRequest{
			SheetId: sheetID,
			Index:   index,
		},
	}
}

func invalidf(format string, args ...any) error {
	return ValidationError(fmt.Sprintf(format, args...))
}
