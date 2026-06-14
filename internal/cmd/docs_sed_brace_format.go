// Package cmd provides CLI commands for Google Docs operations.
package cmd

import (
	"strings"

	"google.golang.org/api/docs/v1"
)

// buildBraceTextStyleRequests converts a braceExpr to UpdateTextStyle requests.
// It handles boolean flags (bold, italic, etc.), value flags (color, font, size),
// negation, and reset.
func buildBraceTextStyleRequests(be *braceExpr, start, end int64) []*docs.Request {
	if be == nil {
		return nil
	}

	// Handle reset: explicit {0} OR implicit (default unless {!0}).
	// Every brace expression implicitly resets all formatting first,
	// making output deterministic regardless of inherited doc styles.
	// Use {!0} to opt into additive mode (preserve existing styles).
	if be.Reset || !be.NoReset {
		return buildResetTextStyleRequests(be, start, end)
	}

	style := &docs.TextStyle{}
	var fields []string

	// Boolean flags
	if be.Bold != nil {
		style.Bold = *be.Bold
		fields = append(fields, "bold")
	}
	if be.Italic != nil {
		style.Italic = *be.Italic
		fields = append(fields, "italic")
	}
	if be.Underline != nil {
		style.Underline = *be.Underline
		fields = append(fields, "underline")
	}
	if be.Strike != nil {
		style.Strikethrough = *be.Strike
		fields = append(fields, "strikethrough")
	}
	if be.SmallCaps != nil {
		style.SmallCaps = *be.SmallCaps
		fields = append(fields, "smallCaps")
	}
	if be.Sup != nil && *be.Sup {
		style.BaselineOffset = "SUPERSCRIPT"
		fields = append(fields, "baselineOffset")
	}
	if be.Sub != nil && *be.Sub {
		style.BaselineOffset = "SUBSCRIPT"
		fields = append(fields, "baselineOffset")
	}
	// Reset baseline if both sup and sub are explicitly false
	if be.Sup != nil && !*be.Sup && be.Sub != nil && !*be.Sub {
		style.BaselineOffset = "NONE"
		fields = append(fields, "baselineOffset")
	}

	// Code flag: monospace font + grey background
	if be.Code != nil && *be.Code {
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: "Courier New"}
		style.BackgroundColor = greyColor(codeBackgroundGrey)
		fields = append(fields, "weightedFontFamily", "backgroundColor")
	}

	// Value flags
	if be.Font != "" {
		style.WeightedFontFamily = &docs.WeightedFontFamily{FontFamily: be.Font}
		fields = append(fields, "weightedFontFamily")
	}
	if be.Size > 0 {
		style.FontSize = &docs.Dimension{Magnitude: be.Size, Unit: "PT"}
		fields = append(fields, "fontSize")
	}
	if be.Color != "" {
		if r, g, b, ok := parseHexColor(be.Color); ok {
			style.ForegroundColor = &docs.OptionalColor{
				Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}},
			}
			fields = append(fields, "foregroundColor")
		}
	}
	if be.Bg != "" {
		if r, g, b, ok := parseHexColor(be.Bg); ok {
			style.BackgroundColor = &docs.OptionalColor{
				Color: &docs.Color{RgbColor: &docs.RgbColor{Red: r, Green: g, Blue: b}},
			}
			fields = append(fields, "backgroundColor")
		}
	}
	if be.URL != "" {
		// Handle bookmark links (#name) vs regular URLs
		if strings.HasPrefix(be.URL, "#") {
			style.Link = &docs.Link{BookmarkId: be.URL[1:]}
		} else {
			style.Link = &docs.Link{Url: be.URL}
		}
		fields = append(fields, "link")
	}

	if len(fields) == 0 {
		return nil
	}

	return []*docs.Request{
		{
			UpdateTextStyle: &docs.UpdateTextStyleRequest{
				Range:     &docs.Range{StartIndex: start, EndIndex: end},
				TextStyle: style,
				Fields:    strings.Join(fields, ","),
			},
		},
	}
}

// buildResetTextStyleRequests builds requests to clear all formatting, then apply any
// additional flags specified after the reset (e.g., {0 b} = reset then bold).
func buildResetTextStyleRequests(be *braceExpr, start, end int64) []*docs.Request {
	requests := []*docs.Request{
		resetDocsTextStyleRequest(start, end, ""),
	}

	// Now apply any flags that were set alongside the reset
	// Create a copy without Reset flag and with NoReset to avoid recursion
	postReset := *be
	postReset.Reset = false
	postReset.NoReset = true

	if braceExprHasTextFormat(&postReset) {
		requests = append(requests, buildBraceTextStyleRequests(&postReset, start, end)...)
	}

	return requests
}

// braceExprHasTextFormat returns true if the braceExpr has any text-level formatting.
func braceExprHasTextFormat(be *braceExpr) bool {
	if be == nil {
		return false
	}
	return be.Bold != nil || be.Italic != nil || be.Underline != nil ||
		be.Strike != nil || be.Code != nil || be.Sup != nil ||
		be.Sub != nil || be.SmallCaps != nil || be.Color != "" ||
		be.Bg != "" || be.Font != "" || be.Size > 0 || be.URL != ""
}

// buildBraceParagraphStyleRequests converts a braceExpr to paragraph-level requests.
// Handles headings, alignment, indent, leading, spacing, bullets.
func buildBraceParagraphStyleRequests(be *braceExpr, start, end int64) []*docs.Request {
	if be == nil {
		return nil
	}

	var requests []*docs.Request

	// Build paragraph style
	paraStyle := &docs.ParagraphStyle{}
	var paraFields []string

	// Heading
	if be.Heading != "" {
		namedStyle := resolveHeading(be.Heading)
		paraStyle.NamedStyleType = namedStyle
		paraFields = append(paraFields, "namedStyleType")
	}

	// Alignment
	if be.Align != "" {
		paraStyle.Alignment = resolveAlign(be.Align)
		paraFields = append(paraFields, "alignment")
	}

	// Indent level (converted to indentStart in points)
	if be.Indent >= 0 {
		// Each indent level = 36pt (standard Google Docs indent)
		indentPt := float64(be.Indent) * indentPointsPerLevel
		paraStyle.IndentStart = &docs.Dimension{Magnitude: indentPt, Unit: "PT"}
		paraFields = append(paraFields, "indentStart")
	}

	// Line height / leading
	if be.Leading > 0 {
		paraStyle.LineSpacing = be.Leading * 100 // 1.5 → 150
		paraFields = append(paraFields, "lineSpacing")
	}

	// Paragraph spacing (above/below)
	if be.SpacingSet {
		if be.SpacingAbove > 0 || be.SpacingBelow > 0 {
			paraStyle.SpaceAbove = &docs.Dimension{Magnitude: be.SpacingAbove, Unit: "PT"}
			paraStyle.SpaceBelow = &docs.Dimension{Magnitude: be.SpacingBelow, Unit: "PT"}
			paraFields = append(paraFields, "spaceAbove", "spaceBelow")
		} else {
			// Reset spacing to zero
			paraStyle.SpaceAbove = &docs.Dimension{Magnitude: 0, Unit: "PT"}
			paraStyle.SpaceBelow = &docs.Dimension{Magnitude: 0, Unit: "PT"}
			paraFields = append(paraFields, "spaceAbove", "spaceBelow")
		}
	}

	if len(paraFields) > 0 {
		requests = append(requests, &docs.Request{
			UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
				Range:          &docs.Range{StartIndex: start, EndIndex: end},
				ParagraphStyle: paraStyle,
				Fields:         strings.Join(paraFields, ","),
			},
		})
	}

	return requests
}

// buildBraceInlineRequests handles inline scoping — multiple styled spans within one replacement.
// Each span has its own start/end positions and formatting.
func buildBraceInlineRequests(spans []*braceSpan, baseIndex int64) []*docs.Request {
	if len(spans) == 0 {
		return nil
	}

	var requests []*docs.Request
	for _, span := range spans {
		if span.IsGlobal || span.Expr == nil {
			continue // Global spans handled separately
		}

		start := baseIndex + int64(span.Start)
		end := baseIndex + int64(span.End)
		if end <= start {
			continue
		}

		requests = append(requests, buildBraceTextStyleRequests(span.Expr, start, end)...)
	}

	return requests
}

// buildBraceBreakRequests handles break flags (+, +=p, +=c, +=s).
// Returns requests to insert horizontal rules, page/column/section breaks.
func buildBraceBreakRequests(be *braceExpr, insertIdx int64) []*docs.Request {
	if be == nil || !be.HasBreak {
		return nil
	}

	var requests []*docs.Request

	switch be.Break {
	case "p": // Page break
		requests = append(requests, &docs.Request{
			InsertPageBreak: &docs.InsertPageBreakRequest{
				Location: &docs.Location{Index: insertIdx},
			},
		})
	case "c": // Column break
		// Column break is just a special character insert
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIdx},
				Text:     "\v", // Vertical tab = column break in Docs
			},
		})
	case "s": // Section break
		requests = append(requests, &docs.Request{
			InsertSectionBreak: &docs.InsertSectionBreakRequest{
				Location:    &docs.Location{Index: insertIdx},
				SectionType: "NEXT_PAGE",
			},
		})
	default: // "" = horizontal rule
		// Insert newline with bottom border
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Location: &docs.Location{Index: insertIdx},
				Text:     "\n",
			},
		})
		requests = append(requests, buildHruleBorderRequest(insertIdx, insertIdx+1))
	}

	return requests
}

// hasBraceTextFormat checks if braceExpr has formatting that requires text styling.
func hasBraceTextFormat(be *braceExpr) bool {
	return braceExprHasTextFormat(be)
}

// hasBraceParagraphFormat checks if braceExpr has formatting that requires paragraph styling.
func hasBraceParagraphFormat(be *braceExpr) bool {
	if be == nil {
		return false
	}
	return be.Heading != "" || be.Align != "" || be.Indent >= 0 ||
		be.Leading > 0 || be.SpacingSet
}

// (Legacy attrsTobraceExpr removed — sedAttrs no longer exists)
