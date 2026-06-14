//nolint:wsl_v5 // Planner fixtures stay compact around complete plan assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestPlanWholeCellReplacementPreservesNewlineAndFormatsUTF16(t *testing.T) {
	t.Parallel()
	plan := PlanWholeCellReplacement(CellInput{
		Text:           "old\n",
		TextStartIndex: 10,
		TextEndIndex:   14,
	}, "**A😀**")

	wantEdits := []TextEdit{{StartIndex: 10, EndIndex: 13, InsertText: "A😀"}}
	if !reflect.DeepEqual(plan.TextEdits, wantEdits) {
		t.Fatalf("text edits = %#v, want %#v", plan.TextEdits, wantEdits)
	}
	wantFormatting := []FormatIntent{{
		StartIndex:           10,
		EndIndex:             13,
		StructuralStartIndex: 10,
		StructuralEndIndex:   13,
		Formats:              []string{"bold"},
	}}
	if !reflect.DeepEqual(plan.Formatting, wantFormatting) {
		t.Fatalf("formatting = %#v, want %#v", plan.Formatting, wantFormatting)
	}
}

func TestPlanWholeCellReplacementExpandsWholeCellAndLiteralDollar(t *testing.T) {
	t.Parallel()
	plan := PlanWholeCellReplacement(CellInput{
		Text:           "value\n\n",
		TextStartIndex: 5,
		TextEndIndex:   12,
	}, "$$[${0}]")

	if plan.MatchCount != 1 || len(plan.TextEdits) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.TextEdits[0].EndIndex != 11 || plan.TextEdits[0].InsertText != "$[value]" {
		t.Fatalf("text edit = %#v", plan.TextEdits[0])
	}
}

func TestPlanCellInsertionKeepsSedBackreferenceSyntaxLiteral(t *testing.T) {
	t.Parallel()
	plan := PlanCellInsertion(7, "**${0} $$**")
	if len(plan.TextEdits) != 1 || plan.TextEdits[0].StartIndex != 7 ||
		plan.TextEdits[0].EndIndex != 7 ||
		plan.TextEdits[0].InsertText != "${0} $$" {
		t.Fatalf("text edits = %#v", plan.TextEdits)
	}
	if len(plan.Formatting) != 1 || !reflect.DeepEqual(plan.Formatting[0].Formats, []string{"bold"}) {
		t.Fatalf("formatting = %#v", plan.Formatting)
	}
}

func TestPlanCellReplacementUsesUTF16CapturesAndGlobalSelection(t *testing.T) {
	t.Parallel()
	plan, err := PlanCellReplacement(CellInput{
		Text:           "😀 ab ab\n",
		TextStartIndex: 20,
		TextEndIndex:   29,
	}, Expression{
		Pattern:     `(a)(b)`,
		Replacement: `${2}${1}!`,
		Global:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []TextEdit{
		{StartIndex: 23, EndIndex: 25, InsertText: "ba!"},
		{StartIndex: 26, EndIndex: 28, InsertText: "ba!"},
	}
	if !reflect.DeepEqual(plan.TextEdits, want) {
		t.Fatalf("text edits = %#v, want %#v", plan.TextEdits, want)
	}
	if plan.MatchCount != 2 {
		t.Fatalf("match count = %d", plan.MatchCount)
	}
}

func TestCellPlannerExpandsEachCellIndependently(t *testing.T) {
	t.Parallel()
	planner, err := NewCellPlanner(Expression{
		Pattern:     `([a-z]+)`,
		Replacement: `[${0}]`,
	})
	if err != nil {
		t.Fatal(err)
	}

	first := planner.Plan(CellInput{Text: "alpha\n", TextStartIndex: 1, TextEndIndex: 7})
	second := planner.Plan(CellInput{Text: "beta\n", TextStartIndex: 20, TextEndIndex: 25})
	if first.TextEdits[0].InsertText != "[alpha]" || second.TextEdits[0].InsertText != "[beta]" {
		t.Fatalf("first = %#v, second = %#v", first, second)
	}
}

func TestPlanCellReplacementSupportsNthAndKeepsSubpatternMarkdownLiteral(t *testing.T) {
	t.Parallel()
	plan, err := PlanCellReplacement(CellInput{
		Text:           "x x x\n", //nolint:dupword // Repetition is the nth-match fixture.
		TextStartIndex: 4,
		TextEndIndex:   10,
	}, Expression{
		Pattern:     "x",
		Replacement: "**y**",
		NthMatch:    2,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []TextEdit{{StartIndex: 6, EndIndex: 7, InsertText: "**y**"}}
	if !reflect.DeepEqual(plan.TextEdits, want) {
		t.Fatalf("text edits = %#v, want %#v", plan.TextEdits, want)
	}
	if len(plan.Formatting) != 0 {
		t.Fatalf("formatting = %#v", plan.Formatting)
	}
}

func TestPlanCellReplacementRejectsInvalidPattern(t *testing.T) {
	t.Parallel()
	if _, err := PlanCellReplacement(CellInput{}, Expression{Pattern: "["}); err == nil {
		t.Fatal("expected compile error")
	}
}
