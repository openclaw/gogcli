package cmd

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/steipete/gogcli/internal/docssed"
)

type (
	braceTableRef = docssed.TableReference
)

// ImageRefPattern is the parser-owned image reference used by the command executor.
type ImageRefPattern = docssed.ImageReference

func tableCellRefFromParsed(ref *docssed.CellReference) *tableCellRef {
	if ref == nil {
		return nil
	}
	return &tableCellRef{
		tableIndex: ref.TableIndex,
		row:        ref.Row,
		col:        ref.Column,
		subPattern: ref.Subpattern,
		rowOp:      ref.RowOperation,
		colOp:      ref.ColumnOperation,
		opTarget:   ref.OperationTarget,
		endRow:     ref.EndRow,
		endCol:     ref.EndColumn,
	}
}

func imageReferencePattern(ref *docssed.ImageReference) string {
	switch {
	case ref == nil:
		return ""
	case ref.AllImages:
		return "!(*)"
	case ref.ByPosition:
		return fmt.Sprintf("!(%d)", ref.Position)
	case ref.ByAlt && ref.AltRegex != nil:
		return fmt.Sprintf("![%s]", ref.Pattern)
	default:
		return ""
	}
}

// braceTableToSedExpr adapts the parser AST to the command executor's current shape.
func braceTableToSedExpr(ref *docssed.TableReference, expression *sedExpr) {
	if ref == nil {
		return
	}
	if !ref.IsAllCells && !ref.HasRange && ref.Row == 0 && ref.Col == 0 &&
		ref.RowOp == "" && ref.ColOp == "" && !ref.IsCreate {
		if ref.TableIndex == 0 {
			expression.tableRef = math.MinInt32
		} else {
			expression.tableRef = ref.TableIndex
		}
		return
	}
	if ref.IsCreate {
		return
	}

	cellRef := &tableCellRef{tableIndex: ref.TableIndex}
	if ref.RowOp != "" {
		cellRef.rowOp, cellRef.opTarget = parseRowColOpValue(ref.RowOp)
	}
	if ref.ColOp != "" {
		cellRef.colOp, cellRef.opTarget = parseRowColOpValue(ref.ColOp)
	}
	switch {
	case ref.IsAllCells:
		cellRef.row = 0
		cellRef.col = 0
	case ref.RowWild:
		cellRef.row = ref.Row
		cellRef.col = 0
	case ref.ColWild:
		cellRef.row = 0
		cellRef.col = ref.Col
	case ref.HasRange:
		cellRef.row = ref.Row
		cellRef.col = ref.Col
		cellRef.endRow = ref.EndRow
		cellRef.endCol = ref.EndCol
	default:
		cellRef.row = ref.Row
		cellRef.col = ref.Col
	}
	expression.cellRef = cellRef
}

func parseRowColOpValue(value string) (string, int) {
	value = strings.TrimSpace(value)
	if value == "$+" {
		return opAppend, 0
	}
	if strings.HasPrefix(value, "+") {
		parsedTarget, err := strconv.Atoi(value[1:])
		if err == nil {
			return opInsert, parsedTarget
		}
		return "", 0
	}
	parsedTarget, err := strconv.Atoi(value)
	if err == nil {
		return opDelete, parsedTarget
	}
	return "", 0
}
