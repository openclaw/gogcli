//nolint:wsl_v5 // Planner fixtures stay compact around complete mutation assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestTableCreatePlannerUsesUTF16AndFirstMatch(t *testing.T) {
	t.Parallel()
	planner, err := NewTableCreatePlanner(
		Expression{Pattern: "slot", Global: true, NthMatch: 2},
		TableCreateSpec{Rows: 2, Columns: 3},
	)
	if err != nil {
		t.Fatal(err)
	}

	got := planner.Plan(DocumentSegment{TextRuns: []DocumentTextRun{
		{Text: "😀 slot slot", StartIndex: 10}, //nolint:dupword // Repetition verifies first-match selection.
		{Text: "slot", StartIndex: 40},
	}})
	want := &TableCreateMutation{
		StartIndex: 13,
		EndIndex:   17,
		Rows:       2,
		Columns:    3,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutation = %#v, want %#v", got, want)
	}
}

func TestTableCreatePlannerSearchesLaterProjectedRuns(t *testing.T) {
	t.Parallel()
	planner, err := NewTableCreatePlanner(
		Expression{Pattern: "nested"},
		TableCreateSpec{Rows: 1, Columns: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := planner.Plan(DocumentSegment{TextRuns: []DocumentTextRun{
		{Text: "before", StartIndex: 1},
		{Text: "nested\n", StartIndex: 30},
	}})
	want := &TableCreateMutation{StartIndex: 30, EndIndex: 36, Rows: 1, Columns: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutation = %#v, want %#v", got, want)
	}
}

func TestTableCreatePlannerNoMatch(t *testing.T) {
	t.Parallel()
	planner, err := NewTableCreatePlanner(
		Expression{Pattern: "missing"},
		TableCreateSpec{Rows: 1, Columns: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := planner.Plan(DocumentSegment{TextRuns: []DocumentTextRun{{Text: "body"}}}); got != nil {
		t.Fatalf("mutation = %#v", got)
	}
}

func TestNewTableCreatePlannerValidatesInput(t *testing.T) {
	t.Parallel()
	if _, err := NewTableCreatePlanner(Expression{Pattern: "["}, TableCreateSpec{Rows: 1, Columns: 1}); err == nil {
		t.Fatal("expected pattern error")
	}
	if _, err := NewTableCreatePlanner(Expression{Pattern: "x"}, TableCreateSpec{}); err == nil {
		t.Fatal("expected shape error")
	}
}
