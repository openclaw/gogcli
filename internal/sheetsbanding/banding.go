package sheetsbanding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"google.golang.org/api/sheets/v4"

	"github.com/openclaw/gogcli/internal/sheetsa1"
)

var errMultipleJSONValues = errors.New("multiple JSON values")

//nolint:tagliatelle // Preserve the existing Sheets CLI JSON contract.
type Item struct {
	BandedRangeID    int64                     `json:"bandedRangeId"`
	SheetID          int64                     `json:"sheetId"`
	SheetTitle       string                    `json:"sheetTitle"`
	A1               string                    `json:"a1,omitempty"`
	Range            *sheets.GridRange         `json:"range,omitempty"`
	RowProperties    *sheets.BandingProperties `json:"rowProperties,omitempty"`
	ColumnProperties *sheets.BandingProperties `json:"columnProperties,omitempty"`
}

func DecodeProperties(data []byte) (*sheets.BandingProperties, error) {
	var properties sheets.BandingProperties

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&properties); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errMultipleJSONValues
		}

		return nil, fmt.Errorf("%w", err)
	}

	return &properties, nil
}

func DefaultRowProperties() *sheets.BandingProperties {
	return &sheets.BandingProperties{
		HeaderColorStyle:     &sheets.ColorStyle{RgbColor: &sheets.Color{Red: 0.88, Green: 0.93, Blue: 1}},
		FirstBandColorStyle:  &sheets.ColorStyle{RgbColor: &sheets.Color{Red: 1, Green: 1, Blue: 1}},
		SecondBandColorStyle: &sheets.ColorStyle{RgbColor: &sheets.Color{Red: 0.96, Green: 0.98, Blue: 1}},
	}
}

func BuildAddRequest(
	gridRange *sheets.GridRange,
	rowProperties, columnProperties *sheets.BandingProperties,
) *sheets.Request {
	return &sheets.Request{
		AddBanding: &sheets.AddBandingRequest{
			BandedRange: &sheets.BandedRange{
				Range:            gridRange,
				RowProperties:    rowProperties,
				ColumnProperties: columnProperties,
			},
		},
	}
}

func Items(spreadsheet *sheets.Spreadsheet, onlySheet string) []Item {
	items := make([]Item, 0)
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

		for _, bandedRange := range sheet.BandedRanges {
			if bandedRange == nil {
				continue
			}

			items = append(items, Item{
				BandedRangeID:    bandedRange.BandedRangeId,
				SheetID:          sheet.Properties.SheetId,
				SheetTitle:       sheetTitle,
				A1:               sheetsa1.FormatGridRange(sheetTitle, bandedRange.Range),
				Range:            bandedRange.Range,
				RowProperties:    bandedRange.RowProperties,
				ColumnProperties: bandedRange.ColumnProperties,
			})
		}
	}

	return items
}

func IDsForSheet(spreadsheet *sheets.Spreadsheet, sheetName string) ([]int64, bool) {
	if spreadsheet == nil {
		return nil, false
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet == nil || sheet.Properties == nil || sheet.Properties.Title != sheetName {
			continue
		}

		ids := make([]int64, 0, len(sheet.BandedRanges))
		for _, bandedRange := range sheet.BandedRanges {
			if bandedRange != nil {
				ids = append(ids, bandedRange.BandedRangeId)
			}
		}

		return ids, true
	}

	return nil, false
}

func DeleteRequest(id int64) *sheets.Request {
	return &sheets.Request{
		DeleteBanding: &sheets.DeleteBandingRequest{
			BandedRangeId: id,
		},
	}
}
