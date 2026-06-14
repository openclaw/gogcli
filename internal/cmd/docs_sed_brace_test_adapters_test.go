package cmd

import "github.com/steipete/gogcli/internal/docssed"

var (
	parseBraceExpr        = docssed.ParseBraceExpression
	mergeBraceSpans       = docssed.MergeBraceSpans
	hasBraceFormatting    = docssed.HasBraceFormatting
	braceExprHasAnyFormat = docssed.BraceExpressionHasAnyFormat
)
