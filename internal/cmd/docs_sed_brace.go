// Package cmd provides CLI commands for Google Docs operations.
package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

// indentNotSet is the sentinel value for braceExpr.Indent meaning "not specified".
// Any non-negative value means the indent level was explicitly set.
const indentNotSet = -1

// braceExpr represents a fully parsed brace expression from SEDMAT syntax.
// It captures all formatting, structural, and semantic attributes specified
// within a {flags} block in a replacement string.
type braceExpr struct {
	// Boolean flags (nil = not set, *true = enabled, *false = negated)
	Bold      *bool // {b} = true, {!b} = false
	Italic    *bool
	Underline *bool
	Strike    *bool
	Code      *bool
	Sup       *bool
	Sub       *bool
	SmallCaps *bool

	// Boolean flags with inline scoping
	InlineSpans []inlineSpan // {b=Warning} → span with text + flags

	// Value flags
	Text    string  // t= (empty = not set, "$0" = default)
	Color   string  // c= (hex or named, resolved to hex)
	Bg      string  // z= (hex or named, resolved to hex)
	Font    string  // f=
	Size    float64 // s= (0 = not set)
	URL     string  // u=
	Heading string  // h= ("t","s","1"-"6","0", "" = not set)
	Leading float64 // l= (0 = not set)
	Align   string  // a= ("left","center","right","justify")
	Opacity int     // o= (0 = not set, 100 = default)
	Indent  int     // n= (-1 = not set)
	Kerning float64 // k=
	Width   int     // x=
	Height  int     // y=

	// Paragraph spacing
	SpacingAbove float64 // p= first value
	SpacingBelow float64 // p= second value (or same as above if single)
	SpacingSet   bool    // whether p= was specified

	// Effect
	Effect string // e=

	// Columns
	Cols int // cols= (0 = not set)

	// Special flags
	Reset    bool   // {0} — explicit full reset
	NoReset  bool   // {!0} — opt out of implicit reset (additive mode)
	Break    string // += ("" = horizontal rule when + present, "p","c","s")
	HasBreak bool   // whether + was present
	Comment  string // "=text
	Bookmark string // @=name

	// Checkbox (tri-state)
	Check *bool // nil = no checkbox, true = checked, false = unchecked

	// Table of contents
	TOC    int  // toc depth (0 = not set, -1 = unlimited)
	HasTOC bool // whether toc was specified

	// Image ref (pattern-side)
	ImgRef string // img=

	// Table ref (pattern-side)
	TableRef string // T= (raw value for further parsing)
}

// inlineSpan represents an inline text span with associated boolean flags.
// Used for inline scoping like {b=Warning} where "Warning" is bolded inline.
type inlineSpan struct {
	Text  string   // The text content of the span
	Flags []string // Which boolean flags apply: "b", "i", "^", etc.
}

// boolFlagMap maps short and long names to canonical short names.
var boolFlagMap = map[string]string{
	"b":         "b",
	"bold":      "b",
	"i":         "i",
	"italic":    "i",
	"_":         "_",
	"underline": "_",
	"-":         "-",
	"strike":    "-",
	"#":         "#",
	"code":      "#",
	"^":         "^",
	"sup":       "^",
	",":         ",",
	"sub":       ",",
	"w":         "w",
	"smallcaps": "w",
}

// parseBraceExpr parses the content inside a brace expression.
// Input is the content between { }, e.g. for `{b c=red t=hello}` the input is `b c=red t=hello`.
func parseBraceExpr(s string) (*braceExpr, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return &braceExpr{Indent: indentNotSet}, nil // -1 means not set
	}

	expr := &braceExpr{Indent: indentNotSet} // -1 means not set

	// Handle comment first — it consumes everything after "=
	if idx := strings.Index(s, `"=`); idx >= 0 {
		expr.Comment = s[idx+2:]
		s = strings.TrimSpace(s[:idx])
	}

	// Check for reset flag at start
	if strings.HasPrefix(s, "0") {
		expr.Reset = true
		s = strings.TrimPrefix(s, "0")
		s = strings.TrimSpace(s)
	}

	// Tokenize remaining content
	tokens := tokenizeBraceContent(s)

	for _, tok := range tokens {
		if err := parseBraceToken(tok, expr); err != nil {
			return nil, err
		}
	}

	return expr, nil
}

// tokenizeBraceContent splits brace content into tokens.
// Tokens are space-separated, but values can contain spaces if quoted.
func tokenizeBraceContent(s string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote:
			if c == quoteChar {
				inQuote = false
			}
			current.WriteByte(c)
		case c == '"' || c == '\'':
			inQuote = true
			quoteChar = c
			current.WriteByte(c)
		case c == ' ' || c == '\t':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// parseBraceToken parses a single token and updates the braceExpr.
func parseBraceToken(tok string, expr *braceExpr) error {
	// Handle break flag: + or +=value
	if tok == "+" || strings.HasPrefix(tok, "+=") {
		expr.HasBreak = true
		if strings.HasPrefix(tok, "+=") {
			expr.Break = tok[2:]
		}
		return nil
	}

	// Handle bookmark: @=name
	if strings.HasPrefix(tok, "@=") {
		expr.Bookmark = tok[2:]
		return nil
	}

	// Handle negation: !flag
	if strings.HasPrefix(tok, "!") {
		flagName := tok[1:]
		// {!0} = opt out of implicit reset (additive mode)
		if flagName == "0" {
			expr.NoReset = true
			return nil
		}
		if canon, ok := boolFlagMap[flagName]; ok {
			setBoolFlag(expr, canon, false)
			return nil
		}
		return fmt.Errorf("unknown negated flag: %s", flagName)
	}

	// Check for key=value
	eqIdx := strings.Index(tok, "=")
	if eqIdx >= 0 {
		key := tok[:eqIdx]
		val := tok[eqIdx+1:]
		return parseBraceKeyValue(key, val, expr)
	}

	// Bare flag (no =)
	return parseBareFlag(tok, expr)
}

// parseBraceKeyValue handles key=value tokens.
func parseBraceKeyValue(key, val string, expr *braceExpr) error {
	// Check if it's a boolean flag with inline scoping (e.g., b=Warning)
	if canon, ok := boolFlagMap[key]; ok {
		// This is inline scoping: {b=text} means bold just "text"
		expr.InlineSpans = append(expr.InlineSpans, inlineSpan{
			Text:  val,
			Flags: []string{canon},
		})
		return nil
	}

	// Value flags
	switch key {
	case "t", "text":
		expr.Text = val
	case "c", "color":
		expr.Color = resolveColor(val)
	case "z", "bg":
		expr.Bg = resolveColor(val)
	case "f", "font":
		expr.Font = val
	case "s", "size":
		if n, err := strconv.ParseFloat(val, 64); err == nil && n > 0 {
			expr.Size = n
		}
	case "u", "url":
		expr.URL = val
	case "h", "heading":
		expr.Heading = val
	case "l", "leading":
		if n, err := strconv.ParseFloat(val, 64); err == nil && n > 0 {
			expr.Leading = n
		}
	case "a", "align":
		expr.Align = strings.ToLower(val)
	case "o", "opacity":
		if n, err := strconv.Atoi(val); err == nil && n >= 0 && n <= 100 {
			expr.Opacity = n
		}
	case "n", "indent":
		if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			expr.Indent = n
		}
	case "k", "kerning":
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			expr.Kerning = n
		}
	case "x", "width":
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			expr.Width = n
		}
	case "y", "height":
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			expr.Height = n
		}
	case "p", "spacing":
		parseSpacing(val, expr)
	case "e", "effect":
		expr.Effect = val
	case "cols":
		if n, err := strconv.Atoi(val); err == nil && n >= 1 {
			expr.Cols = n
		}
	case "check":
		switch strings.ToLower(val) {
		case "y", sendAsYes, boolTrue, "1":
			t := true
			expr.Check = &t
		case "n", "no", boolFalse, "0":
			f := false
			expr.Check = &f
		}
	case "toc":
		expr.HasTOC = true
		if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			expr.TOC = n
		} else {
			expr.TOC = -1 // unlimited
		}
	case "img":
		expr.ImgRef = val
	case "T":
		expr.TableRef = val
	default:
		return fmt.Errorf("unknown key: %s", key)
	}
	return nil
}

// parseBareFlag handles bare flags without = (e.g., {b}, {check}, {toc}).
func parseBareFlag(tok string, expr *braceExpr) error {
	// Check boolean flags
	if canon, ok := boolFlagMap[tok]; ok {
		setBoolFlag(expr, canon, true)
		return nil
	}

	// Special bare flags
	switch tok {
	case "check":
		// Bare check = unchecked checkbox
		f := false
		expr.Check = &f
	case "toc":
		expr.HasTOC = true
		expr.TOC = -1 // unlimited
	// Bare value flags reset to defaults (per SEDMAT spec)
	case "t", "text":
		expr.Text = "$0"
	case "c", "color":
		expr.Color = "#000000" // black
	case "z", "bg":
		expr.Bg = "" // clear (no background)
	case "f", "font":
		expr.Font = "Arial"
	case "s", "size":
		expr.Size = 11
	case "h", "heading":
		expr.Heading = "1" // default to HEADING_1
	case "p", "spacing":
		expr.SpacingSet = true
		// Reset to defaults — SpacingAbove/Below stay 0
	case "cols":
		expr.Cols = 1
	default:
		return fmt.Errorf("unknown flag: %s", tok)
	}
	return nil
}

// parseSpacing parses the p= flag value: "12" (both) or "12,6" (above,below).
func parseSpacing(val string, expr *braceExpr) {
	expr.SpacingSet = true
	if idx := strings.Index(val, ","); idx >= 0 {
		if above, err := strconv.ParseFloat(val[:idx], 64); err == nil {
			expr.SpacingAbove = above
		}
		if below, err := strconv.ParseFloat(val[idx+1:], 64); err == nil {
			expr.SpacingBelow = below
		}
	} else {
		if v, err := strconv.ParseFloat(val, 64); err == nil {
			expr.SpacingAbove = v
			expr.SpacingBelow = v
		}
	}
}

// setBoolFlag sets a boolean flag on braceExpr to the given value.
func setBoolFlag(expr *braceExpr, canon string, val bool) {
	switch canon {
	case "b":
		expr.Bold = &val
	case "i":
		expr.Italic = &val
	case "_":
		expr.Underline = &val
	case "-":
		expr.Strike = &val
	case "#":
		expr.Code = &val
	case "^":
		expr.Sup = &val
	case ",":
		expr.Sub = &val
	case "w":
		expr.SmallCaps = &val
	}
}
