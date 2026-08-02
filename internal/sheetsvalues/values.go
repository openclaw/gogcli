package sheetsvalues

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type ValidationError string

func (e ValidationError) Error() string {
	return string(e)
}

func DecodeStrict(data []byte) ([][]interface{}, error) {
	var values [][]interface{}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	if err := decoder.Decode(&values); err != nil {
		return nil, invalidf("invalid JSON values: %v", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, invalidf("invalid JSON values: trailing content")
	}

	return values, nil
}

func Decode(data []byte) ([][]interface{}, error) {
	var values [][]interface{}
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, invalidf("invalid JSON values: %v", err)
	}

	return values, nil
}

func ParseArgs(values []string) [][]interface{} {
	rawValues := strings.Join(values, " ")
	rows := strings.Split(rawValues, ",")
	parsed := make([][]interface{}, 0, len(rows))

	for _, row := range rows {
		cells := strings.Split(strings.TrimSpace(row), "|")

		rowData := make([]interface{}, len(cells))
		for i, cell := range cells {
			rowData[i] = strings.TrimSpace(cell)
		}

		parsed = append(parsed, rowData)
	}

	return parsed
}

// ParseArgsForShape parses positional update values while ensuring the
// resulting payload cannot extend beyond a requested range. A zero row or
// column count means that dimension is open-ended.
func ParseArgsForShape(values []string, rowCount, columnCount int64) ([][]interface{}, error) {
	if rowCount < 0 || columnCount < 0 {
		return nil, invalidf("invalid update range shape")
	}

	rawValues := strings.TrimSpace(strings.Join(values, " "))
	if rowCount == 1 && columnCount == 1 {
		return [][]interface{}{{rawValues}}, nil
	}

	parsed := ParseArgs(values)
	if rowCount > 0 && int64(len(parsed)) > rowCount {
		return nil, invalidf(
			"positional values have %d rows, which exceeds the update range maximum of %d rows",
			len(parsed),
			rowCount,
		)
	}

	if columnCount > 0 {
		for i, row := range parsed {
			if int64(len(row)) > columnCount {
				return nil, invalidf(
					"positional values row %d has %d cells, which exceeds the update range maximum of %d columns",
					i+1,
					len(row),
					columnCount,
				)
			}
		}
	}

	return parsed, nil
}

func RequireRows(values [][]interface{}) error {
	if len(values) == 0 {
		return invalidf("provide at least one row")
	}

	return nil
}

func DecodeRanges(data []byte) ([]*sheets.ValueRange, error) {
	var ranges []*sheets.ValueRange
	if err := json.Unmarshal(data, &ranges); err != nil {
		return nil, invalidf("invalid JSON data: %v", err)
	}

	if len(ranges) == 0 {
		return nil, invalidf("--data-json must contain at least one value range")
	}

	for i, valueRange := range ranges {
		if valueRange == nil {
			return nil, invalidf("--data-json range %d is null", i)
		}

		valueRange.Range = strings.ReplaceAll(valueRange.Range, `\!`, "!")
		if strings.TrimSpace(valueRange.Range) == "" {
			return nil, invalidf("--data-json range %d has empty range", i)
		}

		if len(valueRange.Values) == 0 {
			return nil, invalidf("--data-json range %d has empty values", i)
		}
	}

	return ranges, nil
}

func invalidf(format string, args ...any) error {
	return ValidationError(fmt.Sprintf(format, args...))
}
