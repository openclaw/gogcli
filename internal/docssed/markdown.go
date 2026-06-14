//nolint:wsl_v5 // Ordered markdown precedence is clearer as compact parsing branches.
package docssed

import (
	"fmt"
	"strings"
)

const (
	escapeAsterisk  = "\x00ESC_ASTERISK\x00"
	escapeHash      = "\x00ESC_HASH\x00"
	escapeTilde     = "\x00ESC_TILDE\x00"
	escapeBacktick  = "\x00ESC_BACKTICK\x00"
	escapeDash      = "\x00ESC_DASH\x00"
	escapePlus      = "\x00ESC_PLUS\x00"
	escapeBackslash = "\x00ESC_BACKSLASH\x00"
)

var (
	markdownEscaper = strings.NewReplacer(
		"\\\\", escapeBackslash,
		"\\*", escapeAsterisk,
		"\\#", escapeHash,
		"\\~", escapeTilde,
		"\\`", escapeBacktick,
		"\\-", escapeDash,
		"\\+", escapePlus,
		"\\n", "\n",
	)
	markdownUnescaper = strings.NewReplacer(
		escapeAsterisk, "*",
		escapeHash, "#",
		escapeTilde, "~",
		escapeBacktick, "`",
		escapeDash, "-",
		escapePlus, "+",
		escapeBackslash, "\\",
	)
	manualReplacementMarkers = []string{
		"**", "*", "~~", "`",
		"# ", "## ", "### ", "#### ", "##### ", "###### ",
		"- ", "+ ",
		"> ",
		"[^",
	}
)

// MarkdownReplacement is the provider-independent interpretation of replacement markdown.
type MarkdownReplacement struct {
	Text    string
	Formats []string
}

// ParseMarkdownReplacement extracts text and formatting from markdown-style replacement text.
func ParseMarkdownReplacement(replacement string) MarkdownReplacement {
	text := escapeMarkdown(replacement)

	formats := []string(nil)
	trimmed := strings.TrimSpace(text)
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return MarkdownReplacement{Text: "\n", Formats: []string{"hrule"}}
	}
	if strings.HasPrefix(text, "```") && strings.HasSuffix(text, "```") && len(text) > 6 {
		inner := text[3 : len(text)-3]
		if index := strings.Index(inner, "\n"); index >= 0 {
			inner = inner[index+1:]
		}
		return MarkdownReplacement{Text: unescapeMarkdown(inner), Formats: []string{"codeblock"}}
	}
	if strings.HasPrefix(text, "> ") {
		return MarkdownReplacement{Text: unescapeMarkdown(text[2:]), Formats: []string{"blockquote"}}
	}
	if strings.HasPrefix(text, "[^") && strings.HasSuffix(text, "]") && len(text) > 3 {
		return MarkdownReplacement{Text: unescapeMarkdown(text[2 : len(text)-1]), Formats: []string{"footnote"}}
	}

	indentLevel := 0
	listText := text
	for strings.HasPrefix(listText, "  ") {
		indentLevel++
		listText = listText[2:]
	}
	switch {
	case strings.HasPrefix(listText, "- "):
		text = listText[2:]
		formats = append(formats, "bullet")
	case strings.HasPrefix(listText, "* ") && !strings.HasSuffix(listText, "*"):
		text = listText[2:]
		formats = append(formats, "bullet")
	case len(listText) > 2 && listText[0] >= '0' && listText[0] <= '9' &&
		listText[1] == '.' && listText[2] == ' ':
		text = listText[3:]
		formats = append(formats, "numbered")
	}
	if len(formats) > 0 && indentLevel > 0 {
		text = strings.Repeat("\t", indentLevel) + text
	}

	switch {
	case strings.HasPrefix(text, "***") && strings.HasSuffix(text, "***") && len(text) > 6:
		return markdownResult(text[3:len(text)-3], append(formats, "bold", "italic"))
	case strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") && len(text) > 4:
		return markdownResult(text[2:len(text)-2], append(formats, "bold"))
	case strings.HasPrefix(text, "*") && strings.HasSuffix(text, "*") && len(text) > 2:
		return markdownResult(text[1:len(text)-1], append(formats, "italic"))
	case strings.HasPrefix(text, "~~") && strings.HasSuffix(text, "~~") && len(text) > 4:
		return markdownResult(text[2:len(text)-2], append(formats, "strikethrough"))
	case strings.HasPrefix(text, "`") && strings.HasSuffix(text, "`") && len(text) > 2:
		return markdownResult(text[1:len(text)-1], append(formats, "code"))
	}
	if index := strings.Index(text, "]("); index > 0 && strings.HasPrefix(text, "[") {
		closeParen := strings.LastIndex(text, ")")
		if closeParen > index+2 {
			linkText := text[1:index]
			linkURL := strings.ReplaceAll(text[index+2:closeParen], "\\/", "/")
			return markdownResult(linkText, append(formats, "link:"+linkURL))
		}
	}
	if strings.HasPrefix(text, "#") {
		level := 0
		for index := 0; index < len(text) && index < 6; index++ {
			if text[index] != '#' {
				break
			}
			level++
		}
		if level > 0 {
			stripped := strings.TrimPrefix(text[level:], " ")
			return markdownResult(stripped, append(formats, fmt.Sprintf("heading%d", level)))
		}
	}
	return markdownResult(text, formats)
}

func markdownResult(text string, formats []string) MarkdownReplacement {
	return MarkdownReplacement{Text: unescapeMarkdown(text), Formats: formats}
}

func escapeMarkdown(value string) string {
	return markdownEscaper.Replace(value)
}

func unescapeMarkdown(value string) string {
	return markdownUnescaper.Replace(value)
}

// CanUseNativeReplacement reports whether Docs native find/replace can preserve replacement semantics.
func CanUseNativeReplacement(replacement string) bool {
	if HasBraceFormatting(replacement) {
		return false
	}
	if strings.HasPrefix(replacement, "![") {
		return false
	}
	if strings.HasPrefix(replacement, "!(") && strings.HasSuffix(replacement, ")") {
		inner := replacement[2 : len(replacement)-1]
		if strings.HasPrefix(inner, "http://") || strings.HasPrefix(inner, "https://") {
			return false
		}
	}
	for _, marker := range manualReplacementMarkers {
		if strings.Contains(replacement, marker) {
			return false
		}
	}
	trimmed := strings.TrimSpace(replacement)
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return false
	}
	if len(replacement) >= 3 && replacement[0] >= '0' && replacement[0] <= '9' &&
		replacement[1] == '.' && replacement[2] == ' ' {
		return false
	}
	if strings.Contains(replacement, "\\n") {
		return false
	}
	for index := 0; index < len(replacement)-1; index++ {
		if replacement[index] != '$' {
			continue
		}
		next := replacement[index+1]
		if (next >= '0' && next <= '9') || next == '{' {
			return false
		}
	}
	return !strings.Contains(replacement, "](")
}
