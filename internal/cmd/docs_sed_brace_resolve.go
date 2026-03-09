package cmd

import (
	"fmt"
	"strings"
	"unicode"
)

// namedColors maps color names to hex values per SEDMAT spec.
var namedColors = map[string]string{
	"black":     "#000000",
	"white":     "#FFFFFF",
	"red":       "#FF0000",
	"green":     "#00FF00",
	"blue":      "#0000FF",
	"yellow":    "#FFFF00",
	"cyan":      "#00FFFF",
	"magenta":   "#FF00FF",
	"orange":    "#FF8C00",
	"purple":    "#800080",
	"pink":      "#FF69B4",
	"brown":     "#8B4513",
	"gray":      "#808080",
	"grey":      "#808080",
	"lightgray": "#D3D3D3",
	"darkgray":  "#404040",
	"navy":      "#000080",
	"teal":      "#008080",
}

// resolveColor returns hex from a color name or passes through hex values.
// If the input is a named color, returns its hex equivalent.
// If already hex (#RRGGBB), returns as-is.
// Otherwise returns the input unchanged.
func resolveColor(s string) string {
	lower := strings.ToLower(s)
	if hex, ok := namedColors[lower]; ok {
		return hex
	}
	// Already hex or unknown — return as-is
	return s
}

// headingMap maps SEDMAT heading values to Google Docs named styles.
var headingMap = map[string]string{
	"t": "TITLE",
	"s": "SUBTITLE",
	"1": "HEADING_1",
	"2": "HEADING_2",
	"3": "HEADING_3",
	"4": "HEADING_4",
	"5": "HEADING_5",
	"6": "HEADING_6",
	"0": "NORMAL_TEXT",
}

// resolveHeading converts SEDMAT heading shorthand to Google Docs named style.
func resolveHeading(h string) string {
	if mapped, ok := headingMap[h]; ok {
		return mapped
	}
	// Check for numeric string
	if len(h) == 1 && h[0] >= '1' && h[0] <= '6' {
		return fmt.Sprintf("HEADING_%s", h)
	}
	return h
}

// alignMap maps SEDMAT alignment values to Google Docs alignment constants.
var alignMap = map[string]string{
	"left":    "START",
	"center":  "CENTER",
	"right":   "END",
	"justify": "JUSTIFIED",
}

// resolveAlign converts SEDMAT alignment shorthand to Google Docs alignment.
func resolveAlign(a string) string {
	if mapped, ok := alignMap[strings.ToLower(a)]; ok {
		return mapped
	}
	return a
}

// breakMap maps SEDMAT break values to descriptions.
var breakMap = map[string]string{
	"":  "horizontal_rule",
	"p": "page_break",
	"c": "column_break",
	"s": "section_break",
}

// resolveBreak converts SEDMAT break shorthand to a descriptive string.
func resolveBreak(b string) string {
	if mapped, ok := breakMap[b]; ok {
		return mapped
	}
	return b
}

// isHexColor returns true if s looks like a valid hex color (#RRGGBB or #RGB).
func isHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	s = s[1:]
	if len(s) != 3 && len(s) != 6 {
		return false
	}
	for _, c := range s {
		if !unicode.Is(unicode.ASCII_Hex_Digit, c) {
			return false
		}
	}
	return true
}

// normalizeHexColor converts #RGB to #RRGGBB format and uppercases.
func normalizeHexColor(s string) string {
	if !strings.HasPrefix(s, "#") {
		return s
	}
	hex := strings.ToUpper(s[1:])
	if len(hex) == 3 {
		// Expand #RGB to #RRGGBB
		return fmt.Sprintf("#%c%c%c%c%c%c", hex[0], hex[0], hex[1], hex[1], hex[2], hex[2])
	}
	return "#" + hex
}
