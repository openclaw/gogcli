//nolint:wsl_v5 // Paragraph selection and mutation construction stay adjacent.
package docssed

import (
	"fmt"
	"regexp"
	"strings"
)

// ParagraphPlanner owns one compiled top-level paragraph match expression.
type ParagraphPlanner struct {
	pattern *regexp.Regexp
}

// NewParagraphPlanner validates one paragraph command expression.
func NewParagraphPlanner(expression Expression) (*ParagraphPlanner, error) {
	pattern, err := regexp.Compile(expression.Pattern)
	if err != nil {
		return nil, fmt.Errorf("compile pattern: %w", err)
	}
	return &ParagraphPlanner{pattern: pattern}, nil
}

// PlanDelete returns full-range deletions for matching top-level paragraphs.
func (p *ParagraphPlanner) PlanDelete(segment DocumentSegment) []AddressMutation {
	paragraphs := p.matchingTopLevelParagraphs(segment)
	mutations := make([]AddressMutation, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		start := paragraph.StartIndex
		if start < 1 {
			start = 1
		}
		mutations = append(mutations, AddressMutation{
			StartIndex: start,
			EndIndex:   paragraph.EndIndex,
		})
	}
	return mutations
}

// PlanInsert returns insert-before or append-after mutations for matching top-level paragraphs.
func (p *ParagraphPlanner) PlanInsert(
	segment DocumentSegment,
	replacement string,
	before bool,
) []AddressMutation {
	insertText := strings.ReplaceAll(replacement, "\\n", "\n")
	if !strings.HasSuffix(insertText, "\n") {
		insertText += "\n"
	}

	paragraphs := p.matchingTopLevelParagraphs(segment)
	mutations := make([]AddressMutation, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		index := paragraph.EndIndex
		if before {
			index = paragraph.StartIndex
		}
		mutations = append(mutations, AddressMutation{
			StartIndex: index,
			InsertText: insertText,
		})
	}
	return mutations
}

func (p *ParagraphPlanner) matchingTopLevelParagraphs(segment DocumentSegment) []DocumentParagraph {
	paragraphs := make([]DocumentParagraph, 0, len(segment.Blocks))
	for _, block := range segment.Blocks {
		if block.Kind != DocumentBlockParagraph ||
			block.ItemIndex < 0 ||
			block.ItemIndex >= len(segment.Paragraphs) {
			continue
		}
		paragraph := segment.Paragraphs[block.ItemIndex]
		if p.pattern.MatchString(strings.TrimRight(paragraph.Text, "\n")) {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	return paragraphs
}
