package sheetsvalues

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeStrictPreservesNumbersAndRejectsTrailingContent(t *testing.T) {
	values, err := DecodeStrict([]byte(`[[9007199254740993]]`))
	if err != nil {
		t.Fatalf("DecodeStrict() error = %v", err)
	}

	number, ok := values[0][0].(json.Number)
	if !ok || number.String() != "9007199254740993" {
		t.Fatalf("number = %#v", values[0][0])
	}

	if _, err := DecodeStrict([]byte(`[["a"]] trailing`)); err == nil ||
		!strings.Contains(err.Error(), "trailing content") {
		t.Fatalf("trailing error = %v", err)
	}
}

func TestDecodeUsesNativeJSONNumbers(t *testing.T) {
	values, err := Decode([]byte(`[[2]]`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if value, ok := values[0][0].(float64); !ok || value != 2 {
		t.Fatalf("value = %#v", values[0][0])
	}
}

func TestParseArgs(t *testing.T) {
	values := ParseArgs([]string{"a | b,", "c|d"})
	if len(values) != 2 ||
		len(values[0]) != 2 ||
		values[0][0] != "a" ||
		values[0][1] != "b" ||
		values[1][0] != "c" ||
		values[1][1] != "d" {
		t.Fatalf("values = %#v", values)
	}
}

func TestParseArgsForShapeSingleCellPreservesDelimiters(t *testing.T) {
	values, err := ParseArgsForShape([]string{"text, with, commas | and pipes"}, 1, 1)
	if err != nil {
		t.Fatalf("ParseArgsForShape() error = %v", err)
	}

	if len(values) != 1 || len(values[0]) != 1 ||
		values[0][0] != "text, with, commas | and pipes" {
		t.Fatalf("values = %#v", values)
	}
}

func TestParseArgsForShapeMatchesMultiCellRange(t *testing.T) {
	values, err := ParseArgsForShape([]string{"a|b,c|d"}, 2, 2)
	if err != nil {
		t.Fatalf("ParseArgsForShape() error = %v", err)
	}

	if len(values) != 2 || len(values[0]) != 2 || len(values[1]) != 2 ||
		values[0][0] != "a" || values[0][1] != "b" ||
		values[1][0] != "c" || values[1][1] != "d" {
		t.Fatalf("values = %#v", values)
	}
}

func TestParseArgsForShapeAllowsSmallerMatrix(t *testing.T) {
	values, err := ParseArgsForShape([]string{"a"}, 2, 2)
	if err != nil {
		t.Fatalf("ParseArgsForShape() error = %v", err)
	}

	if len(values) != 1 || len(values[0]) != 1 || values[0][0] != "a" {
		t.Fatalf("values = %#v, want one-cell matrix", values)
	}
}

func TestParseArgsForShapeRejectsExceedingRangeBounds(t *testing.T) {
	for _, tc := range []struct {
		name       string
		values     []string
		rows, cols int64
		want       string
	}{
		{name: "rows", values: []string{"a,b,c"}, rows: 2, cols: 1, want: "3 rows, which exceeds"},
		{name: "columns", values: []string{"a|b|c"}, rows: 1, cols: 2, want: "3 cells, which exceeds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseArgsForShape(tc.values, tc.rows, tc.cols)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseArgsForShape() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRequireRows(t *testing.T) {
	if err := RequireRows(nil); err == nil || !strings.Contains(err.Error(), "at least one row") {
		t.Fatalf("RequireRows() error = %v", err)
	}
}

func TestDecodeRanges(t *testing.T) {
	ranges, err := DecodeRanges([]byte(`[{"range":"Sheet1\\!A1","values":[["a"]]}]`))
	if err != nil {
		t.Fatalf("DecodeRanges() error = %v", err)
	}

	if len(ranges) != 1 || ranges[0].Range != "Sheet1!A1" {
		t.Fatalf("ranges = %#v", ranges)
	}

	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{name: "invalid json", data: `nope`, want: "invalid JSON data"},
		{name: "empty array", data: `[]`, want: "at least one value range"},
		{name: "null range", data: `[null]`, want: "range 0 is null"},
		{name: "empty range", data: `[{"range":"","values":[["a"]]}]`, want: "empty range"},
		{name: "missing values", data: `[{"range":"Sheet1!A1"}]`, want: "empty values"},
		{name: "empty values", data: `[{"range":"Sheet1!A1","values":[]}]`, want: "empty values"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := DecodeRanges([]byte(tc.data))
			if gotErr == nil || !strings.Contains(gotErr.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", gotErr, tc.want)
			}
		})
	}
}
