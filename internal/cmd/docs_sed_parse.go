package cmd

import (
	"fmt"

	"github.com/steipete/gogcli/internal/docssed"
)

func parseSedExpr(raw string) (pattern, replacement string, global bool, err error) {
	expression, err := docssed.ParseSubstitution(raw)
	if err != nil {
		return "", "", false, err
	}
	return expression.Pattern, expression.Replacement, expression.Global, nil
}

func parseSedExprWithCell(raw string) (pattern, replacement string, global bool, cellRef *tableCellRef, err error) {
	expression, err := docssed.ParseSubstitution(raw)
	if err != nil {
		return "", "", false, nil, err
	}
	pattern = expression.Pattern
	cellRef = parseTableCellRef(pattern)
	if cellRef != nil {
		if cellRef.subPattern != "" {
			pattern = cellRef.subPattern
		} else {
			pattern = ""
		}
	}
	return pattern, expression.Replacement, expression.Global, cellRef, nil
}

func parseMarkdownReplacement(replacement string) (text string, formats []string) {
	parsed := docssed.ParseMarkdownReplacement(replacement)
	return parsed.Text, parsed.Formats
}

func parseFullExpr(raw string) (sedExpr, error) {
	program, err := docssed.Parse(raw)
	if err != nil {
		return sedExpr{}, err
	}
	program, err = docssed.Enrich(program)
	if err != nil {
		return sedExpr{}, err
	}
	return sedExprFromSemantic(program.Expressions[0]), nil
}

func sedExprFromSemantic(expression docssed.Expression) sedExpr {
	converted := sedExprFromCore(expression)
	converted.cellRef = tableCellRefFromParsed(expression.Cell)
	braceTableToSedExpr(expression.Table, &converted)
	if expression.Image != nil {
		converted.pattern = imageReferencePattern(expression.Image)
	}
	converted.brace = expression.Brace
	converted.braceSpans = expression.BraceSpans
	if expression.TableCreate != nil {
		if expression.TableCreate.Header {
			converted.replacement = fmt.Sprintf(
				"|%dx%d:header|",
				expression.TableCreate.Rows,
				expression.TableCreate.Columns,
			)
		} else {
			converted.replacement = fmt.Sprintf(
				"|%dx%d|",
				expression.TableCreate.Rows,
				expression.TableCreate.Columns,
			)
		}
	}
	return converted
}

func parseAddress(raw string) (*sedAddress, string, error) {
	return docssed.ParseAddress(raw)
}

func parseNthFlag(raw string) int {
	return docssed.NthFlag(raw)
}

func parseDCommand(raw string) (sedExpr, error) {
	expression, err := docssed.ParseDelete(raw)
	return sedExprFromCore(expression), err
}

func parseAICommand(raw string, command byte) (sedExpr, error) {
	expression, err := docssed.ParseInsertAppend(raw, docssed.Command(command))
	return sedExprFromCore(expression), err
}

func parseYCommand(raw string) (sedExpr, error) {
	expression, err := docssed.ParseTransliterate(raw)
	return sedExprFromCore(expression), err
}

func sedExprFromCore(expression docssed.Expression) sedExpr {
	return sedExpr{
		pattern:     expression.Pattern,
		replacement: expression.Replacement,
		global:      expression.Global,
		nthMatch:    expression.NthMatch,
		command:     byte(expression.Command),
		addr:        expression.Address,
	}
}

func semanticExpressionFromSedExpr(expression sedExpr) docssed.Expression {
	return docssed.Expression{
		Pattern:     expression.pattern,
		Replacement: expression.replacement,
		Global:      expression.global,
		NthMatch:    expression.nthMatch,
		Command:     docssed.Command(expression.command),
		Address:     expression.addr,
		Brace:       expression.brace,
		BraceSpans:  expression.braceSpans,
	}
}
