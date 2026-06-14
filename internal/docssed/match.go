//nolint:wsl_v5 // Match-selection and range-planning stages stay adjacent.
package docssed

import (
	"fmt"
	"regexp"
	"unicode/utf16"
)

// ReplacementKind identifies the mutation represented by a matched replacement.
type ReplacementKind string

const (
	ReplacementText  ReplacementKind = "text"
	ReplacementImage ReplacementKind = "image"
)

// Replacement is the provider-independent interpretation of one expanded replacement.
type Replacement struct {
	Kind         ReplacementKind
	ExpandedText string
	Text         string
	Formats      []string
	Image        *ImageSpec
	Brace        *BraceExpression
	BraceSpans   []*BraceSpan
}

// MatchAction describes one indexed replacement in document order.
type MatchAction struct {
	StartIndex  int64
	EndIndex    int64
	OldText     string
	Replacement Replacement
}

// MatchPlanner owns a compiled expression for repeated planning.
type MatchPlanner struct {
	pattern    *regexp.Regexp
	expression Expression
}

// NewMatchPlanner validates and compiles one match expression.
func NewMatchPlanner(expression Expression) (*MatchPlanner, error) {
	pattern, err := regexp.Compile(expression.Pattern)
	if err != nil {
		return nil, fmt.Errorf("compile pattern: %w", err)
	}
	return &MatchPlanner{pattern: pattern, expression: expression}, nil
}

// PlanMatches finds matches independently within each projected Docs text run.
func PlanMatches(segment DocumentSegment, expression Expression) ([]MatchAction, error) {
	planner, err := NewMatchPlanner(expression)
	if err != nil {
		return nil, err
	}
	return planner.PlanSegment(segment), nil
}

// PlanSegment finds matches independently within each projected Docs text run.
func (p *MatchPlanner) PlanSegment(segment DocumentSegment) []MatchAction {
	actions := make([]MatchAction, 0, len(segment.TextRuns))
	for _, run := range segment.TextRuns {
		actions = append(actions, planTextMatches(
			run.Text,
			run.StartIndex,
			p.pattern,
			p.expression,
			matchSelectionPerRun,
		)...)
	}
	if p.expression.NthMatch <= 0 {
		return actions
	}
	if len(actions) < p.expression.NthMatch {
		return nil
	}
	return actions[p.expression.NthMatch-1 : p.expression.NthMatch]
}

// PlanParagraphMatches finds matches within each addressed paragraph.
func PlanParagraphMatches(paragraphs []DocumentParagraph, expression Expression) ([]MatchAction, error) {
	planner, err := NewMatchPlanner(expression)
	if err != nil {
		return nil, err
	}
	return planner.PlanParagraphs(paragraphs), nil
}

// PlanParagraphs finds matches independently within each addressed paragraph.
func (p *MatchPlanner) PlanParagraphs(paragraphs []DocumentParagraph) []MatchAction {
	actions := make([]MatchAction, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		actions = append(actions, planTextMatches(
			paragraph.Text,
			paragraph.StartIndex,
			p.pattern,
			p.expression,
			matchSelectionPerParagraph,
		)...)
	}
	return actions
}

type matchSelection int

const (
	matchSelectionPerRun matchSelection = iota
	matchSelectionPerParagraph
)

func planTextMatches(
	text string,
	baseIndex int64,
	pattern *regexp.Regexp,
	expression Expression,
	selection matchSelection,
) []MatchAction {
	if text == "" {
		return nil
	}

	limit := -1
	switch {
	case selection == matchSelectionPerRun && expression.NthMatch > 0:
		limit = -1
	case selection == matchSelectionPerParagraph && expression.NthMatch > 0:
		limit = expression.NthMatch
	case !expression.Global:
		limit = 1
	}
	locations := pattern.FindAllStringSubmatchIndex(text, limit)
	if selection == matchSelectionPerParagraph && expression.NthMatch > 0 {
		if len(locations) < expression.NthMatch {
			return nil
		}
		locations = locations[expression.NthMatch-1 : expression.NthMatch]
	}

	actions := make([]MatchAction, 0, len(locations))
	for _, location := range locations {
		oldText := text[location[0]:location[1]]
		expanded := string(pattern.ExpandString(nil, expression.Replacement, text, location))
		start, end := matchRange(baseIndex, text, location[0], location[1])
		actions = append(actions, MatchAction{
			StartIndex:  start,
			EndIndex:    end,
			OldText:     oldText,
			Replacement: classifyReplacement(expanded, expression),
		})
	}
	return actions
}

func matchRange(baseIndex int64, text string, startByte, endByte int) (int64, int64) {
	start := baseIndex + utf16Length(text[:startByte])
	return start, start + utf16Length(text[startByte:endByte])
}

func utf16Length(text string) int64 {
	return int64(len(utf16.Encode([]rune(text))))
}

func classifyReplacement(expanded string, expression Expression) Replacement {
	if image := ParseImageSyntax(expanded); image != nil {
		return Replacement{
			Kind:         ReplacementImage,
			ExpandedText: expanded,
			Image:        image,
		}
	}
	if expression.Brace != nil && expression.Brace.ImgRef != "" {
		return Replacement{
			Kind:         ReplacementImage,
			ExpandedText: expanded,
			Image: &ImageSpec{
				URL:    expression.Brace.ImgRef,
				Width:  expression.Brace.Width,
				Height: expression.Brace.Height,
			},
		}
	}
	if expression.Brace != nil {
		return Replacement{
			Kind:         ReplacementText,
			ExpandedText: expanded,
			Text:         expanded,
			Brace:        expression.Brace,
			BraceSpans:   expression.BraceSpans,
		}
	}

	markdown := ParseMarkdownReplacement(expanded)
	return Replacement{
		Kind:         ReplacementText,
		ExpandedText: expanded,
		Text:         markdown.Text,
		Formats:      markdown.Formats,
	}
}
