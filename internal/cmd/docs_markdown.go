package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf16"
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
)

// MarkdownElement represents a parsed markdown element
type MarkdownElement struct {
	Type     MarkdownElementType
	Content  string
	Children []MarkdownElement
	URL      string // for links
	Level    int    // for headings and lists
}

// TextStyle represents text formatting
type TextStyle struct {
	Bold   bool
	Italic bool
	Code   bool
	Link   string
	Start  int64
	End    int64
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
	var codeBlockContent strings.Builder

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End code block
				elements = append(elements, MarkdownElement{
					Type:    MDCodeBlock,
					Content: codeBlockContent.String(),
				})
				codeBlockContent.Reset()
				inCodeBlock = false
			} else {
				// Start code block
				inCodeBlock = true
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
			elements = append(elements, MarkdownElement{
				Type: MDEmptyLine,
			})
			continue
		}

		// Horizontal rule
		if isHorizontalRule(line) {
			elements = append(elements, MarkdownElement{
				Type: MDHorizontalRule,
			})
			continue
		}

		// Headings
		if headingLevel, content := parseHeading(line); headingLevel > 0 {
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
			})
			continue
		}

		// Blockquote
		if strings.HasPrefix(line, "> ") {
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

		// Numbered list
		if match := regexp.MustCompile(`^(\d+)\.\s+(.+)`).FindStringSubmatch(line); match != nil {
			elements = append(elements, MarkdownElement{
				Type:    MDNumberedList,
				Content: match[2],
			})
			continue
		}

		// Bullet list
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			content := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			elements = append(elements, MarkdownElement{
				Type:    MDListItem,
				Content: content,
			})
			continue
		}

		// Regular paragraph
		elements = append(elements, MarkdownElement{
			Type:    MDParagraph,
			Content: line,
		})
	}

	return elements
}

// InlineMatch represents a matched inline pattern
type InlineMatch struct {
	Start   int
	End     int
	Content string
	Type    string
	URL     string
}

// ParseInlineFormatting parses inline markdown formatting within text
// Returns styles with indices relative to the stripped plain text (UTF-16 code units)
func ParseInlineFormatting(text string) ([]TextStyle, string) {
	var matches []InlineMatch

	// Find all links [text](url)
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	for _, idx := range linkRegex.FindAllStringSubmatchIndex(text, -1) {
		matches = append(matches, InlineMatch{
			Start:   idx[0],
			End:     idx[1],
			Content: text[idx[2]:idx[3]],
			Type:    "link",
			URL:     text[idx[4]:idx[5]],
		})
	}

	// Find all inline code `code`
	codeRegex := regexp.MustCompile("`([^`]+)`")
	for _, idx := range codeRegex.FindAllStringSubmatchIndex(text, -1) {
		matches = append(matches, InlineMatch{
			Start:   idx[0],
			End:     idx[1],
			Content: text[idx[2]:idx[3]],
			Type:    "code",
		})
	}

	// Find bold-italic ***text***
	biRegex := regexp.MustCompile(`\*\*\*([^*]+)\*\*\*`)
	for _, idx := range biRegex.FindAllStringSubmatchIndex(text, -1) {
		matches = append(matches, InlineMatch{
			Start:   idx[0],
			End:     idx[1],
			Content: text[idx[2]:idx[3]],
			Type:    "bolditalic",
		})
	}

	// Find bold **text** (not overlapping with other patterns)
	boldRegex := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	for _, idx := range boldRegex.FindAllStringSubmatchIndex(text, -1) {
		overlaps := false
		for _, m := range matches {
			if idx[0] < m.End && idx[1] > m.Start {
				overlaps = true
				break
			}
		}
		if !overlaps {
			matches = append(matches, InlineMatch{
				Start:   idx[0],
				End:     idx[1],
				Content: text[idx[2]:idx[3]],
				Type:    "bold",
			})
		}
	}

	// For italic, we need to be careful not to match asterisks that are part of bold
	boldPositions := make(map[int]bool)
	for _, m := range matches {
		if m.Type == "bold" || m.Type == "bolditalic" {
			for i := m.Start; i <= m.End; i++ {
				boldPositions[i] = true
			}
		}
	}

	// Find italic *text* but skip positions that are part of bold markers
	italicRegex := regexp.MustCompile(`\*([^*]+)\*`)
	for _, idx := range italicRegex.FindAllStringSubmatchIndex(text, -1) {
		touchesBold := false
		for i := idx[0]; i <= idx[1]; i++ {
			if boldPositions[i] {
				touchesBold = true
				break
			}
		}
		if !touchesBold {
			overlaps := false
			for _, m := range matches {
				if idx[0] < m.End && idx[1] > m.Start {
					overlaps = true
					break
				}
			}
			if !overlaps {
				matches = append(matches, InlineMatch{
					Start:   idx[0],
					End:     idx[1],
					Content: text[idx[2]:idx[3]],
					Type:    "italic",
				})
			}
		}
	}

	// Sort matches by start position
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].Start > matches[j].Start {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Build stripped text and position map simultaneously
	var stripped strings.Builder
	// positionMap stores original byte offset -> stripped UTF-16 offset
	positionMap := make(map[int]int64)

	currentByte := 0
	var strippedUTF16Len int64 = 0

	for currentByte < len(text) {
		matchFound := false
		for _, m := range matches {
			if m.Start == currentByte {
				positionMap[currentByte] = strippedUTF16Len
				stripped.WriteString(m.Content)
				strippedUTF16Len += utf16Len(m.Content)
				currentByte = m.End
				matchFound = true
				break
			}
		}

		if !matchFound {
			positionMap[currentByte] = strippedUTF16Len
			char, size := nextRune(text[currentByte:])
			stripped.WriteString(char)
			strippedUTF16Len += utf16Len(char)
			currentByte += size
		}
	}

	positionMap[len(text)] = strippedUTF16Len
	strippedText := stripped.String()

	// Convert matches to styles with stripped UTF-16 positions
	var styles []TextStyle
	for _, m := range matches {
		styles = append(styles, TextStyle{
			Start:  positionMap[m.Start],
			End:    positionMap[m.End],
			Bold:   m.Type == "bold" || m.Type == "bolditalic",
			Italic: m.Type == "italic" || m.Type == "bolditalic",
			Code:   m.Type == "code",
			Link:   m.URL,
		})
	}

	return styles, strippedText
}

// nextRune returns the first rune and its byte size from a string
func nextRune(s string) (string, int) {
	for i, r := range s {
		if i > 0 {
			return s[:i], i
		}
		if len(s) == 1 {
			return s, 1
		}
		_ = r
	}
	return "", 0
}

func parseHeading(line string) (int, string) {
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	match := headingRegex.FindStringSubmatch(line)
	if match == nil {
		return 0, ""
	}
	return len(match[1]), match[2]
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
