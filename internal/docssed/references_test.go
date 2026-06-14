//nolint:wsl_v5 // Table-driven parser tests stay compact around assertions.
package docssed

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseTableReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  *TableReference
	}{
		{input: "|1|", want: &TableReference{TableIndex: 1}},
		{input: "|-2|", want: &TableReference{TableIndex: -2}},
		{input: "|*|", want: &TableReference{}},
		{input: "|0|"},
		{input: "|abc|"},
		{input: "|3x4|"},
		{input: "|1|[A1]"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got := ParseTableReference(test.input)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ParseTableReference(%q) = %+v, want %+v", test.input, got, test.want)
			}
		})
	}
}

func TestParseTableCellReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  *CellReference
	}{
		{input: "|1|[2,3]", want: &CellReference{TableIndex: 1, Row: 2, Column: 3}},
		{input: "|1|[A1]", want: &CellReference{TableIndex: 1, Row: 1, Column: 1}},
		{input: "|-1|[1,1]", want: &CellReference{TableIndex: -1, Row: 1, Column: 1}},
		{input: "|1|[2,4]:old", want: &CellReference{TableIndex: 1, Row: 2, Column: 4, Subpattern: "old"}},
		{input: "|1|[*,2]", want: &CellReference{TableIndex: 1, Column: 2}},
		{input: "|1|[2,*]", want: &CellReference{TableIndex: 1, Row: 2}},
		{input: "|1|[+2,3]", want: &CellReference{
			TableIndex: 1, Column: 3, RowOperation: "append", OperationTarget: 2,
		}},
		{input: "|1|[2,+3]", want: &CellReference{
			TableIndex: 1, Row: 2, ColumnOperation: "append", OperationTarget: 3,
		}},
		{input: "|1|[row:+2]", want: &CellReference{
			TableIndex: 1, RowOperation: "insert", OperationTarget: 2,
		}},
		{input: "|1|[row:$+]", want: &CellReference{TableIndex: 1, RowOperation: "append"}},
		{input: "|1|[col:-1]", want: &CellReference{
			TableIndex: 1, ColumnOperation: "delete", OperationTarget: -1,
		}},
		{input: "|1|[1,1:2,3]", want: &CellReference{
			TableIndex: 1, Row: 1, Column: 1, EndRow: 2, EndColumn: 3,
		}},
		{input: "hello"},
		{input: "|1|foo"},
		{input: "|x|[1,1]"},
		{input: "|1|[$+,1]"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got := ParseTableCellReference(test.input)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ParseTableCellReference(%q) = %+v, want %+v", test.input, got, test.want)
			}
		})
	}
}

func TestParseExcelReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input       string
		row, column int
		ok          bool
	}{
		{input: "A1", row: 1, column: 1, ok: true},
		{input: "B2", row: 2, column: 2, ok: true},
		{input: "Z1", row: 1, column: 26, ok: true},
		{input: "AA10", row: 10, column: 27, ok: true},
		{input: "A0"},
		{input: "1A"},
		{input: "A"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			row, column, ok := ParseExcelReference(test.input)
			if row != test.row || column != test.column || ok != test.ok {
				t.Fatalf("ParseExcelReference(%q) = (%d, %d, %v)", test.input, row, column, ok)
			}
		})
	}
}

func TestParseBraceTableReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec string
		want *TableReference
		err  string
	}{
		{name: "table", spec: "1", want: &TableReference{TableIndex: 1}},
		{name: "last table", spec: "-1", want: &TableReference{TableIndex: -1}},
		{name: "all tables", spec: "*", want: &TableReference{}},
		{name: "create", spec: "3x4", want: &TableReference{IsCreate: true, CreateRows: 3, CreateCols: 4}},
		{name: "create header", spec: "3X4:header", want: &TableReference{
			IsCreate: true, CreateRows: 3, CreateCols: 4, HasHeader: true,
		}},
		{name: "Excel cell", spec: "1!B2", want: &TableReference{TableIndex: 1, Row: 2, Col: 2}},
		{name: "numeric cell", spec: "2!3,4", want: &TableReference{TableIndex: 2, Row: 3, Col: 4}},
		{name: "all cells", spec: "1!*", want: &TableReference{TableIndex: 1, IsAllCells: true}},
		{name: "row wildcard", spec: "1!2,*", want: &TableReference{TableIndex: 1, Row: 2, RowWild: true}},
		{name: "column wildcard", spec: "1!*,3", want: &TableReference{TableIndex: 1, Col: 3, ColWild: true}},
		{name: "range", spec: "1!A1:C3", want: &TableReference{
			TableIndex: 1, Row: 1, Col: 1, HasRange: true, EndRow: 3, EndCol: 3,
		}},
		{name: "row operation", spec: "1!row=+2", want: &TableReference{TableIndex: 1, RowOp: "+2"}},
		{name: "column operation", spec: "1!col=$+", want: &TableReference{TableIndex: 1, ColOp: "$+"}},
		{name: "zero", spec: "0", err: "cannot be 0"},
		{name: "bad table", spec: "abc", err: "invalid table index"},
		{name: "bad create suffix", spec: "3x4:foo", err: "invalid table create suffix"},
		{name: "bad cell", spec: "1!nope", err: "invalid cell spec"},
		{name: "bad range row", spec: "1!x,1:2,2", err: "invalid row"},
		{name: "bad range column", spec: "1!1,x:2,2", err: "invalid col"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseBraceTableReference(test.spec)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) {
					t.Fatalf("error = %v, want containing %q", err, test.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBraceTableReference: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("reference = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseBraceImageReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec    string
		index   int
		all     bool
		pattern string
		err     bool
	}{
		{spec: "1", index: 1},
		{spec: "-2", index: -2},
		{spec: "*", all: true},
		{spec: "img-.*", pattern: "img-.*"},
		{spec: "", err: true},
		{spec: "[", err: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.spec, func(t *testing.T) {
			t.Parallel()
			got, err := ParseBraceImageReference(test.spec)
			if test.err {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBraceImageReference: %v", err)
			}
			if got.Position != test.index || got.AllImages != test.all || got.Pattern != test.pattern {
				t.Fatalf("reference = %+v", got)
			}
		})
	}
}

func TestDetectBraceReference(t *testing.T) {
	t.Parallel()
	remaining, table, image, err := DetectBraceReference("{T=1!A1}old")
	if err != nil || remaining != "old" || table == nil || table.Row != 1 || table.Col != 1 || image != nil {
		t.Fatalf("table result = (%q, %+v, %+v, %v)", remaining, table, image, err)
	}
	remaining, table, image, err = DetectBraceReference("{img=logo}")
	if err != nil || remaining != "" || table != nil || image == nil || image.Pattern != "logo" {
		t.Fatalf("image result = (%q, %+v, %+v, %v)", remaining, table, image, err)
	}
	remaining, table, image, err = DetectBraceReference("{T=1")
	if err != nil || remaining != "{T=1" || table != nil || image != nil {
		t.Fatalf("unclosed result = (%q, %+v, %+v, %v)", remaining, table, image, err)
	}
}

func TestParseImageReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  *ImageReference
		regex string
	}{
		{input: "!(1)", want: &ImageReference{ByPosition: true, Position: 1}},
		{input: "!(-1)", want: &ImageReference{ByPosition: true, Position: -1}},
		{input: "!(*)", want: &ImageReference{ByPosition: true, AllImages: true}},
		{input: "![](2)", want: &ImageReference{ByPosition: true, Position: 2}},
		{input: "![fig-.*]", want: &ImageReference{ByAlt: true}, regex: "fig-.*"},
		{input: "!(https://example.com/image.png)"},
		{input: "![alt](https://example.com/image.png)"},
		{input: "![[invalid]"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got := ParseImageReference(test.input)
			if test.want == nil {
				if got != nil {
					t.Fatalf("reference = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.ByPosition != test.want.ByPosition || got.Position != test.want.Position ||
				got.AllImages != test.want.AllImages || got.ByAlt != test.want.ByAlt {
				t.Fatalf("reference = %+v, want %+v", got, test.want)
			}
			if test.regex != "" && (got.AltRegex == nil || got.AltRegex.String() != test.regex) {
				t.Fatalf("regex = %v, want %q", got.AltRegex, test.regex)
			}
		})
	}
}

func TestParseTableCreate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  *TableCreateSpec
	}{
		{input: "|3x4|", want: &TableCreateSpec{Rows: 3, Columns: 4}},
		{input: "|3X4:HEADER|", want: &TableCreateSpec{Rows: 3, Columns: 4, Header: true}},
		{input: "|1x1|", want: &TableCreateSpec{Rows: 1, Columns: 1}},
		{input: "|101x1|"},
		{input: "|1x27|"},
		{input: "|3x4:foo|"},
		{input: "|1|[A1]"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			if got := ParseTableCreate(test.input); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ParseTableCreate(%q) = %+v, want %+v", test.input, got, test.want)
			}
		})
	}
}

func TestParsePipeTable(t *testing.T) {
	t.Parallel()
	got := ParsePipeTable("| Name | Role |\n|---|---|\n| Alice | Engineer |")
	want := &TableCreateSpec{
		Rows: 2, Columns: 2,
		Cells: [][]string{{"Name", "Role"}, {"Alice", "Engineer"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePipeTable = %+v, want %+v", got, want)
	}
	if got := ParsePipeTable("| A | B |\nnot a row"); got != nil {
		t.Fatalf("invalid table = %+v, want nil", got)
	}
}

func TestReferenceStrings(t *testing.T) {
	t.Parallel()
	if got := (&TableReference{TableIndex: 1, Row: 2, Col: 3}).String(); got != "{T=table:1 cell:[2,3]}" {
		t.Fatalf("table string = %q", got)
	}
	if got := (&TableReference{IsCreate: true, CreateRows: 3, CreateCols: 4, HasHeader: true}).String(); got != "{T=create:3x4 header}" {
		t.Fatalf("create string = %q", got)
	}
	if got := (&ImageReference{ByAlt: true, Pattern: "logo"}).String(); got != "{img=logo}" {
		t.Fatalf("image string = %q", got)
	}
}

func FuzzParseReferences(f *testing.F) {
	for _, seed := range []string{
		"|1|[A1]",
		"|1|[1,1:2,2]",
		"{T=1!A1}",
		"{img=logo}",
		"!(1)",
		"![alt]",
		"|3x4|",
		"| A | B |\n| C | D |",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		ParseTableReference(input)
		ParseTableCellReference(input)
		ParseExcelReference(input)
		_, _ = ParseBraceTableReference(input)
		_, _ = ParseBraceImageReference(input)
		_, _, _, _ = DetectBraceReference(input)
		ParseImageReference(input)
		ParseTableCreate(input)
		ParsePipeTable(input)
	})
}
