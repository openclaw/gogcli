package docsmarkdown

import (
	"regexp"
	"strings"
)

func parseHeading(line string) (int, string) {
	prefix, content, ok := parseMarkdownATXHeadingLine(line)
	if !ok {
		return 0, ""
	}
	hashes := strings.TrimSpace(prefix)

	return len(hashes), content
}

var markdownHeadingAnchorRegex = regexp.MustCompile(`\s+\{#([^}\s]+)\}\s*$`)

// ExplicitHeadingAnchor identifies one source heading and its explicit anchor.
type ExplicitHeadingAnchor struct {
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

// StripHeadingAnchors removes explicit heading anchors outside code fences.
func StripHeadingAnchors(markdown string) string {
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

// ExplicitHeadingAnchors returns explicit anchors from parsed Markdown headings.
func ExplicitHeadingAnchors(markdown string) []ExplicitHeadingAnchor {
	elements := ParseMarkdown(markdown)
	anchors := make([]ExplicitHeadingAnchor, 0)
	seen := map[string]int{}

	for _, el := range elements {
		if !IsHeadingElement(el.Type) {
			continue
		}
		text := markdownHeadingSourceText(el.Content)
		seen[text]++

		anchor := strings.TrimSpace(el.Anchor)
		if anchor == "" {
			continue
		}
		anchors = append(anchors, ExplicitHeadingAnchor{
			Anchor:     anchor,
			Text:       text,
			Occurrence: seen[text],
		})
	}

	return anchors
}

// ImportExplicitHeadingAnchors returns anchors in Drive import heading order.
func ImportExplicitHeadingAnchors(markdown string) []ExplicitHeadingAnchor {
	headings := markdownImportHeadings(markdown)
	anchors := make([]ExplicitHeadingAnchor, 0)

	seen := map[string]int{}
	for _, heading := range headings {
		seen[heading.Text]++
		if heading.Anchor == "" {
			continue
		}
		anchors = append(anchors, ExplicitHeadingAnchor{
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

	return HeadingNormalizedText(text)
}

// HeadingNormalizedText collapses heading whitespace for stable matching.
func HeadingNormalizedText(text string) string {
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

// IsHeadingElement reports whether an element is a heading.
func IsHeadingElement(t MarkdownElementType) bool {
	return t >= MDHeading1 && t <= MDHeading6
}

// StripElementHeadingAnchors removes explicit anchors from parsed heading content.
func StripElementHeadingAnchors(elements []MarkdownElement) {
	for i := range elements {
		if IsHeadingElement(elements[i].Type) {
			if stripped, anchor := stripMarkdownHeadingAnchor(elements[i].Content); anchor != "" {
				elements[i].Content = stripped
				elements[i].Anchor = anchor
			}
		}
	}
}
