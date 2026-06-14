//nolint:wsl_v5 // Validation, compilation, and compact mutation construction stay adjacent.
package docssed

import (
	"errors"
	"fmt"
)

var errInvalidTableShape = errors.New("invalid table shape")

// TableCreateMutation replaces one matched placeholder with a table.
type TableCreateMutation struct {
	StartIndex int64
	EndIndex   int64
	Rows       int
	Columns    int
}

// TableCreatePlanner owns the compiled placeholder expression for one table.
type TableCreatePlanner struct {
	matcher *MatchPlanner
	spec    TableCreateSpec
}

// NewTableCreatePlanner validates one table creation expression and shape.
func NewTableCreatePlanner(expression Expression, spec TableCreateSpec) (*TableCreatePlanner, error) {
	if spec.Rows < 1 || spec.Columns < 1 {
		return nil, fmt.Errorf("%w: %dx%d", errInvalidTableShape, spec.Rows, spec.Columns)
	}
	matcher, err := NewMatchPlanner(Expression{Pattern: expression.Pattern})
	if err != nil {
		return nil, err
	}
	return &TableCreatePlanner{matcher: matcher, spec: spec}, nil
}

// Plan returns the first placeholder mutation in source order.
func (p *TableCreatePlanner) Plan(segment DocumentSegment) *TableCreateMutation {
	actions := p.matcher.PlanSegment(segment)
	if len(actions) == 0 {
		return nil
	}
	return &TableCreateMutation{
		StartIndex: actions[0].StartIndex,
		EndIndex:   actions[0].EndIndex,
		Rows:       p.spec.Rows,
		Columns:    p.spec.Columns,
	}
}
