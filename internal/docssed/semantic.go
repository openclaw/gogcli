//nolint:wsl_v5 // Semantic enrichment keeps ordered parsing stages adjacent.
package docssed

import (
	"fmt"
	"strings"
)

// Enrich resolves provider-independent table, cell, image, and brace semantics.
func Enrich(program Program) (Program, error) {
	for index := range program.Expressions {
		expression, err := enrichExpression(program.Expressions[index])
		if err != nil {
			return Program{}, err
		}
		program.Expressions[index] = expression
	}
	return program, nil
}

func enrichExpression(expression Expression) (Expression, error) {
	if expression.Command != CommandSubstitute {
		return expression, nil
	}

	expression.Cell = ParseTableCellReference(expression.Pattern)
	if expression.Cell != nil {
		expression.Pattern = expression.Cell.Subpattern
	}

	if expression.Cell == nil && strings.HasPrefix(expression.Pattern, "{") {
		remaining, table, image, err := DetectBraceReference(expression.Pattern)
		if err != nil {
			return Expression{}, fmt.Errorf("brace pattern: %w", err)
		}
		expression.Pattern = remaining
		expression.Table = table
		expression.Image = image
		if table != nil && table.IsCreate {
			expression.TableCreate = tableCreateSpec(table)
		}
	}

	if expression.Cell == nil && expression.Table == nil && expression.Image == nil {
		if table := ParseTableReference(expression.Pattern); table != nil {
			expression.Table = table
			expression.Pattern = ""
		}
	}

	if HasBraceFormatting(expression.Replacement) {
		cleaned, spans := ParseBraceReplacement(expression.Replacement)
		if len(spans) > 0 {
			expression.Replacement = cleaned
			expression.BraceSpans = spans
			if len(spans) == 1 && spans[0].IsGlobal {
				expression.Brace = spans[0].Expr
			} else {
				expression.Brace = MergeBraceSpans(spans)
			}
			if expression.Brace != nil && expression.Brace.TableRef != "" {
				table, err := ParseBraceTableReference(expression.Brace.TableRef)
				if err == nil && table.IsCreate {
					expression.TableCreate = tableCreateSpec(table)
					expression.Brace = nil
					expression.BraceSpans = nil
				}
			}
		}
	}
	return expression, nil
}

func tableCreateSpec(reference *TableReference) *TableCreateSpec {
	if reference == nil || !reference.IsCreate {
		return nil
	}
	return &TableCreateSpec{
		Rows:    reference.CreateRows,
		Columns: reference.CreateCols,
		Header:  reference.HasHeader,
	}
}

// MergeBraceSpans combines global formatting spans; inline spans remain positioned separately.
func MergeBraceSpans(spans []*BraceSpan) *BraceExpression {
	merged := &BraceExpression{Indent: IndentNotSet}
	for _, span := range spans {
		if span == nil || !span.IsGlobal || span.Expr == nil {
			continue
		}
		source := span.Expr
		if source.Bold != nil {
			merged.Bold = source.Bold
		}
		if source.Italic != nil {
			merged.Italic = source.Italic
		}
		if source.Underline != nil {
			merged.Underline = source.Underline
		}
		if source.Strike != nil {
			merged.Strike = source.Strike
		}
		if source.Code != nil {
			merged.Code = source.Code
		}
		if source.Sup != nil {
			merged.Sup = source.Sup
		}
		if source.Sub != nil {
			merged.Sub = source.Sub
		}
		if source.SmallCaps != nil {
			merged.SmallCaps = source.SmallCaps
		}
		if source.Color != "" {
			merged.Color = source.Color
		}
		if source.Bg != "" {
			merged.Bg = source.Bg
		}
		if source.Font != "" {
			merged.Font = source.Font
		}
		if source.Size > 0 {
			merged.Size = source.Size
		}
		if source.URL != "" {
			merged.URL = source.URL
		}
		if source.Heading != "" {
			merged.Heading = source.Heading
		}
		if source.Align != "" {
			merged.Align = source.Align
		}
		if source.Leading > 0 {
			merged.Leading = source.Leading
		}
		if source.SpacingSet {
			merged.SpacingSet = true
			merged.SpacingAbove = source.SpacingAbove
			merged.SpacingBelow = source.SpacingBelow
		}
		if source.Indent >= 0 {
			merged.Indent = source.Indent
		}
		if source.Reset {
			merged.Reset = true
		}
		if source.HasBreak {
			merged.HasBreak = true
			merged.Break = source.Break
		}
	}
	return merged
}
