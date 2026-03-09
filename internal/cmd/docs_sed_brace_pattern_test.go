package cmd

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBraceTableRef_TableIndex(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		wantIndex  int
		wantErr    bool
		wantErrMsg string
	}{
		{"first table", "1", 1, false, ""},
		{"second table", "2", 2, false, ""},
		{"last table", "-1", -1, false, ""},
		{"second to last", "-2", -2, false, ""},
		{"all tables", "*", 0, false, ""},
		{"zero invalid", "0", 0, true, "cannot be 0"},
		{"invalid string", "abc", 0, true, "invalid table index"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantIndex, ref.TableIndex)
		})
	}
}

func TestParseBraceTableRef_Create(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		wantRows   int
		wantCols   int
		wantHeader bool
		wantErr    bool
	}{
		{"3x4", "3x4", 3, 4, false, false},
		{"5x2", "5x2", 5, 2, false, false},
		{"3x4:header", "3x4:header", 3, 4, true, false},
		{"5X3:header", "5X3:header", 5, 3, true, false}, // case insensitive
		{"1x1", "1x1", 1, 1, false, false},
		{"invalid suffix", "3x4:foo", 0, 0, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, ref.IsCreate)
			assert.Equal(t, tt.wantRows, ref.CreateRows)
			assert.Equal(t, tt.wantCols, ref.CreateCols)
			assert.Equal(t, tt.wantHeader, ref.HasHeader)
		})
	}
}

func TestParseBraceTableRef_Cell(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantRow int
		wantCol int
		wantErr bool
	}{
		{"excel A1", "1!A1", 1, 1, false},
		{"excel B2", "1!B2", 2, 2, false},
		{"excel C3", "1!C3", 3, 3, false},
		{"excel Z1", "1!Z1", 1, 26, false},
		{"excel AA1", "1!AA1", 1, 27, false},
		{"row,col 1,1", "1!1,1", 1, 1, false},
		{"row,col 2,3", "1!2,3", 2, 3, false},
		{"row,col 10,5", "1!10,5", 10, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, ref.TableIndex)
			assert.Equal(t, tt.wantRow, ref.Row)
			assert.Equal(t, tt.wantCol, ref.Col)
		})
	}
}

func TestParseBraceTableRef_Wildcards(t *testing.T) {
	tests := []struct {
		name        string
		spec        string
		wantAllCell bool
		wantRowWild bool
		wantColWild bool
		wantRow     int
		wantCol     int
	}{
		{"all cells", "1!*", true, false, false, 0, 0},
		{"entire row 1", "1!1,*", false, true, false, 1, 0},
		{"entire row 5", "1!5,*", false, true, false, 5, 0},
		{"entire col 2", "1!*,2", false, false, true, 0, 2},
		{"entire col 1", "1!*,1", false, false, true, 0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			require.NoError(t, err)
			assert.Equal(t, 1, ref.TableIndex)
			assert.Equal(t, tt.wantAllCell, ref.IsAllCells)
			assert.Equal(t, tt.wantRowWild, ref.RowWild)
			assert.Equal(t, tt.wantColWild, ref.ColWild)
			assert.Equal(t, tt.wantRow, ref.Row)
			assert.Equal(t, tt.wantCol, ref.Col)
		})
	}
}

func TestParseBraceTableRef_Range(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		wantRow    int
		wantCol    int
		wantEndRow int
		wantEndCol int
	}{
		{"excel A1:C3", "1!A1:C3", 1, 1, 3, 3},
		{"excel B2:D4", "1!B2:D4", 2, 2, 4, 4},
		{"row,col 1,1:3,3", "1!1,1:3,3", 1, 1, 3, 3},
		{"row,col 2,2:5,4", "1!2,2:5,4", 2, 2, 5, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			require.NoError(t, err)
			assert.True(t, ref.HasRange)
			assert.Equal(t, tt.wantRow, ref.Row)
			assert.Equal(t, tt.wantCol, ref.Col)
			assert.Equal(t, tt.wantEndRow, ref.EndRow)
			assert.Equal(t, tt.wantEndCol, ref.EndCol)
		})
	}
}

func TestParseBraceTableRef_RowOps(t *testing.T) {
	tests := []struct {
		name   string
		spec   string
		wantOp string
	}{
		{"insert before 2", "1!row=+2", "+2"},
		{"append row", "1!row=$+", "$+"},
		{"delete row 2", "1!row=2", "2"},
		{"delete last row", "1!row=-1", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOp, ref.RowOp)
		})
	}
}

func TestParseBraceTableRef_ColOps(t *testing.T) {
	tests := []struct {
		name   string
		spec   string
		wantOp string
	}{
		{"insert before 3", "1!col=+3", "+3"},
		{"append col", "1!col=$+", "$+"},
		{"delete col 3", "1!col=3", "3"},
		{"delete last col", "1!col=-1", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceTableRef(tt.spec)
			require.NoError(t, err)
			assert.Equal(t, tt.wantOp, ref.ColOp)
		})
	}
}

func TestParseBraceImgRef(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		wantIndex int
		wantAll   bool
		wantPat   string
		wantErr   bool
	}{
		{"first image", "1", 1, false, "", false},
		{"second image", "2", 2, false, "", false},
		{"last image", "-1", -1, false, "", false},
		{"second to last", "-2", -2, false, "", false},
		{"all images", "*", 0, true, "", false},
		{"regex pattern", "logo", 0, false, "logo", false},
		{"regex complex", "img-.*", 0, false, "img-.*", false},
		{"empty", "", 0, false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := parseBraceImgRef(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantIndex, ref.Index)
			assert.Equal(t, tt.wantAll, ref.IsAll)
			assert.Equal(t, tt.wantPat, ref.Pattern)
		})
	}
}

func TestDetectBracePattern_Table(t *testing.T) {
	tests := []struct {
		name          string
		pattern       string
		wantRemaining string
		wantTableIdx  int
		wantRow       int
		wantCol       int
		wantErr       bool
	}{
		{"table 1", "{T=1}", "", 1, 0, 0, false},
		{"table -1", "{T=-1}", "", -1, 0, 0, false},
		{"table * all", "{T=*}", "", 0, 0, 0, false},
		{"table cell A1", "{T=1!A1}", "", 1, 1, 1, false},
		{"table cell with pattern", "{T=1!A1}old", "old", 1, 1, 1, false},
		{"table row,col", "{T=2!3,4}", "", 2, 3, 4, false},
		{"not a brace pattern", "foo", "foo", 0, 0, 0, false},
		{"unclosed brace", "{T=1", "{T=1", 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining, tableRef, imgRef, err := detectBracePattern(tt.pattern)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRemaining, remaining)
			assert.Nil(t, imgRef)

			if tt.wantTableIdx != 0 || tt.wantRow != 0 || tt.wantCol != 0 {
				require.NotNil(t, tableRef)
				assert.Equal(t, tt.wantTableIdx, tableRef.TableIndex)
				assert.Equal(t, tt.wantRow, tableRef.Row)
				assert.Equal(t, tt.wantCol, tableRef.Col)
			}
		})
	}
}

func TestDetectBracePattern_Image(t *testing.T) {
	tests := []struct {
		name          string
		pattern       string
		wantRemaining string
		wantIndex     int
		wantAll       bool
		wantPattern   string
	}{
		{"image 1", "{img=1}", "", 1, false, ""},
		{"image -1", "{img=-1}", "", -1, false, ""},
		{"image all", "{img=*}", "", 0, true, ""},
		{"image regex", "{img=logo}", "", 0, false, "logo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining, tableRef, imgRef, err := detectBracePattern(tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.wantRemaining, remaining)
			assert.Nil(t, tableRef)
			require.NotNil(t, imgRef)
			assert.Equal(t, tt.wantIndex, imgRef.Index)
			assert.Equal(t, tt.wantAll, imgRef.IsAll)
			assert.Equal(t, tt.wantPattern, imgRef.Pattern)
		})
	}
}

func TestBraceTableToSedExpr_TableRef(t *testing.T) {
	tests := []struct {
		name         string
		bt           *braceTableRef
		wantTableRef int
		wantCellRef  bool
	}{
		{
			name:         "table 1",
			bt:           &braceTableRef{TableIndex: 1},
			wantTableRef: 1,
			wantCellRef:  false,
		},
		{
			name:         "table -1",
			bt:           &braceTableRef{TableIndex: -1},
			wantTableRef: -1,
			wantCellRef:  false,
		},
		{
			name:         "all tables",
			bt:           &braceTableRef{TableIndex: 0},
			wantTableRef: -2147483648, // math.MinInt32
			wantCellRef:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &sedExpr{}
			braceTableToSedExpr(tt.bt, expr)
			assert.Equal(t, tt.wantTableRef, expr.tableRef)
			if tt.wantCellRef {
				assert.NotNil(t, expr.cellRef)
			} else {
				assert.Nil(t, expr.cellRef)
			}
		})
	}
}

func TestBraceTableToSedExpr_CellRef(t *testing.T) {
	tests := []struct {
		name           string
		bt             *braceTableRef
		wantTableIndex int
		wantRow        int
		wantCol        int
	}{
		{
			name:           "single cell",
			bt:             &braceTableRef{TableIndex: 1, Row: 2, Col: 3},
			wantTableIndex: 1,
			wantRow:        2,
			wantCol:        3,
		},
		{
			name:           "all cells wildcard",
			bt:             &braceTableRef{TableIndex: 1, IsAllCells: true},
			wantTableIndex: 1,
			wantRow:        0,
			wantCol:        0,
		},
		{
			name:           "entire row",
			bt:             &braceTableRef{TableIndex: 1, Row: 2, RowWild: true},
			wantTableIndex: 1,
			wantRow:        2,
			wantCol:        0,
		},
		{
			name:           "entire col",
			bt:             &braceTableRef{TableIndex: 1, Col: 3, ColWild: true},
			wantTableIndex: 1,
			wantRow:        0,
			wantCol:        3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &sedExpr{}
			braceTableToSedExpr(tt.bt, expr)
			require.NotNil(t, expr.cellRef)
			assert.Equal(t, tt.wantTableIndex, expr.cellRef.tableIndex)
			assert.Equal(t, tt.wantRow, expr.cellRef.row)
			assert.Equal(t, tt.wantCol, expr.cellRef.col)
		})
	}
}

func TestBraceTableToSedExpr_RowColOps(t *testing.T) {
	tests := []struct {
		name      string
		bt        *braceTableRef
		wantRowOp string
		wantColOp string
		wantTgt   int
	}{
		{
			name:      "insert row",
			bt:        &braceTableRef{TableIndex: 1, RowOp: "+2"},
			wantRowOp: "insert",
			wantTgt:   2,
		},
		{
			name:      "append row",
			bt:        &braceTableRef{TableIndex: 1, RowOp: "$+"},
			wantRowOp: "append",
			wantTgt:   0,
		},
		{
			name:      "delete row",
			bt:        &braceTableRef{TableIndex: 1, RowOp: "3"},
			wantRowOp: "delete",
			wantTgt:   3,
		},
		{
			name:      "insert col",
			bt:        &braceTableRef{TableIndex: 1, ColOp: "+4"},
			wantColOp: "insert",
			wantTgt:   4,
		},
		{
			name:      "append col",
			bt:        &braceTableRef{TableIndex: 1, ColOp: "$+"},
			wantColOp: "append",
			wantTgt:   0,
		},
		{
			name:      "delete last col",
			bt:        &braceTableRef{TableIndex: 1, ColOp: "-1"},
			wantColOp: "delete",
			wantTgt:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := &sedExpr{}
			braceTableToSedExpr(tt.bt, expr)
			require.NotNil(t, expr.cellRef)
			if tt.wantRowOp != "" {
				assert.Equal(t, tt.wantRowOp, expr.cellRef.rowOp)
				assert.Equal(t, tt.wantTgt, expr.cellRef.opTarget)
			}
			if tt.wantColOp != "" {
				assert.Equal(t, tt.wantColOp, expr.cellRef.colOp)
				assert.Equal(t, tt.wantTgt, expr.cellRef.opTarget)
			}
		})
	}
}

func TestBraceImgToImageRefPattern(t *testing.T) {
	tests := []struct {
		name      string
		bi        *braceImgRef
		wantPos   bool
		wantAll   bool
		wantIdx   int
		wantByAlt bool
	}{
		{
			name:    "position 1",
			bi:      &braceImgRef{Index: 1},
			wantPos: true,
			wantIdx: 1,
		},
		{
			name:    "position -1",
			bi:      &braceImgRef{Index: -1},
			wantPos: true,
			wantIdx: -1,
		},
		{
			name:    "all images",
			bi:      &braceImgRef{IsAll: true},
			wantPos: true,
			wantAll: true,
		},
		{
			name:      "by alt pattern",
			bi:        &braceImgRef{Pattern: "logo", Regex: mustCompileRegex("logo")},
			wantByAlt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := braceImgToImageRefPattern(tt.bi)
			require.NotNil(t, ref)
			assert.Equal(t, tt.wantPos, ref.ByPosition)
			assert.Equal(t, tt.wantAll, ref.AllImages)
			assert.Equal(t, tt.wantIdx, ref.Position)
			assert.Equal(t, tt.wantByAlt, ref.ByAlt)
		})
	}
}

func TestParseFullExpr_BraceTablePattern(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantTableRef int
		wantCellRef  bool
		wantRow      int
		wantCol      int
		wantPattern  string
	}{
		{
			name:         "table delete",
			raw:          "s/{T=1}//",
			wantTableRef: 1,
		},
		{
			name:         "all tables delete",
			raw:          "s/{T=*}//",
			wantTableRef: -2147483648,
		},
		{
			name:        "cell replace",
			raw:         "s/{T=1!A1}/new/",
			wantCellRef: true,
			wantRow:     1,
			wantCol:     1,
		},
		{
			name:        "cell with subpattern",
			raw:         "s/{T=1!A1}old/new/",
			wantCellRef: true,
			wantRow:     1,
			wantCol:     1,
			wantPattern: "old",
		},
		{
			name:        "row wildcard",
			raw:         "s/{T=2!1,*}/header/",
			wantCellRef: true,
			wantRow:     1,
			wantCol:     0,
		},
		{
			name:        "col wildcard",
			raw:         "s/{T=2!*,3}/value/",
			wantCellRef: true,
			wantRow:     0,
			wantCol:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseFullExpr(tt.raw)
			require.NoError(t, err)

			if tt.wantTableRef != 0 {
				assert.Equal(t, tt.wantTableRef, expr.tableRef)
			}

			if tt.wantCellRef {
				require.NotNil(t, expr.cellRef)
				assert.Equal(t, tt.wantRow, expr.cellRef.row)
				assert.Equal(t, tt.wantCol, expr.cellRef.col)
			}

			if tt.wantPattern != "" {
				assert.Equal(t, tt.wantPattern, expr.pattern)
			}
		})
	}
}

func TestParseFullExpr_BraceImagePattern(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantPattern string
	}{
		{"image 1", "s/{img=1}//", "!(1)"},
		{"image -1", "s/{img=-1}//", "!(-1)"},
		{"image all", "s/{img=*}//", "!(*)"},
		{"image regex", "s/{img=logo}//", "![logo]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parseFullExpr(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPattern, expr.pattern)
		})
	}
}

func TestBraceTableRef_String(t *testing.T) {
	tests := []struct {
		name string
		bt   *braceTableRef
		want string
	}{
		{"nil", nil, "<nil>"},
		{"create", &braceTableRef{IsCreate: true, CreateRows: 3, CreateCols: 4}, "{T=create:3x4}"},
		{"create header", &braceTableRef{IsCreate: true, CreateRows: 3, CreateCols: 4, HasHeader: true}, "{T=create:3x4 header}"},
		{"table 1", &braceTableRef{TableIndex: 1}, "{T=table:1}"},
		{"all tables", &braceTableRef{TableIndex: 0}, "{T=table:*}"},
		{"cell", &braceTableRef{TableIndex: 1, Row: 2, Col: 3}, "{T=table:1 cell:[2,3]}"},
		{"all cells", &braceTableRef{TableIndex: 1, IsAllCells: true}, "{T=table:1 cells:*}"},
		{"row wild", &braceTableRef{TableIndex: 1, Row: 2, RowWild: true}, "{T=table:1 row:2,*}"},
		{"col wild", &braceTableRef{TableIndex: 1, Col: 3, ColWild: true}, "{T=table:1 col:*,3}"},
		{"range", &braceTableRef{TableIndex: 1, Row: 1, Col: 1, EndRow: 3, EndCol: 3, HasRange: true}, "{T=table:1 range:[1,1:3,3]}"},
		{"row op", &braceTableRef{TableIndex: 1, RowOp: "+2"}, "{T=table:1 rowOp:+2}"},
		{"col op", &braceTableRef{TableIndex: 1, ColOp: "$+"}, "{T=table:1 colOp:$+}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.bt.String())
		})
	}
}

func TestBraceImgRef_String(t *testing.T) {
	tests := []struct {
		name string
		bi   *braceImgRef
		want string
	}{
		{"nil", nil, "<nil>"},
		{"index 1", &braceImgRef{Index: 1}, "{img=1}"},
		{"index -1", &braceImgRef{Index: -1}, "{img=-1}"},
		{"all", &braceImgRef{IsAll: true}, "{img=*}"},
		{"pattern", &braceImgRef{Pattern: "logo"}, "{img=logo}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.bi.String())
		})
	}
}

func TestBraceTableToTableCreateSpec(t *testing.T) {
	tests := []struct {
		name       string
		bt         *braceTableRef
		wantNil    bool
		wantRows   int
		wantCols   int
		wantHeader bool
	}{
		{"nil", nil, true, 0, 0, false},
		{"not create", &braceTableRef{TableIndex: 1}, true, 0, 0, false},
		{"create 3x4", &braceTableRef{IsCreate: true, CreateRows: 3, CreateCols: 4}, false, 3, 4, false},
		{"create 2x3 header", &braceTableRef{IsCreate: true, CreateRows: 2, CreateCols: 3, HasHeader: true}, false, 2, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := braceTableToTableCreateSpec(tt.bt)
			if tt.wantNil {
				assert.Nil(t, spec)
				return
			}
			require.NotNil(t, spec)
			assert.Equal(t, tt.wantRows, spec.rows)
			assert.Equal(t, tt.wantCols, spec.cols)
			assert.Equal(t, tt.wantHeader, spec.header)
		})
	}
}

func TestParseRowColOpValue(t *testing.T) {
	tests := []struct {
		op         string
		wantOp     string
		wantTarget int
	}{
		{"$+", "append", 0},
		{"+2", "insert", 2},
		{"+10", "insert", 10},
		{"3", "delete", 3},
		{"-1", "delete", -1},
		{"-5", "delete", -5},
		{"invalid", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			op, target := parseRowColOpValue(tt.op)
			assert.Equal(t, tt.wantOp, op)
			assert.Equal(t, tt.wantTarget, target)
		})
	}
}

// Helper to create compiled regex
func mustCompileRegex(pattern string) *regexp.Regexp {
	re, _ := regexp.Compile(pattern)
	return re
}
