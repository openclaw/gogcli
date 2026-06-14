package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBraceTableToSedExpr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		ref          *braceTableRef
		wantTableRef int
		wantCell     *tableCellRef
	}{
		{name: "table", ref: &braceTableRef{TableIndex: 1}, wantTableRef: 1},
		{name: "last table", ref: &braceTableRef{TableIndex: -1}, wantTableRef: -1},
		{name: "all tables", ref: &braceTableRef{}, wantTableRef: -2147483648},
		{name: "cell", ref: &braceTableRef{TableIndex: 1, Row: 2, Col: 3}, wantCell: &tableCellRef{
			tableIndex: 1, row: 2, col: 3,
		}},
		{name: "all cells", ref: &braceTableRef{TableIndex: 1, IsAllCells: true}, wantCell: &tableCellRef{
			tableIndex: 1,
		}},
		{name: "row wildcard", ref: &braceTableRef{TableIndex: 1, Row: 2, RowWild: true}, wantCell: &tableCellRef{
			tableIndex: 1, row: 2,
		}},
		{name: "column wildcard", ref: &braceTableRef{TableIndex: 1, Col: 3, ColWild: true}, wantCell: &tableCellRef{
			tableIndex: 1, col: 3,
		}},
		{name: "range", ref: &braceTableRef{
			TableIndex: 1, Row: 1, Col: 1, HasRange: true, EndRow: 2, EndCol: 3,
		}, wantCell: &tableCellRef{tableIndex: 1, row: 1, col: 1, endRow: 2, endCol: 3}},
		{name: "insert row", ref: &braceTableRef{TableIndex: 1, RowOp: "+2"}, wantCell: &tableCellRef{
			tableIndex: 1, rowOp: opInsert, opTarget: 2,
		}},
		{name: "append column", ref: &braceTableRef{TableIndex: 1, ColOp: "$+"}, wantCell: &tableCellRef{
			tableIndex: 1, colOp: opAppend,
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expression := &sedExpr{}
			braceTableToSedExpr(test.ref, expression)
			assert.Equal(t, test.wantTableRef, expression.tableRef)
			assert.Equal(t, test.wantCell, expression.cellRef)
		})
	}
}

func TestParseFullExprBraceReferences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		raw          string
		wantTableRef int
		wantCell     *tableCellRef
		wantPattern  string
	}{
		{name: "table", raw: "s/{T=1}//", wantTableRef: 1},
		{name: "all tables", raw: "s/{T=*}//", wantTableRef: -2147483648},
		{name: "cell", raw: "s/{T=1!A1}/new/", wantCell: &tableCellRef{tableIndex: 1, row: 1, col: 1}},
		{name: "cell subpattern", raw: "s/{T=1!A1}old/new/", wantCell: &tableCellRef{
			tableIndex: 1, row: 1, col: 1,
		}, wantPattern: "old"},
		{name: "row wildcard", raw: "s/{T=2!1,*}/header/", wantCell: &tableCellRef{
			tableIndex: 2, row: 1,
		}},
		{name: "image", raw: "s/{img=1}//", wantPattern: "!(1)"},
		{name: "all images", raw: "s/{img=*}//", wantPattern: "!(*)"},
		{name: "image regex", raw: "s/{img=logo}//", wantPattern: "![logo]"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expression, err := parseFullExpr(test.raw)
			require.NoError(t, err)
			assert.Equal(t, test.wantTableRef, expression.tableRef)
			assert.Equal(t, test.wantCell, expression.cellRef)
			assert.Equal(t, test.wantPattern, expression.pattern)
		})
	}
}

func TestParseRowColOpValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value     string
		operation string
		target    int
	}{
		{value: "$+", operation: opAppend},
		{value: "+2", operation: opInsert, target: 2},
		{value: "3", operation: opDelete, target: 3},
		{value: "-1", operation: opDelete, target: -1},
		{value: "invalid"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()
			operation, target := parseRowColOpValue(test.value)
			assert.Equal(t, test.operation, operation)
			assert.Equal(t, test.target, target)
		})
	}
}
