//nolint:wsl_v5 // Phase fixtures stay compact around complete plan assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestPlanTextMutationsSeparatesExecutionPhases(t *testing.T) {
	t.Parallel()
	brace := &BraceExpression{Bold: boolValue(true)}
	span := &BraceSpan{Expr: brace, Start: 0, End: 1}
	image := &ImageSpec{URL: "https://example.com/image.png", Width: 100}
	actions := []MatchAction{
		{
			StartIndex: 1,
			EndIndex:   4,
			Replacement: Replacement{
				Kind:  ReplacementImage,
				Image: image,
			},
		},
		{
			StartIndex: 5,
			EndIndex:   8,
			Replacement: Replacement{
				Kind:    ReplacementText,
				Text:    "note",
				Formats: []string{"footnote"},
			},
		},
		{
			StartIndex: 9,
			EndIndex:   12,
			Replacement: Replacement{
				Kind:    ReplacementText,
				Text:    "\n",
				Formats: []string{"hrule"},
			},
		},
		{
			StartIndex: 13,
			EndIndex:   16,
			Replacement: Replacement{
				Kind:    ReplacementText,
				Text:    "\t😀code",
				Formats: []string{"codeblock"},
			},
		},
		{
			StartIndex: 20,
			EndIndex:   23,
			Replacement: Replacement{
				Kind:       ReplacementText,
				Text:       "x",
				Brace:      brace,
				BraceSpans: []*BraceSpan{span},
			},
		},
	}

	plan := PlanTextMutations(actions)
	if plan.MatchCount != 5 {
		t.Fatalf("match count = %d", plan.MatchCount)
	}
	if !reflect.DeepEqual(plan.Images, []ImageMutation{{
		StartIndex: 1,
		EndIndex:   4,
		Image:      image,
	}}) {
		t.Fatalf("images = %#v", plan.Images)
	}
	if !reflect.DeepEqual(plan.Footnotes, []FootnoteMutation{{
		StartIndex: 3,
		EndIndex:   6,
		Text:       "note",
	}}) {
		t.Fatalf("footnotes = %#v", plan.Footnotes)
	}
	wantEdits := []TextEdit{
		{StartIndex: 7, EndIndex: 10, InsertText: "\n", HorizontalRule: true},
		{StartIndex: 11, EndIndex: 14, InsertText: "\t😀code"},
		{StartIndex: 18, EndIndex: 21, InsertText: "x"},
	}
	if !reflect.DeepEqual(plan.TextEdits, wantEdits) {
		t.Fatalf("text edits = %#v, want %#v", plan.TextEdits, wantEdits)
	}
	wantFormatting := []FormatIntent{
		{
			StartIndex:           9,
			EndIndex:             16,
			StructuralStartIndex: 7,
			StructuralEndIndex:   14,
			Formats:              []string{"codeblock", "code"},
			LeadingTab:           true,
		},
		{
			StartIndex:           20,
			EndIndex:             21,
			StructuralStartIndex: 18,
			StructuralEndIndex:   19,
			Brace:                brace,
			BraceSpans:           []*BraceSpan{span},
		},
	}
	if !reflect.DeepEqual(plan.Formatting, wantFormatting) {
		t.Fatalf("formatting = %#v, want %#v", plan.Formatting, wantFormatting)
	}
}

func TestPlanTextMutationsPreservesOrderAndDeletion(t *testing.T) {
	t.Parallel()
	actions := []MatchAction{
		{StartIndex: 2, EndIndex: 3, Replacement: Replacement{Kind: ReplacementText}},
		{
			StartIndex: 8,
			EndIndex:   9,
			Replacement: Replacement{
				Kind:    ReplacementText,
				Text:    "bold",
				Formats: []string{"bold"},
			},
		},
	}
	plan := PlanTextMutations(actions)
	want := []TextEdit{
		{StartIndex: 2, EndIndex: 3},
		{StartIndex: 8, EndIndex: 9, InsertText: "bold"},
	}
	if !reflect.DeepEqual(plan.TextEdits, want) {
		t.Fatalf("text edits = %#v, want %#v", plan.TextEdits, want)
	}
	if len(plan.Formatting) != 1 || plan.Formatting[0].StartIndex != 7 ||
		plan.Formatting[0].EndIndex != 11 ||
		plan.Formatting[0].StructuralStartIndex != 7 ||
		plan.Formatting[0].StructuralEndIndex != 11 {
		t.Fatalf("formatting = %#v", plan.Formatting)
	}
}

func TestPlanTextMutationsTransformsMixedPhaseIndices(t *testing.T) {
	t.Parallel()
	brace := &BraceExpression{Bold: boolValue(true)}
	actions := []MatchAction{
		{
			StartIndex: 1,
			EndIndex:   4,
			Replacement: Replacement{
				Kind:  ReplacementImage,
				Image: &ImageSpec{URL: "https://example.com/image.png"},
			},
		},
		{
			StartIndex: 10,
			EndIndex:   13,
			Replacement: Replacement{
				Kind: ReplacementText,
				Text: "longer",
			},
		},
		{
			StartIndex: 20,
			EndIndex:   24,
			Replacement: Replacement{
				Kind:    ReplacementText,
				Text:    "note",
				Formats: []string{"footnote"},
			},
		},
		{
			StartIndex: 30,
			EndIndex:   33,
			Replacement: Replacement{
				Kind:  ReplacementText,
				Text:  "x",
				Brace: brace,
			},
		},
	}

	plan := PlanTextMutations(actions)
	wantText := []TextEdit{
		{StartIndex: 8, EndIndex: 11, InsertText: "longer"},
		{StartIndex: 28, EndIndex: 31, InsertText: "x"},
	}
	if !reflect.DeepEqual(plan.TextEdits, wantText) {
		t.Fatalf("text edits = %#v, want %#v", plan.TextEdits, wantText)
	}
	if !reflect.DeepEqual(plan.Footnotes, []FootnoteMutation{{
		StartIndex: 21,
		EndIndex:   25,
		Text:       "note",
	}}) {
		t.Fatalf("footnotes = %#v", plan.Footnotes)
	}
	if len(plan.Formatting) != 1 {
		t.Fatalf("formatting = %#v", plan.Formatting)
	}
	formatting := plan.Formatting[0]
	if formatting.StartIndex != 31 || formatting.EndIndex != 32 ||
		formatting.StructuralStartIndex != 28 || formatting.StructuralEndIndex != 29 {
		t.Fatalf("formatting = %#v", formatting)
	}
}

func boolValue(value bool) *bool {
	return &value
}
