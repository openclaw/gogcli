package slidesmarkdown

import (
	"regexp"
	"strings"
)

// faShortcodeRE matches :fa-name:, :fas-name:, :far-name:, :fab-name:,
// :fal-name:, :fad-name:.
var faShortcodeRE = regexp.MustCompile(`:fa([srlbd])?-([a-z0-9][a-z0-9-]*):`)

// emphasisRE matches **bold**, __bold__, _italic_, *italic*, `code`.
// Greedy, non-nested. We process emphasis on text spans between FA shortcodes.
var emphasisRE = regexp.MustCompile(
	"(\\*\\*[^*\\n]+\\*\\*)|(__[^_\\n]+__)|(\\*[^*\\n]+\\*)|(_[^_\\n]+_)|(`[^`\\n]+`)",
)

// parseInlines tokenizes a single line of markdown text into Inline runs.
// FA shortcodes are extracted first (so emphasis processing doesn't see
// the colons inside them), then emphasis is applied to the remaining text.
func parseInlines(text string, defaultFAStyle string) []Inline {
	var out []Inline

	idxs := faShortcodeRE.FindAllStringSubmatchIndex(text, -1)
	cursor := 0

	for _, m := range idxs {
		// Append text before the icon.
		if m[0] > cursor {
			out = append(out, parseEmphasis(text[cursor:m[0]])...)
		}

		stylePrefix := ""
		if m[2] != -1 {
			stylePrefix = text[m[2]:m[3]]
		}
		name := text[m[4]:m[5]]
		out = append(out, IconRef{Style: faStyleFromPrefix(stylePrefix, defaultFAStyle), Name: name})
		cursor = m[1]
	}

	if cursor < len(text) {
		out = append(out, parseEmphasis(text[cursor:])...)
	}

	return out
}

func faStyleFromPrefix(prefix, defaultStyle string) string {
	switch prefix {
	case "":
		return defaultStyle
	case "s":
		return "solid"
	case "r":
		return "regular"
	case "b":
		return "brands"
	case "l", "d":
		// FA Free has no light or duotone; substitute with solid.
		return "solid"
	default:
		return defaultStyle
	}
}

func parseEmphasis(s string) []Inline {
	var out []Inline
	cursor := 0

	for _, m := range emphasisRE.FindAllStringIndex(s, -1) {
		if m[0] > cursor {
			out = append(out, TextRun{Text: s[cursor:m[0]]})
		}

		token := s[m[0]:m[1]]
		switch {
		case strings.HasPrefix(token, "**") && strings.HasSuffix(token, "**"):
			out = append(out, TextRun{Text: token[2 : len(token)-2], Bold: true})
		case strings.HasPrefix(token, "__") && strings.HasSuffix(token, "__"):
			out = append(out, TextRun{Text: token[2 : len(token)-2], Bold: true})
		case strings.HasPrefix(token, "`") && strings.HasSuffix(token, "`"):
			out = append(out, TextRun{Text: token[1 : len(token)-1], Code: true})
		case strings.HasPrefix(token, "*") && strings.HasSuffix(token, "*"):
			out = append(out, TextRun{Text: token[1 : len(token)-1], Italic: true})
		case strings.HasPrefix(token, "_") && strings.HasSuffix(token, "_"):
			out = append(out, TextRun{Text: token[1 : len(token)-1], Italic: true})
		}
		cursor = m[1]
	}

	if cursor < len(s) {
		out = append(out, TextRun{Text: s[cursor:]})
	}

	return out
}

// stripFAShortcodes removes :fa*-name: tokens from text (used for speaker
// notes which can't render images).
func stripFAShortcodes(text string) string {
	return faShortcodeRE.ReplaceAllString(text, "")
}
