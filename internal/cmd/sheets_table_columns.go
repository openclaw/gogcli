package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type sheetsTableColumnInput struct {
	ColumnName         string                                `json:"columnName"`
	ColumnType         string                                `json:"columnType"`
	DataValidation     *sheets.TableColumnDataValidationRule `json:"dataValidation,omitempty"`
	DataValidationRule *sheets.TableColumnDataValidationRule `json:"dataValidationRule,omitempty"`
}

func parseSheetsTableColumnsJSON(input string) ([]*sheets.TableColumnProperties, error) {
	b, err := resolveInlineOrFileBytes(input)
	if err != nil {
		return nil, fmt.Errorf("read --columns-json: %w", err)
	}

	var raw []sheetsTableColumnInput
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("invalid columns JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("invalid columns JSON: multiple JSON values")
		}
		return nil, fmt.Errorf("invalid columns JSON: %w", err)
	}
	if len(raw) == 0 {
		return nil, usage("provide at least one table column")
	}

	cols := make([]*sheets.TableColumnProperties, 0, len(raw))
	for i, in := range raw {
		name := strings.TrimSpace(in.ColumnName)
		if name == "" {
			return nil, usagef("columns[%d].columnName is required", i)
		}
		colType, err := normalizeSheetsTableColumnType(in.ColumnType)
		if err != nil {
			return nil, usagef("columns[%d].columnType: %v", i, err)
		}
		validation := in.DataValidationRule
		if validation == nil {
			validation = in.DataValidation
		}
		if validation != nil && colType != "DROPDOWN" {
			return nil, usagef("columns[%d].dataValidationRule requires columnType DROPDOWN", i)
		}
		col := &sheets.TableColumnProperties{
			ColumnIndex:        int64(i),
			ColumnName:         name,
			ColumnType:         colType,
			DataValidationRule: validation,
		}
		col.ForceSendFields = []string{"ColumnIndex"}
		cols = append(cols, col)
	}
	return cols, nil
}

func normalizeSheetsTableColumnType(input string) (string, error) {
	t := strings.ToUpper(strings.TrimSpace(input))
	if t == "" {
		return "TEXT", nil
	}
	switch t {
	case "NUMBER":
		return "", fmt.Errorf("NUMBER is not a Sheets table column type; use DOUBLE")
	case "CHECKBOX":
		return "", fmt.Errorf("CHECKBOX is not a Sheets table column type; use BOOLEAN")
	case "RATING":
		return "", fmt.Errorf("RATING is not a Sheets table column type; use RATINGS_CHIP")
	case "SMART_CHIP":
		return "", fmt.Errorf("SMART_CHIP is not a Sheets table column type; use FILES_CHIP, PEOPLE_CHIP, FINANCE_CHIP, or PLACE_CHIP")
	case "COLUMN_TYPE_UNSPECIFIED":
		return "", fmt.Errorf("COLUMN_TYPE_UNSPECIFIED is not valid for create")
	}
	if validSheetsTableColumnTypes[t] {
		return t, nil
	}
	return "", fmt.Errorf("unknown type %q; valid types: %s", t, strings.Join(validSheetsTableColumnTypeNames(), ", "))
}

var validSheetsTableColumnTypes = map[string]bool{
	"TEXT":         true,
	"DOUBLE":       true,
	"CURRENCY":     true,
	"PERCENT":      true,
	"DATE":         true,
	"TIME":         true,
	"DATE_TIME":    true,
	"BOOLEAN":      true,
	"DROPDOWN":     true,
	"FILES_CHIP":   true,
	"PEOPLE_CHIP":  true,
	"FINANCE_CHIP": true,
	"PLACE_CHIP":   true,
	"RATINGS_CHIP": true,
}

func validSheetsTableColumnTypeNames() []string {
	names := make([]string, 0, len(validSheetsTableColumnTypes))
	for name := range validSheetsTableColumnTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
