package cmd

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// MarkdownElementType represents the type of markdown element
type MarkdownElementType int

const (
	MDText MarkdownElementType = iota
	MDHeading1
	MDHeading2
	MDHeading3
	MDHeading4
	MDHeading5
	MDHeading6
	MDBold
	MDItalic
	MDBoldItalic
	MDCode
	MDCodeBlock
	MDLink
	MDImage
	MDListItem
	MDNumberedList
	MDBlockquote
	MDHorizontalRule
	MDParagraph
	MDEmptyLine
	MDTable
)

// MarkdownElement represents a parsed markdown element
type MarkdownElement struct {
	Type       MarkdownElementType
	Content    string
	Anchor     string // for headings: explicit Pandoc-style {#id}
	Children   []MarkdownElement
	URL        string     // for links
	Level      int        // for headings and lists
	TableCells [][]string // for tables: rows of cells
}

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

// ParagraphStyle represents paragraph-level formatting
type ParagraphStyle struct {
	Type  MarkdownElementType
	Start int64
	End   int64
}

// utf16Len returns the number of UTF-16 code units in a string
func utf16Len(s string) int64 {
	return int64(len(utf16.Encode([]rune(s))))
}

// ParseMarkdown parses markdown text into structured elements
func ParseMarkdown(text string) []MarkdownElement {
	var elements []MarkdownElement
	lines := strings.Split(text, "\n")

	inCodeBlock := false
	var codeFenceChar byte
	var codeFenceLen int
	var codeBlockContent strings.Builder
	var listIndents []int
	listActive := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Handle fenced code blocks.
		if fenceChar, fenceLen, ok := markdownCodeFence(line); ok {
			listIndents = nil
			listActive = false
			if inCodeBlock {
				if fenceChar != codeFenceChar || fenceLen < codeFenceLen {
					if codeBlockContent.Len() > 0 {
						codeBlockContent.WriteString("\n")
					}
					codeBlockContent.WriteString(line)
					continue
				}
				// End code block
				elements = append(elements, MarkdownElement{
					Type:    MDCodeBlock,
					Content: codeBlockContent.String(),
				})
				codeBlockContent.Reset()
				inCodeBlock = false
				codeFenceChar = 0
				codeFenceLen = 0
			} else {
				// Start code block
				inCodeBlock = true
				codeFenceChar = fenceChar
				codeFenceLen = fenceLen
			}
			continue
		}

		if inCodeBlock {
			if codeBlockContent.Len() > 0 {
				codeBlockContent.WriteString("\n")
			}
			codeBlockContent.WriteString(line)
			continue
		}

		// Empty line
		if strings.TrimSpace(line) == "" {
			listIndents = nil
			listActive = false
			if len(elements) > 0 && elements[len(elements)-1].Type != MDEmptyLine {
				elements = append(elements, MarkdownElement{Type: MDEmptyLine})
			}
			continue
		}

		// Horizontal rule
		if isHorizontalRule(line) {
			listIndents = nil
			listActive = false
			elements = append(elements, MarkdownElement{
				Type: MDHorizontalRule,
			})
			continue
		}

		// Headings
		if headingLevel, content := parseHeading(line); headingLevel > 0 {
			listIndents = nil
			listActive = false
			_, anchor := stripMarkdownHeadingAnchor(content)
			headingType := MDHeading1
			switch headingLevel {
			case 1:
				headingType = MDHeading1
			case 2:
				headingType = MDHeading2
			case 3:
				headingType = MDHeading3
			case 4:
				headingType = MDHeading4
			case 5:
				headingType = MDHeading5
			case 6:
				headingType = MDHeading6
			}
			elements = append(elements, MarkdownElement{
				Type:    headingType,
				Content: content,
				Anchor:  anchor,
				Level:   headingLevel,
			})
			continue
		}

		// Blockquote
		if strings.HasPrefix(line, "> ") {
			listIndents = nil
			listActive = false
			content := strings.TrimPrefix(line, "> ")
			if debugMarkdown {
				fmt.Printf("[PARSE] Blockquote detected: %q -> %q\n", line, content)
			}
			elements = append(elements, MarkdownElement{
				Type:    MDBlockquote,
				Content: content,
			})
			continue
		}

		// Lists, including tab-scoped markdown nesting. The Docs API derives
		// nesting from leading tabs after CreateParagraphBullets is applied.
		if listType, content, indent, ok := parseMarkdownListItem(line); ok && (indent == 0 || listActive) {
			if indent == 0 {
				listIndents = nil
			}
			elements = append(elements, MarkdownElement{
				Type:    listType,
				Content: content,
				Level:   markdownListLevel(indent, &listIndents),
			})
			listActive = true
			continue
		}

		// Table detection - line starts with | and has multiple |
		if strings.HasPrefix(line, "|") && strings.Count(line, "|") >= 2 {
			if debugMarkdown {
				fmt.Printf("[TABLE DEBUG] Found potential table row: %q\n", line)
				if i+1 < len(lines) {
					fmt.Printf("[TABLE DEBUG] Next line: %q, isSep: %v\n", lines[i+1], isTableSeparator(lines[i+1]))
				}
			}
			// Check if next line is separator (|---|---| pattern)
			if i+1 < len(lines) && isTableSeparator(lines[i+1]) {
				if debugMarkdown {
					fmt.Printf("[TABLE DEBUG] Parsing table starting at line %d\n", i)
				}
				// Parse table
				tableCells := parseMarkdownTable(lines[i:])
				elements = append(elements, MarkdownElement{
					Type:       MDTable,
					TableCells: tableCells,
				})
				listIndents = nil
				listActive = false
				// Skip all table lines
				i += countMarkdownTableLines(lines[i:]) - 1
				continue
			}
		}

		// Regular paragraph
		listIndents = nil
		listActive = false
		elements = append(elements, MarkdownElement{
			Type:    MDParagraph,
			Content: line,
		})
	}

	if inCodeBlock {
		elements = append(elements, MarkdownElement{
			Type:    MDCodeBlock,
			Content: codeBlockContent.String(),
		})
	}

	if len(elements) > 0 && elements[len(elements)-1].Type == MDEmptyLine {
		elements = elements[:len(elements)-1]
	}

	return elements
}

var markdownNumberedListRE = regexp.MustCompile(`^(\d+)\.\s+(.+)`)

func parseMarkdownListItem(line string) (MarkdownElementType, string, int, bool) {
	indent, rest := markdownListIndentColumns(line)
	if match := markdownNumberedListRE.FindStringSubmatch(rest); match != nil {
		return MDNumberedList, match[2], indent, true
	}
	if strings.HasPrefix(rest, "- ") || strings.HasPrefix(rest, "* ") {
		return MDListItem, rest[2:], indent, true
	}
	return MDText, "", 0, false
}

func markdownListIndentColumns(line string) (int, string) {
	column := 0
	i := 0
	for i < len(line) {
		switch line[i] {
		case ' ':
			column++
			i++
		case '\t':
			column += 4 - column%4
			i++
		default:
			return column, line[i:]
		}
	}
	return column, ""
}

func markdownListLevel(indent int, indents *[]int) int {
	if indent <= 0 {
		return 0
	}
	for i, seen := range *indents {
		if seen == indent {
			return i + 1
		}
	}
	*indents = append(*indents, indent)
	sort.Ints(*indents)
	for i, seen := range *indents {
		if seen == indent {
			return i + 1
		}
	}
	return len(*indents)
}

// isTableSeparator checks if a line is a markdown table separator (|---|---|)
func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	// Remove outer pipes
	inner := strings.Trim(trimmed, "|")
	// Split by | and check each segment
	segments := strings.Split(inner, "|")
	// A genuine separator must have at least one segment that actually contains
	// dashes. Without this guard a row of empty pipe cells (e.g. an empty
	// markdown table header `|     |     |`) would be misclassified as a
	// separator because every segment hits the `continue` and never trips the
	// dash check — see #609.
	sawDashSegment := false
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		// Each segment should be only dashes (with optional leading/trailing colon for alignment)
		for i, c := range seg {
			if c != '-' && c != ' ' && c != ':' {
				return false
			}
			// Colon only allowed at start or end for alignment
			if c == ':' && i != 0 && i != len(seg)-1 {
				return false
			}
		}
		// Must have at least one dash
		if strings.Count(seg, "-") == 0 {
			return false
		}
		sawDashSegment = true
	}
	return sawDashSegment
}

// parseMarkdownTable parses a markdown table into rows of cells
func parseMarkdownTable(lines []string) [][]string {
	var rows [][]string

	for lineIndex, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		// The parser only calls this after recognizing the second line as the
		// table delimiter. Separator-shaped rows later in the table are data.
		if lineIndex == 1 && isTableSeparator(line) {
			continue
		}

		// Parse row: | cell1 | cell2 | cell3 |
		cells := parseTableRow(line)
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}

	if len(rows) > 1 && isEmptyMarkdownTableRow(rows[0]) {
		rows = rows[1:]
	}
	return rows
}

func countMarkdownTableLines(lines []string) int {
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "|") {
			break
		}
		count++
	}
	return count
}

// parseTableRow parses a single table row into cells
func parseTableRow(line string) []string {
	// Remove outer pipes
	trimmed := strings.Trim(line, "|")

	// Split by |
	parts := strings.Split(trimmed, "|")

	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		cell = normalizeMarkdownTableBreaks(cell)
		cells = append(cells, cell)
	}

	return cells
}

func normalizeMarkdownTableBreaks(cell string) string {
	var out strings.Builder
	changed := false
	for i := 0; i < len(cell); {
		if cell[i] == '\\' && i+1 < len(cell) {
			out.WriteString(cell[i : i+2])
			i += 2
			continue
		}
		if cell[i] == '`' {
			if _, end, ok := parseInlineCodeSpan(cell, i); ok {
				out.WriteString(cell[i:end])
				i = end
				continue
			}
		}
		if breakLen := markdownTableBreakPrefixLen(cell[i:]); breakLen > 0 {
			out.WriteByte('\n')
			i += breakLen
			changed = true
			continue
		}
		out.WriteByte(cell[i])
		i++
	}
	if !changed {
		return cell
	}
	return out.String()
}

func markdownTableBreakPrefixLen(text string) int {
	if len(text) < len("<br>") || text[0] != '<' ||
		(text[1] != 'b' && text[1] != 'B') ||
		(text[2] != 'r' && text[2] != 'R') {
		return 0
	}
	i := 3
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	if i < len(text) && text[i] == '/' {
		i++
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	}
	if i >= len(text) || text[i] != '>' {
		return 0
	}
	return i + 1
}

func isEmptyMarkdownTableRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func normalizeMarkdownTablesForDriveImport(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	fenceMarker := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if marker := docsMarkdownFenceMarker(line); marker != "" {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			out = append(out, line)
			continue
		}
		if inFence || !isMarkdownTableCandidateLine(line) || i+2 >= len(lines) || !isTableSeparator(lines[i+1]) || isIndentedMarkdownCodeLine(lines[i+1]) {
			out = append(out, line)
			continue
		}
		header := parseTableRow(strings.TrimSpace(line))
		if !isEmptyMarkdownTableRow(header) || !isMarkdownTableCandidateLine(lines[i+2]) {
			out = append(out, line)
			continue
		}
		out = append(out, lines[i+2], lines[i+1])
		i += 2
		for i+1 < len(lines) {
			next := lines[i+1]
			if strings.TrimSpace(next) == "" || !strings.HasPrefix(strings.TrimSpace(next), "|") {
				break
			}
			out = append(out, next)
			i++
		}
	}
	return strings.Join(out, "\n")
}

func docsMarkdownFenceMarker(line string) string {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "```"):
		return "```"
	case strings.HasPrefix(trimmed, "~~~"):
		return "~~~"
	default:
		return ""
	}
}

func isMarkdownTableCandidateLine(line string) bool {
	return !isIndentedMarkdownCodeLine(line) && strings.HasPrefix(strings.TrimSpace(line), "|")
}

func isIndentedMarkdownCodeLine(line string) bool {
	return strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")
}

func markdownHasTableCellBreaks(markdown string) bool {
	lines := strings.Split(markdown, "\n")
	inFence := false
	fenceMarker := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if marker := docsMarkdownFenceMarker(line); marker != "" {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if inFence || !isMarkdownTableCandidateLine(line) || i+1 >= len(lines) || !isTableSeparator(lines[i+1]) {
			continue
		}
		if normalizeMarkdownTableBreaks(line) != line {
			return true
		}
		for j := i + 2; j < len(lines); j++ {
			row := lines[j]
			if strings.TrimSpace(row) == "" || !isMarkdownTableCandidateLine(row) {
				break
			}
			if normalizeMarkdownTableBreaks(row) != row {
				return true
			}
		}
	}
	return false
}

const inlineTypeCode = "code"

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

func parseHeading(line string) (int, string) {
	prefix, content, ok := parseMarkdownATXHeadingLine(line)
	if !ok {
		return 0, ""
	}
	hashes := strings.TrimSpace(prefix)
	return len(hashes), content
}

var markdownHeadingAnchorRegex = regexp.MustCompile(`\s+\{#([^}\s]+)\}\s*$`)

type markdownExplicitHeadingAnchor struct {
	Anchor     string
	Text       string
	Occurrence int
}

type markdownSourceHeading struct {
	Text   string
	Anchor string
}

func stripMarkdownHeadingAnchor(content string) (string, string) {
	match := markdownHeadingAnchorRegex.FindStringSubmatchIndex(content)
	if match == nil {
		return content, ""
	}
	anchor := content[match[2]:match[3]]
	return strings.TrimSpace(content[:match[0]]), anchor
}

func stripMarkdownHeadingAnchors(markdown string) string {
	lines := strings.SplitAfter(markdown, "\n")
	inCodeBlock := false
	var codeFenceChar byte
	var codeFenceLen int
	pendingSetextLine := -1
	for i, line := range lines {
		body, lineEnding := splitMarkdownLineEnding(line)
		if fenceChar, fenceLen, ok := markdownCodeFence(body); ok {
			pendingSetextLine = -1
			if inCodeBlock {
				if fenceChar == codeFenceChar && fenceLen >= codeFenceLen {
					inCodeBlock = false
					codeFenceChar = 0
					codeFenceLen = 0
				}
			} else {
				inCodeBlock = true
				codeFenceChar = fenceChar
				codeFenceLen = fenceLen
			}
			continue
		}
		if inCodeBlock {
			continue
		}

		if prefix, content, ok := parseMarkdownATXHeadingLine(body); ok {
			pendingSetextLine = -1
			stripped, anchor := stripMarkdownHeadingAnchor(content)
			if anchor == "" {
				continue
			}
			lines[i] = prefix + stripped + lineEnding
			continue
		}

		if isMarkdownSetextUnderline(body) {
			if pendingSetextLine >= 0 {
				prevBody, prevLineEnding := splitMarkdownLineEnding(lines[pendingSetextLine])
				stripped, anchor := stripMarkdownHeadingAnchor(prevBody)
				if anchor != "" {
					lines[pendingSetextLine] = stripped + prevLineEnding
				}
			}
			pendingSetextLine = -1
			continue
		}

		if !isMarkdownSetextHeadingCandidate(body) {
			pendingSetextLine = -1
			continue
		}
		pendingSetextLine = i
	}
	return strings.Join(lines, "")
}

func markdownExplicitHeadingAnchors(markdown string) []markdownExplicitHeadingAnchor {
	elements := ParseMarkdown(markdown)
	anchors := make([]markdownExplicitHeadingAnchor, 0)
	seen := map[string]int{}
	for _, el := range elements {
		if !isMarkdownHeadingElement(el.Type) {
			continue
		}
		text := markdownHeadingSourceText(el.Content)
		seen[text]++
		anchor := strings.TrimSpace(el.Anchor)
		if anchor == "" {
			continue
		}
		anchors = append(anchors, markdownExplicitHeadingAnchor{
			Anchor:     anchor,
			Text:       text,
			Occurrence: seen[text],
		})
	}
	return anchors
}

func markdownImportExplicitHeadingAnchors(markdown string) []markdownExplicitHeadingAnchor {
	headings := markdownImportHeadings(markdown)
	anchors := make([]markdownExplicitHeadingAnchor, 0)
	seen := map[string]int{}
	for _, heading := range headings {
		seen[heading.Text]++
		if heading.Anchor == "" {
			continue
		}
		anchors = append(anchors, markdownExplicitHeadingAnchor{
			Anchor:     heading.Anchor,
			Text:       heading.Text,
			Occurrence: seen[heading.Text],
		})
	}
	return anchors
}

func markdownImportHeadings(markdown string) []markdownSourceHeading {
	var headings []markdownSourceHeading
	lines := strings.Split(markdown, "\n")
	inCodeBlock := false
	var codeFenceChar byte
	var codeFenceLen int
	pendingSetextLine := ""
	pendingSetext := false
	for _, line := range lines {
		body := strings.TrimSuffix(line, "\r")
		if fenceChar, fenceLen, ok := markdownCodeFence(body); ok {
			pendingSetext = false
			if inCodeBlock {
				if fenceChar == codeFenceChar && fenceLen >= codeFenceLen {
					inCodeBlock = false
					codeFenceChar = 0
					codeFenceLen = 0
				}
			} else {
				inCodeBlock = true
				codeFenceChar = fenceChar
				codeFenceLen = fenceLen
			}
			continue
		}
		if inCodeBlock {
			continue
		}
		if _, content, ok := parseMarkdownATXHeadingLine(body); ok {
			headings = append(headings, markdownSourceHeadingFromContent(content))
			pendingSetext = false
			continue
		}
		if isMarkdownSetextUnderline(body) {
			if pendingSetext {
				headings = append(headings, markdownSourceHeadingFromContent(pendingSetextLine))
			}
			pendingSetext = false
			continue
		}
		if !isMarkdownSetextHeadingCandidate(body) {
			pendingSetext = false
			continue
		}
		pendingSetextLine = body
		pendingSetext = true
	}
	return headings
}

func markdownSourceHeadingFromContent(content string) markdownSourceHeading {
	stripped, anchor := stripMarkdownHeadingAnchor(content)
	return markdownSourceHeading{
		Text:   markdownHeadingSourceText(stripped),
		Anchor: strings.TrimSpace(anchor),
	}
}

func markdownHeadingSourceText(content string) string {
	stripped, _ := stripMarkdownHeadingAnchor(content)
	_, text := ParseInlineFormatting(stripped)
	return markdownHeadingNormalizedText(text)
}

func markdownHeadingNormalizedText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func splitMarkdownLineEnding(line string) (string, string) {
	body := line
	lineEnding := ""
	if strings.HasSuffix(body, "\n") {
		body = strings.TrimSuffix(body, "\n")
		lineEnding = "\n"
	}
	if strings.HasSuffix(body, "\r") {
		body = strings.TrimSuffix(body, "\r")
		lineEnding = "\r" + lineEnding
	}
	return body, lineEnding
}

func parseMarkdownATXHeadingLine(line string) (string, string, bool) {
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	if spaces > 3 || spaces >= len(line) || line[spaces] != '#' {
		return "", "", false
	}
	hashEnd := spaces
	for hashEnd < len(line) && line[hashEnd] == '#' {
		hashEnd++
	}
	if hashEnd-spaces > 6 || hashEnd >= len(line) {
		return "", "", false
	}
	if line[hashEnd] != ' ' && line[hashEnd] != '\t' {
		return "", "", false
	}
	contentStart := hashEnd
	for contentStart < len(line) && (line[contentStart] == ' ' || line[contentStart] == '\t') {
		contentStart++
	}
	if contentStart >= len(line) {
		return "", "", false
	}
	return line[:contentStart], line[contentStart:], true
}

func isMarkdownSetextUnderline(line string) bool {
	if !markdownLineAllowsSetextIndent(line) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	ch := trimmed[0]
	if ch != '=' && ch != '-' {
		return false
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] != ch {
			return false
		}
	}
	return true
}

func isMarkdownSetextHeadingCandidate(line string) bool {
	if !markdownLineAllowsSetextIndent(line) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "|") {
		return false
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
		return false
	}
	if markdownNumberedListRE.MatchString(trimmed) {
		return false
	}
	return true
}

func markdownLineAllowsSetextIndent(line string) bool {
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	if spaces > 3 {
		return false
	}
	return spaces >= len(line) || line[spaces] != '\t'
}

func isMarkdownHeadingElement(t MarkdownElementType) bool {
	return t >= MDHeading1 && t <= MDHeading6
}

func stripMarkdownElementHeadingAnchors(elements []MarkdownElement) {
	for i := range elements {
		if isMarkdownHeadingElement(elements[i].Type) {
			if stripped, anchor := stripMarkdownHeadingAnchor(elements[i].Content); anchor != "" {
				elements[i].Content = stripped
				elements[i].Anchor = anchor
			}
		}
		if len(elements[i].Children) > 0 {
			stripMarkdownElementHeadingAnchors(elements[i].Children)
		}
	}
}

func markdownCodeFence(line string) (byte, int, bool) {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 3 {
		i++
	}
	if i < len(line) && line[i] == ' ' {
		return 0, 0, false
	}
	if i >= len(line) {
		return 0, 0, false
	}
	ch := line[i]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	j := i
	for j < len(line) && line[j] == ch {
		j++
	}
	if j-i < 3 {
		return 0, 0, false
	}
	return ch, j - i, true
}

func isHorizontalRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	char := trimmed[0]
	if char != '-' && char != '*' && char != '_' {
		return false
	}
	for _, c := range trimmed {
		if c != rune(char) && c != ' ' {
			return false
		}
	}
	return true
}
