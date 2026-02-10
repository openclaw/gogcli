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
	Bold      bool
	Italic    bool
	Code      bool
	Link      string
	Start     int
	End       int
}

// ParagraphStyle represents paragraph-level formatting
type ParagraphStyle struct {
	Type      MarkdownElementType
	Start     int
	End       int
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

// ParseInlineFormatting parses inline markdown formatting within text
func ParseInlineFormatting(text string) ([]TextStyle, string) {
	var styles []TextStyle
	plainText := text
	offset := 0
	
	// Bold and italic (***text*** or **_text_**)
	biRegex := regexp.MustCompile(`\*\*\*([^*]+)\*\*\*|\*\*_([^_]+)_\*\*`)
	plainText = biRegex.ReplaceAllStringFunc(plainText, func(match string) string {
		extract := biRegex.FindStringSubmatch(match)
		content := extract[1]
		if extract[2] != "" {
			content = extract[2]
		}
		start := offset
		offset += len(content)
		styles = append(styles, TextStyle{
			Bold:   true,
			Italic: true,
			Start:  start,
			End:    offset,
		})
		return content
	})
	
	// Bold (**text**)
	boldRegex := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	plainText = boldRegex.ReplaceAllStringFunc(plainText, func(match string) string {
		extract := boldRegex.FindStringSubmatch(match)
		content := extract[1]
		start := offset
		offset += len(content)
		styles = append(styles, TextStyle{
			Bold:  true,
			Start: start,
			End:   offset,
		})
		return content
	})
	
	// Italic (*text* or _text_)
	italicRegex := regexp.MustCompile(`\*([^*]+)\*|_([^_]+)_`)
	plainText = italicRegex.ReplaceAllStringFunc(plainText, func(match string) string {
		extract := italicRegex.FindStringSubmatch(match)
		content := extract[1]
		if extract[2] != "" {
			content = extract[2]
		}
		start := offset
		offset += len(content)
		styles = append(styles, TextStyle{
			Italic: true,
			Start:  start,
			End:    offset,
		})
		return content
	})
	
	// Inline code (`code`)
	codeRegex := regexp.MustCompile("`([^`]+)`")
	plainText = codeRegex.ReplaceAllStringFunc(plainText, func(match string) string {
		extract := codeRegex.FindStringSubmatch(match)
		content := extract[1]
		start := offset
		offset += len(content)
		styles = append(styles, TextStyle{
			Code:  true,
			Start: start,
			End:   offset,
		})
		return content
	})
	
	// Links [text](url)
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	plainText = linkRegex.ReplaceAllStringFunc(plainText, func(match string) string {
		extract := linkRegex.FindStringSubmatch(match)
		text := extract[1]
		url := extract[2]
		start := offset
		offset += len(text)
		styles = append(styles, TextStyle{
			Link:  url,
			Start: start,
			End:   offset,
		})
		return text
	})
	
	return styles, plainText
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
