package cmd

import "github.com/openclaw/gogcli/internal/docssed"

var (
	parseBraceExpr        = docssed.ParseBraceExpression
	mergeBraceSpans       = docssed.MergeBraceSpans
	hasBraceFormatting    = docssed.HasBraceFormatting
	braceExprHasAnyFormat = docssed.BraceExpressionHasAnyFormat
)
