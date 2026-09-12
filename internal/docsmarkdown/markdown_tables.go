package docsmarkdown

import (
	"strings"
)

// IsTableSeparator checks if a line is a markdown table separator (|---|---|).
func IsTableSeparator(line string) bool {
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
		if lineIndex == 1 && IsTableSeparator(line) {
			continue
		}

		// Parse row: | cell1 | cell2 | cell3 |
		cells := ParseTableRow(line)
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

// ParseTableRow parses a single table row into cells.
func ParseTableRow(line string) []string {
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

// NormalizeTablesForDriveImport ensures Drive conversion receives non-empty table headers.
func NormalizeTablesForDriveImport(markdown string) string {
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

		if inFence || !isMarkdownTableCandidateLine(line) || i+2 >= len(lines) || !IsTableSeparator(lines[i+1]) || isIndentedMarkdownCodeLine(lines[i+1]) {
			out = append(out, line)
			continue
		}

		header := ParseTableRow(strings.TrimSpace(line))
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

// HasTableCellBreaks reports whether a Markdown table contains HTML line breaks.
func HasTableCellBreaks(markdown string) bool {
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

		if inFence || !isMarkdownTableCandidateLine(line) || i+1 >= len(lines) || !IsTableSeparator(lines[i+1]) {
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
