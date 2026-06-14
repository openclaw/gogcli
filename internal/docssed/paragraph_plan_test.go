//nolint:wsl_v5 // Planner fixtures stay compact around complete mutation assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestParagraphPlannerDeleteUsesTopLevelBlocks(t *testing.T) {
	t.Parallel()
	planner, err := NewParagraphPlanner(Expression{Pattern: "(?i)match"})
	if err != nil {
		t.Fatal(err)
	}
	segment := DocumentSegment{
		Paragraphs: []DocumentParagraph{
			{Text: "MATCH\n", StartIndex: 0, EndIndex: 7},
			{Text: "match nested\n", StartIndex: 10, EndIndex: 23},
			{Text: "no\n", StartIndex: 30, EndIndex: 33},
			{Text: "match\n\n", StartIndex: 40, EndIndex: 47},
		},
		Blocks: []DocumentBlock{
			{Kind: DocumentBlockParagraph, ItemIndex: 0},
			{Kind: DocumentBlockTable, ItemIndex: 0},
			{Kind: DocumentBlockParagraph, ItemIndex: 2},
			{Kind: DocumentBlockParagraph, ItemIndex: 3},
		},
	}

	got := planner.PlanDelete(segment)
	want := []AddressMutation{
		{StartIndex: 1, EndIndex: 7},
		{StartIndex: 40, EndIndex: 47},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mutations = %#v, want %#v", got, want)
	}
}

func TestParagraphPlannerInsertAndAppendNormalizeNewlines(t *testing.T) {
	t.Parallel()
	planner, err := NewParagraphPlanner(Expression{Pattern: "target"})
	if err != nil {
		t.Fatal(err)
	}
	segment := DocumentSegment{
		Paragraphs: []DocumentParagraph{
			{Text: "target\n", StartIndex: 5, EndIndex: 12},
			{Text: "target two\n", StartIndex: 20, EndIndex: 31},
		},
		Blocks: []DocumentBlock{
			{Kind: DocumentBlockParagraph, ItemIndex: 0},
			{Kind: DocumentBlockParagraph, ItemIndex: 1},
		},
	}

	before := planner.PlanInsert(segment, `one\ntwo`, true)
	wantBefore := []AddressMutation{
		{StartIndex: 5, InsertText: "one\ntwo\n"},
		{StartIndex: 20, InsertText: "one\ntwo\n"},
	}
	if !reflect.DeepEqual(before, wantBefore) {
		t.Fatalf("before = %#v, want %#v", before, wantBefore)
	}

	after := planner.PlanInsert(segment, "after\n", false)
	wantAfter := []AddressMutation{
		{StartIndex: 12, InsertText: "after\n"},
		{StartIndex: 31, InsertText: "after\n"},
	}
	if !reflect.DeepEqual(after, wantAfter) {
		t.Fatalf("after = %#v, want %#v", after, wantAfter)
	}
}

func TestNewParagraphPlannerRejectsInvalidPattern(t *testing.T) {
	t.Parallel()
	if _, err := NewParagraphPlanner(Expression{Pattern: "["}); err == nil {
		t.Fatal("expected compile error")
	}
}
