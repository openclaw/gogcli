package docsmarkdown

import (
	"regexp"
	"slices"
	"strings"
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
	MDCodeBlock
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
	Anchor     string     // for headings: explicit Pandoc-style {#id}
	Level      int        // for headings and lists
	TableCells [][]string // for tables: rows of cells
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
			// Check if next line is separator (|---|---| pattern)
			if i+1 < len(lines) && IsTableSeparator(lines[i+1]) {
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
	slices.Sort(*indents)

	for i, seen := range *indents {
		if seen == indent {
			return i + 1
		}
	}

	return len(*indents)
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
