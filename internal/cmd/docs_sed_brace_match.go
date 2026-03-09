package cmd

import (
	"strings"
)

// braceSpan represents a positioned brace expression within a replacement string.
// It tracks where in the output text the formatting should be applied.
type braceSpan struct {
	Expr      *braceExpr // The parsed brace expression
	Start     int        // Start position in the cleaned output text
	End       int        // End position in the cleaned output text
	IsGlobal  bool       // True if this applies to the whole match (e.g., {b} alone)
	RawBraces string     // Original {content} for debugging
}

// findBraceExprs finds all `{...}` groups in a replacement string and returns
// the cleaned text plus positioned spans. Handles:
//   - `{b}` as entire replacement → whole-match formatting
//   - `{b=text}` inline → inline span at position
//   - Multiple brace groups: `H{,=2}O` → "H2O" with subscript on "2"
//   - Escaped braces: `\{` and `\}` are literals
func findBraceExprs(replacement string) (string, []*braceSpan) {
	if !strings.Contains(replacement, "{") {
		return replacement, nil
	}

	var spans []*braceSpan
	var cleaned strings.Builder
	cleanedPos := 0

	i := 0
	for i < len(replacement) {
		// Handle escaped braces
		if i+1 < len(replacement) && replacement[i] == '\\' {
			if replacement[i+1] == '{' || replacement[i+1] == '}' {
				cleaned.WriteByte(replacement[i+1])
				cleanedPos++
				i += 2
				continue
			}
			// Other escape sequences
			cleaned.WriteByte(replacement[i])
			cleanedPos++
			i++
			continue
		}

		// Look for opening brace
		if replacement[i] == '{' {
			// Find matching closing brace (handle nesting for error detection)
			closeIdx := findMatchingBrace(replacement, i)
			if closeIdx < 0 {
				// Unmatched brace — treat as literal
				cleaned.WriteByte('{')
				cleanedPos++
				i++
				continue
			}

			braceContent := replacement[i+1 : closeIdx]
			rawBraces := replacement[i : closeIdx+1]

			expr, err := parseBraceExpr(braceContent)
			if err != nil {
				// Parse error — treat as literal
				cleaned.WriteByte('{')
				cleanedPos++
				i++
				continue
			}

			// Determine span behavior
			span := &braceSpan{
				Expr:      expr,
				Start:     cleanedPos,
				RawBraces: rawBraces,
			}

			// Check if this has inline spans with text
			if len(expr.InlineSpans) > 0 {
				// Handle inline scoping: write the span text
				for _, is := range expr.InlineSpans {
					spanStart := cleanedPos
					cleaned.WriteString(is.Text)
					cleanedPos += len(is.Text)
					span.End = cleanedPos

					// Create a span for each inline text
					inlineExpr := &braceExpr{Indent: indentNotSet}
					for _, flag := range is.Flags {
						setBoolFlag(inlineExpr, flag, true)
					}
					spans = append(spans, &braceSpan{
						Expr:      inlineExpr,
						Start:     spanStart,
						End:       cleanedPos,
						RawBraces: rawBraces,
					})
				}
				i = closeIdx + 1
				continue
			}

			// Check if Text is set explicitly
			if expr.Text != "" && expr.Text != "$0" {
				// Write the explicit text
				span.Start = cleanedPos
				cleaned.WriteString(expr.Text)
				cleanedPos += len(expr.Text)
				span.End = cleanedPos
				spans = append(spans, span)
				i = closeIdx + 1
				continue
			}

			// Check if this is a global expression (applies to whole match)
			// Global if:
			// 1. The brace is the entire replacement, or
			// 2. Text is set to $0 (explicit whole-match), or
			// 3. No inline spans and no explicit text (implicit whole-match)
			isGlobal := isGlobalBraceExpr(replacement, i, closeIdx)
			if isGlobal {
				span.IsGlobal = true
				span.End = -1 // Will be set by caller to match length
			}

			spans = append(spans, span)
			i = closeIdx + 1
			continue
		}

		// Regular character
		cleaned.WriteByte(replacement[i])
		cleanedPos++
		i++
	}

	return cleaned.String(), spans
}

// findMatchingBrace finds the index of the closing brace matching the one at pos.
// Returns -1 if no matching brace is found. Correctly handles escaped braces (\{ and \}).
func findMatchingBrace(s string, pos int) int {
	if pos >= len(s) || s[pos] != '{' {
		return -1
	}
	depth := 1
	for i := pos + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip next character (escaped)
			continue
		}
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// isGlobalBraceExpr determines if a brace expression should apply to the whole match.
// Returns true if:
//  1. The brace expression is the entire replacement (no surrounding text), OR
//  2. The brace is a trailing-only expression: text{flags} with nothing after.
//     In SEDMAT, `text{b c=red}` means "format all of 'text' with bold+red".
func isGlobalBraceExpr(replacement string, openIdx, closeIdx int) bool {
	before := strings.TrimSpace(replacement[:openIdx])
	after := strings.TrimSpace(replacement[closeIdx+1:])
	// Standalone brace (only thing in replacement) — global
	if before == "" && after == "" {
		return true
	}
	// Leading brace ({f=Georgia}text) — applies to all following text
	if before == "" && after != "" {
		return true
	}
	// Trailing brace (text{b}) — applies to all preceding text
	if after == "" && before != "" {
		return true
	}
	return false
}

// hasBraceFormatting returns true if the replacement contains brace formatting.
// Used to determine if fast-path native replacement can be used.
func hasBraceFormatting(replacement string) bool {
	// Look for { not preceded by \
	for i := 0; i < len(replacement); i++ {
		if replacement[i] == '{' {
			if i == 0 || replacement[i-1] != '\\' {
				// Check if there's content and a closing brace
				closeIdx := findMatchingBrace(replacement, i)
				if closeIdx > i+1 {
					// Has non-empty brace content
					content := replacement[i+1 : closeIdx]
					// Verify it looks like a brace expr (has known flags or key=value)
					if looksLikeBraceExpr(content) {
						return true
					}
				}
			}
		}
	}
	return false
}

// valueKeySet contains all value flag prefixes for fast brace expression detection.
// Package-level to avoid per-call allocation.
var valueKeySet = func() map[string]bool {
	keys := []string{
		"t=", "text=", "c=", "color=", "z=", "bg=", "f=", "font=",
		"s=", "size=", "u=", "url=", "h=", "heading=", "l=", "leading=",
		"a=", "align=", "o=", "opacity=", "n=", "indent=", "k=", "kerning=",
		"x=", "width=", "y=", "height=", "p=", "spacing=", "e=", "effect=",
		"cols=", "check", "toc", "img=", "T=", "@=", `"=`,
	}
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}()

// looksLikeBraceExpr returns true if the content looks like a valid brace expression.
// Used for heuristic detection to distinguish {formatting} from literal braces.
func looksLikeBraceExpr(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}

	// Check for known patterns
	// Reset
	if content == "0" || strings.HasPrefix(content, "0 ") {
		return true
	}
	// Boolean flags
	for flag := range boolFlagMap {
		if content == flag || strings.HasPrefix(content, flag+" ") || strings.HasPrefix(content, flag+"=") {
			return true
		}
		if content == "!"+flag || strings.HasPrefix(content, "!"+flag+" ") {
			return true
		}
	}
	// Value flags — check if content contains any known key prefix
	for key := range valueKeySet {
		if strings.Contains(content, key) {
			return true
		}
	}
	// Break flag
	if content == "+" || strings.HasPrefix(content, "+=") {
		return true
	}
	return false
}

// braceExprHasAnyFormat returns true if the expression sets any formatting.
// Used to filter out empty or no-op expressions.
func braceExprHasAnyFormat(expr *braceExpr) bool {
	if expr == nil {
		return false
	}
	// Check boolean flags
	if expr.Bold != nil || expr.Italic != nil || expr.Underline != nil ||
		expr.Strike != nil || expr.Code != nil || expr.Sup != nil ||
		expr.Sub != nil || expr.SmallCaps != nil {
		return true
	}
	// Check inline spans
	if len(expr.InlineSpans) > 0 {
		return true
	}
	// Check value flags (note: Indent -1 means not set, 0+ means explicitly set)
	if expr.Text != "" || expr.Color != "" || expr.Bg != "" || expr.Font != "" ||
		expr.Size > 0 || expr.URL != "" || expr.Heading != "" || expr.Leading > 0 ||
		expr.Align != "" || expr.Opacity > 0 || expr.Indent > indentNotSet || expr.Kerning != 0 ||
		expr.Width > 0 || expr.Height > 0 || expr.SpacingSet || expr.Effect != "" ||
		expr.Cols > 0 {
		return true
	}
	// Check special flags
	if expr.Reset || expr.HasBreak || expr.Comment != "" || expr.Bookmark != "" ||
		expr.Check != nil || expr.HasTOC || expr.ImgRef != "" || expr.TableRef != "" {
		return true
	}
	return false
}
