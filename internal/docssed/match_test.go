//nolint:wsl_v5 // Planner fixtures stay compact around staged assertions.
package docssed

import (
	"reflect"
	"testing"
)

func TestPlanMatchesPreservesRunBoundariesAndUTF16Ranges(t *testing.T) {
	t.Parallel()
	segment := DocumentSegment{TextRuns: []DocumentTextRun{
		{Text: "A😀 fo", StartIndex: 7, EndIndex: 13},
		{Text: "o foo", StartIndex: 13, EndIndex: 18},
	}}

	actions, err := PlanMatches(segment, Expression{
		Pattern:     "foo",
		Replacement: "**bar**",
		Global:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []MatchAction{{
		StartIndex: 15,
		EndIndex:   18,
		OldText:    "foo",
		Replacement: Replacement{
			Kind:         ReplacementText,
			ExpandedText: "**bar**",
			Text:         "bar",
			Formats:      []string{"bold"},
		},
	}}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions = %#v, want %#v", actions, want)
	}
}

func TestPlanMatchesGlobalAndNthSelection(t *testing.T) {
	t.Parallel()
	segment := DocumentSegment{TextRuns: []DocumentTextRun{
		{Text: "foo foo", StartIndex: 1}, //nolint:dupword // Repetition is the match fixture.
		{Text: "foo foo", StartIndex: 9}, //nolint:dupword // Repetition is the match fixture.
	}}

	nonGlobal, err := PlanMatches(segment, Expression{Pattern: "foo", Replacement: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchStarts(nonGlobal); !reflect.DeepEqual(got, []int64{1, 9}) {
		t.Fatalf("non-global starts = %v", got)
	}

	global, err := PlanMatches(segment, Expression{Pattern: "foo", Replacement: "x", Global: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchStarts(global); !reflect.DeepEqual(got, []int64{1, 5, 9, 13}) {
		t.Fatalf("global starts = %v", got)
	}

	nth, err := PlanMatches(segment, Expression{Pattern: "foo", Replacement: "x", NthMatch: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchStarts(nth); !reflect.DeepEqual(got, []int64{9}) {
		t.Fatalf("nth starts = %v", got)
	}
}

func TestPlanMatchesExpandsAndClassifiesReplacements(t *testing.T) {
	t.Parallel()
	segment := DocumentSegment{TextRuns: []DocumentTextRun{{Text: "first last", StartIndex: 1}}}

	actions, err := PlanMatches(segment, Expression{
		Pattern:     `(first) (last)`,
		Replacement: `${2}, ${1}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Replacement.Text != "last, first" {
		t.Fatalf("capture actions = %#v", actions)
	}

	imageActions, err := PlanMatches(segment, Expression{
		Pattern:     `first last`,
		Replacement: `![Logo](https://example.com/logo.png){width=100}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantImage := &ImageSpec{URL: "https://example.com/logo.png", Alt: "Logo", Width: 100}
	if len(imageActions) != 1 ||
		imageActions[0].Replacement.Kind != ReplacementImage ||
		!reflect.DeepEqual(imageActions[0].Replacement.Image, wantImage) {
		t.Fatalf("image actions = %#v", imageActions)
	}

	brace := &BraceExpression{ImgRef: "https://example.com/brace.png", Width: 50}
	braceActions, err := PlanMatches(segment, Expression{
		Pattern:     `first last`,
		Replacement: "ignored",
		Brace:       brace,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantBraceImage := &ImageSpec{URL: "https://example.com/brace.png", Width: 50}
	if len(braceActions) != 1 || !reflect.DeepEqual(braceActions[0].Replacement.Image, wantBraceImage) {
		t.Fatalf("brace actions = %#v", braceActions)
	}
}

func TestPlanMatchesPreservesSedReplacementExpansion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		expression string
		text       string
		want       string
	}{
		{name: "numeric capture", expression: `s/(foo)/\1-\1/`, text: "foo", want: "foo-foo"},
		{name: "whole match", expression: `s/foo/[&]/`, text: "foo", want: "[foo]"},
		{name: "literal ampersand", expression: `s/foo/\&/`, text: "foo", want: "&"},
		{name: "literal dollar", expression: `s/foo/$/`, text: "foo", want: "$"},
		{name: "named capture", expression: `s/(?P<word>foo)/${word}!/`, text: "foo", want: "foo!"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expression, err := ParseExpression(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			actions, err := PlanMatches(
				DocumentSegment{TextRuns: []DocumentTextRun{{Text: test.text, StartIndex: 1}}},
				expression,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) != 1 || actions[0].Replacement.ExpandedText != test.want {
				t.Fatalf("actions = %#v, want expanded text %q", actions, test.want)
			}
		})
	}
}

func TestPlanMatchesMalformedImageFallsBackToText(t *testing.T) {
	t.Parallel()
	actions, err := PlanMatches(
		DocumentSegment{TextRuns: []DocumentTextRun{{Text: "foo", StartIndex: 1}}},
		Expression{Pattern: "foo", Replacement: "![alt](missing-close"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 ||
		actions[0].Replacement.Kind != ReplacementText ||
		actions[0].Replacement.Text != "![alt](missing-close" {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestPlanMatchesZeroWidthRange(t *testing.T) {
	t.Parallel()
	actions, err := PlanMatches(
		DocumentSegment{TextRuns: []DocumentTextRun{{Text: "😀x", StartIndex: 5}}},
		Expression{Pattern: "^", Replacement: "start"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].StartIndex != 5 || actions[0].EndIndex != 5 {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestPlanParagraphMatchesUsesUTF16AndNthPerParagraph(t *testing.T) {
	t.Parallel()
	paragraphs := []DocumentParagraph{
		{Text: "😀 foo foo", StartIndex: 10}, //nolint:dupword // Repetition is the match fixture.
		{Text: "foo foo", StartIndex: 30},   //nolint:dupword // Repetition is the match fixture.
	}
	actions, err := PlanParagraphMatches(paragraphs, Expression{
		Pattern:     "foo",
		Replacement: "$$${0}",
		NthMatch:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := matchStarts(actions); !reflect.DeepEqual(got, []int64{17, 34}) {
		t.Fatalf("starts = %v", got)
	}

	for _, action := range actions {
		if action.Replacement.ExpandedText != "$foo" {
			t.Fatalf("replacement = %#v", action.Replacement)
		}
	}
}

func TestPlanMatchesInvalidPattern(t *testing.T) {
	t.Parallel()
	if _, err := PlanMatches(DocumentSegment{}, Expression{Pattern: "["}); err == nil {
		t.Fatal("expected compile error")
	}
}

func matchStarts(actions []MatchAction) []int64 {
	starts := make([]int64, len(actions))
	for index, action := range actions {
		starts[index] = action.StartIndex
	}
	return starts
}
