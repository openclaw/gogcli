package cmd

import (
	"regexp"
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
	Code  bool
	Link  string
	Start int
	End   int
}

// ParagraphStyle represents paragraph-level formatting
type ParagraphStyle struct {
	Type  MarkdownElementType
	Start int
	End   int
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
// Returns styles with indices relative to the stripped plain text
func ParseInlineFormatting(text string) ([]TextStyle, string) {
	var matches []InlineMatch

	// Links [text](url)
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

	// Inline code (`code`)
	codeRegex := regexp.MustCompile("`([^`]+)`")
	for _, idx := range codeRegex.FindAllStringSubmatchIndex(text, -1) {
		matches = append(matches, InlineMatch{
			Start:   idx[0],
			End:     idx[1],
			Content: text[idx[2]:idx[3]],
			Type:    "code",
		})
	}

	// Bold and italic (***text***)
	biRegex := regexp.MustCompile(`\*\*\*([^*]+)\*\*\*`)
	for _, idx := range biRegex.FindAllStringSubmatchIndex(text, -1) {
		matches = append(matches, InlineMatch{
			Start:   idx[0],
			End:     idx[1],
			Content: text[idx[2]:idx[3]],
			Type:    "bolditalic",
		})
	}

	// Bold (**text**)
	boldRegex := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	for _, idx := range boldRegex.FindAllStringSubmatchIndex(text, -1) {
		// Check if this overlaps with an existing match
		overlaps := false
		for _, m := range matches {
			if idx[0] >= m.Start && idx[1] <= m.End {
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

	// Italic (*text*)
	italicRegex := regexp.MustCompile(`\*([^*]+)\*`)
	for _, idx := range italicRegex.FindAllStringSubmatchIndex(text, -1) {
		// Check if this overlaps with an existing match
		overlaps := false
		for _, m := range matches {
			if idx[0] >= m.Start && idx[1] <= m.End {
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

	// Sort matches by start position
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[i].Start > matches[j].Start {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Build the stripped text and track position mapping
	var stripped strings.Builder
	positionMap := make(map[int]int)

	for i, ch := range text {
		insideMatch := false
		for _, m := range matches {
			if i >= m.Start && i < m.End {
				insideMatch = true
				// If this is the start of a match, add the content
				if i == m.Start {
					positionMap[i] = stripped.Len()
					stripped.WriteString(m.Content)
				}
				break
			}
		}

		if !insideMatch {
			positionMap[i] = stripped.Len()
			stripped.WriteRune(ch)
		}
	}
	positionMap[len(text)] = stripped.Len()

	strippedText := stripped.String()

	// Convert matches to styles with correct positions
	var styles []TextStyle
	for _, m := range matches {
		style := TextStyle{
			Start: positionMap[m.Start],
			End:   positionMap[m.End],
		}

		switch m.Type {
		case "bold":
			style.Bold = true
		case "italic":
			style.Italic = true
		case "bolditalic":
			style.Bold = true
			style.Italic = true
		case "code":
			style.Code = true
		case "link":
			style.Link = m.URL
		}

		styles = append(styles, style)
	}

	return styles, strippedText
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
