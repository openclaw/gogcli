package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/ui"
)

// runDryRun validates and classifies expressions without making API calls.
// It prints a summary of each expression's type, validity, and content.
// No authentication is required since no Google API calls are made.
func (c *DocsSedCmd) runDryRun(_ context.Context, u *ui.UI, exprs []sedExpr) error {
	for i, expr := range exprs {
		kind := classifyExpression(expr)

		valid := "ok"
		if expr.pattern != "" && expr.pattern != "^" && expr.pattern != "$" && expr.pattern != "^$" {
			if _, err := expr.compilePattern(); err != nil {
				valid = fmt.Sprintf("ERROR: %s", err)
			}
		}

		flag := ""
		if expr.global {
			flag = "g"
		}

		prefix := "s"
		if expr.command != 0 {
			prefix = string(expr.command)
		}
		nthStr := ""
		if expr.nthMatch > 0 {
			nthStr = fmt.Sprintf("%d", expr.nthMatch)
		}

		// Include brace flags in output if present
		braceInfo := ""
		if expr.brace != nil {
			braceInfo = formatBraceFlags(expr.brace)
		}

		if braceInfo != "" {
			u.Out().Printf("%d\t%s\t%s\t%s/%s/%s/%s%s\t%s", i+1, kind, valid, prefix, expr.pattern, truncateSed(expr.replacement, 40), flag, nthStr, braceInfo)
		} else {
			u.Out().Printf("%d\t%s\t%s\t%s/%s/%s/%s%s", i+1, kind, valid, prefix, expr.pattern, truncateSed(expr.replacement, 40), flag, nthStr)
		}
	}

	u.Out().Printf("---")
	u.Out().Printf("dry-run: %d expressions parsed, no changes made", len(exprs))
	return nil
}

// classifyExpression determines the type of a sed expression for dry-run display.
func classifyExpression(expr sedExpr) string {
	switch expr.command {
	case 'd':
		return "delete"
	case 'a':
		return "append-after"
	case 'i':
		return "insert-before"
	case 'y':
		return "transliterate"
	}
	if expr.cellRef != nil {
		kind := fmt.Sprintf("cell |%d|[%d,%d]", expr.cellRef.tableIndex, expr.cellRef.row, expr.cellRef.col)
		if expr.cellRef.row == 0 || expr.cellRef.col == 0 {
			kind += " (wildcard)"
		}
		return kind
	}
	if expr.tableRef != 0 {
		if expr.replacement == "" {
			return fmt.Sprintf("delete table %d", expr.tableRef)
		}
		return fmt.Sprintf("table %d op", expr.tableRef)
	}
	if parseTableCreate(expr.replacement) != nil || parseTableFromPipes(expr.replacement) != nil {
		return "create table"
	}
	if parseImageRefPattern(expr.pattern) != nil {
		return "image"
	}
	if expr.pattern == "^" || expr.pattern == "$" || expr.pattern == "^$" {
		return "positional"
	}
	// Check for brace formatting
	if expr.brace != nil && braceExprHasAnyFormat(expr.brace) {
		return "brace"
	}
	if canUseNativeReplace(expr.replacement) && expr.global && expr.brace == nil {
		return "native"
	}
	return "manual"
}

// formatBraceFlags returns a compact string representation of brace flags for display.
func formatBraceFlags(be *braceExpr) string {
	if be == nil {
		return ""
	}

	var parts []string

	// Reset
	if be.Reset {
		parts = append(parts, "0")
	}

	// Boolean flags
	if be.Bold != nil {
		if *be.Bold {
			parts = append(parts, "b")
		} else {
			parts = append(parts, "!b")
		}
	}
	if be.Italic != nil {
		if *be.Italic {
			parts = append(parts, "i")
		} else {
			parts = append(parts, "!i")
		}
	}
	if be.Underline != nil {
		if *be.Underline {
			parts = append(parts, "_")
		} else {
			parts = append(parts, "!_")
		}
	}
	if be.Strike != nil {
		if *be.Strike {
			parts = append(parts, "-")
		} else {
			parts = append(parts, "!-")
		}
	}
	if be.Code != nil && *be.Code {
		parts = append(parts, "#")
	}
	if be.Sup != nil && *be.Sup {
		parts = append(parts, "^")
	}
	if be.Sub != nil && *be.Sub {
		parts = append(parts, ",")
	}
	if be.SmallCaps != nil && *be.SmallCaps {
		parts = append(parts, "w")
	}

	// Value flags
	if be.Color != "" {
		parts = append(parts, "c="+be.Color)
	}
	if be.Bg != "" {
		parts = append(parts, "z="+be.Bg)
	}
	if be.Font != "" {
		parts = append(parts, "f="+be.Font)
	}
	if be.Size > 0 {
		parts = append(parts, fmt.Sprintf("s=%.0f", be.Size))
	}
	if be.URL != "" {
		parts = append(parts, "u="+truncateSed(be.URL, 20))
	}
	if be.Heading != "" {
		parts = append(parts, "h="+be.Heading)
	}
	if be.Align != "" {
		parts = append(parts, "a="+be.Align)
	}
	if be.HasBreak {
		if be.Break == "" {
			parts = append(parts, "+")
		} else {
			parts = append(parts, "+="+be.Break)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// truncateSed shortens a string to max characters, appending "..." if truncated.
func truncateSed(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
