package docsmarkdown

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// TextStyle represents text formatting
type TextStyle struct {
	Bold          bool
	Italic        bool
	Strikethrough bool
	Code          bool
	Link          string
	Start         int64
	End           int64
}

// utf16Len returns the number of UTF-16 code units in a string
func utf16Len(s string) int64 {
	return int64(len(utf16.Encode([]rune(s))))
}

// ParseInlineFormatting parses inline markdown formatting within text
// Returns styles with indices relative to the stripped plain text (UTF-16 code units)
func ParseInlineFormatting(text string) ([]TextStyle, string) {
	stripped, styles := parseInlineSegment(text)
	return styles, stripped
}

func parseInlineSegment(text string) (string, []TextStyle) {
	var stripped strings.Builder
	var styles []TextStyle

	for i := 0; i < len(text); {
		if text[i] == '`' {
			if content, end, ok := parseInlineCodeSpan(text, i); ok {
				start := utf16Len(stripped.String())
				stripped.WriteString(content)
				styles = append(styles, TextStyle{Start: start, End: start + utf16Len(content), Code: true})
				i = end

				continue
			}
		}

		if text[i] == '[' {
			if label, url, end, ok := parseInlineLink(text[i:]); ok {
				labelText, labelStyles := parseInlineSegment(label)
				start := utf16Len(stripped.String())
				stripped.WriteString(labelText)
				styles = appendShiftedStyles(styles, labelStyles, start)
				styles = append(styles, TextStyle{Start: start, End: start + utf16Len(labelText), Link: url})
				i += end

				continue
			}
		}

		if marker, bold, italic, strikethrough, ok := inlineMarkerAt(text, i); ok {
			searchFrom := i + len(marker)
			if end := findClosingInlineMarker(text, searchFrom, marker); end >= 0 && end > searchFrom {
				content, nestedStyles := parseInlineSegment(text[searchFrom:end])
				start := utf16Len(stripped.String())
				stripped.WriteString(content)
				styles = append(styles, TextStyle{
					Start:         start,
					End:           start + utf16Len(content),
					Bold:          bold,
					Italic:        italic,
					Strikethrough: strikethrough,
				})
				styles = appendShiftedStyles(styles, nestedStyles, start)
				i = end + len(marker)

				continue
			}
		}

		char, size := nextRune(text[i:])
		stripped.WriteString(char)
		i += size
	}

	return stripped.String(), styles
}

func parseInlineCodeSpan(text string, i int) (content string, end int, ok bool) {
	runEnd := i
	for runEnd < len(text) && text[runEnd] == '`' {
		runEnd++
	}
	marker := text[i:runEnd]

	searchFrom := runEnd
	for {
		rel := strings.Index(text[searchFrom:], marker)
		if rel < 0 {
			return "", 0, false
		}
		closeStart := searchFrom + rel

		closeEnd := closeStart + len(marker)
		if closeEnd < len(text) && text[closeEnd] == '`' {
			searchFrom = closeEnd
			continue
		}

		content = text[runEnd:closeStart]
		if content == "" {
			return "", 0, false
		}

		return content, closeEnd, true
	}
}

func parseInlineLink(text string) (label string, url string, end int, ok bool) {
	labelEndRel := strings.IndexByte(text[1:], ']')
	if labelEndRel < 0 {
		return "", "", 0, false
	}

	labelEnd := 1 + labelEndRel
	if labelEnd+1 >= len(text) || text[labelEnd+1] != '(' {
		return "", "", 0, false
	}
	urlStart := labelEnd + len("](")

	urlEndRel := strings.IndexByte(text[urlStart:], ')')
	if urlEndRel < 0 {
		return "", "", 0, false
	}

	return text[1:labelEnd], text[urlStart : urlStart+urlEndRel], urlStart + urlEndRel + 1, true
}

func appendShiftedStyles(styles []TextStyle, nested []TextStyle, offset int64) []TextStyle {
	for _, style := range nested {
		style.Start += offset
		style.End += offset
		styles = append(styles, style)
	}

	return styles
}

func inlineMarkerAt(text string, i int) (marker string, bold bool, italic bool, strikethrough bool, ok bool) {
	for _, candidate := range []string{"***", "___", "**", "__", "~~", "*", "_"} {
		if !strings.HasPrefix(text[i:], candidate) {
			continue
		}

		if candidate[0] == '_' && !isUnderscoreOpeningDelimiter(text, i, len(candidate)) {
			return "", false, false, false, false
		}

		if candidate == "~~" {
			if i > 0 && text[i-1] == '~' {
				return "", false, false, false, false
			}

			if tildeRunLenAt(text, i) != len(candidate) {
				return "", false, false, false, false
			}

			return candidate, false, false, true, true
		}

		switch len(candidate) {
		case 3:
			return candidate, true, true, false, true
		case 2:
			return candidate, true, false, false, true
		default:
			return candidate, false, true, false, true
		}
	}

	return "", false, false, false, false
}

func tildeRunLenAt(text string, i int) int {
	runEnd := i
	for runEnd < len(text) && text[runEnd] == '~' {
		runEnd++
	}

	return runEnd - i
}

func findClosingInlineMarker(text string, searchFrom int, marker string) int {
	for i := searchFrom; i < len(text); {
		if text[i] == '`' {
			if _, end, ok := parseInlineCodeSpan(text, i); ok {
				i = end
				continue
			}
		}

		if text[i] == marker[0] {
			closeIdx, next, ok := closingInlineMarkerInRun(text, searchFrom, i, marker)
			if ok && (marker[0] != '_' || isUnderscoreClosingDelimiter(text, closeIdx, len(marker))) {
				return closeIdx
			}
			i = next

			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}

	return -1
}

func closingInlineMarkerInRun(text string, searchFrom int, i int, marker string) (closeIdx int, next int, ok bool) {
	runEnd := i
	for runEnd < len(text) && text[runEnd] == marker[0] {
		runEnd++
	}
	runLen := runEnd - i

	markerLen := len(marker)
	if runLen < markerLen {
		return 0, runEnd, false
	}

	if marker == "~~" {
		return i, runEnd, runLen == markerLen
	}

	if markerLen == 1 {
		if runLen == 1 {
			return i, runEnd, true
		}

		if isClosingInlineDelimiterRun(text, i) {
			if runLen%2 == 0 {
				if hasLaterSingleClosingMarker(text, runEnd, marker[0]) {
					return 0, runEnd, false
				}

				return i, runEnd, true
			}

			return runEnd - 1, runEnd, true
		}

		return 0, runEnd, false
	}

	if runLen == markerLen || runLen%(markerLen*2) == 0 {
		return i, runEnd, true
	}

	if markerLen == 2 && runLen == 3 && isClosingInlineDelimiterRun(text, i) {
		if hasUnclosedSingleMarker(text[searchFrom:i], marker[0]) {
			return runEnd - markerLen, runEnd, true
		}

		return i, runEnd, true
	}

	return 0, runEnd, false
}

func isClosingInlineDelimiterRun(text string, i int) bool {
	before, hasBefore := runeBefore(text, i)
	return hasBefore && !unicode.IsSpace(before)
}

func hasUnclosedSingleMarker(text string, marker byte) bool {
	open := false

	for i := 0; i < len(text); {
		if text[i] == '`' {
			if _, end, ok := parseInlineCodeSpan(text, i); ok {
				i = end
				continue
			}
		}

		if text[i] != marker {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size

			continue
		}

		runEnd := i
		for runEnd < len(text) && text[runEnd] == marker {
			runEnd++
		}

		if runEnd-i == 1 {
			open = !open
		}
		i = runEnd
	}

	return open
}

func hasLaterSingleClosingMarker(text string, from int, marker byte) bool {
	for i := from; i < len(text); {
		if text[i] == '`' {
			if _, end, ok := parseInlineCodeSpan(text, i); ok {
				i = end
				continue
			}
		}

		if text[i] != marker {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size

			continue
		}

		runEnd := i
		for runEnd < len(text) && text[runEnd] == marker {
			runEnd++
		}

		runLen := runEnd - i
		if isClosingInlineDelimiterRun(text, i) && runLen%2 == 1 {
			return true
		}
		i = runEnd
	}

	return false
}

func isUnderscoreOpeningDelimiter(text string, i int, size int) bool {
	before, hasBefore := runeBefore(text, i)

	after, hasAfter := runeAfter(text, i+size)
	if !hasAfter || unicode.IsSpace(after) {
		return false
	}

	return !(hasBefore && isMarkdownWordRune(before) && isMarkdownWordRune(after))
}

func isUnderscoreClosingDelimiter(text string, i int, size int) bool {
	before, hasBefore := runeBefore(text, i)
	after, hasAfter := runeAfter(text, i+size)

	if !hasBefore || unicode.IsSpace(before) {
		return false
	}

	return !(hasAfter && isMarkdownWordRune(before) && isMarkdownWordRune(after))
}

func runeBefore(text string, i int) (rune, bool) {
	if i <= 0 {
		return 0, false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:i])

	return r, r != utf8.RuneError
}

func runeAfter(text string, i int) (rune, bool) {
	if i >= len(text) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(text[i:])

	return r, r != utf8.RuneError
}

func isMarkdownWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// nextRune returns the first rune and its byte size from a string.
// For a string consisting of a single multi-byte rune (e.g. Thai or other
// non-ASCII text), the previous range-based implementation returned size 0,
// which caused callers like ParseInlineFormatting to spin in an infinite loop.
func nextRune(s string) (string, int) {
	if s == "" {
		return "", 0
	}
	_, size := utf8.DecodeRuneInString(s)

	return s[:size], size
}
