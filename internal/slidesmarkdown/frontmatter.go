package slidesmarkdown

import (
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// slideBlock is the intermediate form between raw markdown and the parsed
// Slide AST: per-slide frontmatter + the raw body markdown for that slide.
type slideBlock struct {
	Frontmatter SlideFrontmatter
	Body        string
}

const markdownTripleDash = "---"

var yamlKeyLineRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*:\s`)

// splitMarkdownIntoSlideBlocks walks markdown line by line, splits on bare
// "---" separators, and detects per-slide frontmatter using the rule from
// the design spec (§4.1):
//
//  1. A "---" at file start, or immediately following another "---" separator
//     (only blank lines between), opens a frontmatter candidate.
//  2. The next non-blank line must match a YAML key (^[A-Za-z_][\w-]*:\s).
//     If not, the original "---" is a separator and the candidate is abandoned.
//  3. Scan the contiguous YAML header lines; a blank or non-key line before
//     the closing "---" abandons the candidate so key-value prose after a slide
//     separator stays body text.
func splitMarkdownIntoSlideBlocks(markdown string) ([]slideBlock, error) {
	// Normalize CRLF so downstream regex matches and body strings stay clean
	// regardless of authoring platform.
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(markdown, "\n")
	var blocks []slideBlock

	i := 0
	for i < len(lines) {
		// Try to consume a frontmatter block at the current position.
		// tryConsumeFrontmatter will consume the opening "---" itself, so if
		// the current position IS a "---" that turns out to be a frontmatter
		// opener, it is removed from the body.
		fm, after, ok, err := tryConsumeFrontmatter(lines, i)
		if err != nil {
			return nil, err
		}

		if ok {
			i = after
			// Skip the blank line(s) separating frontmatter from body.
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
		} else {
			// Not frontmatter: if we're sitting on a "---" it was already
			// determined to be a plain separator — skip it plus trailing blanks.
			if i < len(lines) && isBareDelimiter(lines[i]) {
				i++
				for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
					i++
				}
			}
			fm = SlideFrontmatter{Raw: map[string]string{}}
		}

		// Consume body lines until the next bare "---" separator or EOF.
		// Markdown fences are handled here, before Goldmark sees the slide
		// body, so delimiter examples inside code blocks stay intact.
		bodyStart := i
		var fenceChar byte

		fenceLen := 0
		for i < len(lines) {
			inFence := fenceLen > 0
			if !inFence && isBareDelimiter(lines[i]) {
				break
			}
			fenceChar, fenceLen = updateMarkdownFenceState(lines[i], fenceChar, fenceLen)
			i++
		}
		bodyLines := lines[bodyStart:i]

		body := strings.Join(bodyLines, "\n")
		if strings.TrimSpace(body) == "" {
			continue
		}
		blocks = append(blocks, slideBlock{Frontmatter: fm, Body: body})

		// Leave the "---" in place; the next iteration will call
		// tryConsumeFrontmatter which will decide if it opens frontmatter or
		// is a plain separator.
	}

	return blocks, nil
}

func tryConsumeFrontmatter(lines []string, start int) (SlideFrontmatter, int, bool, error) {
	// Skip leading blank lines.
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	if i >= len(lines) || !isBareDelimiter(lines[i]) {
		return SlideFrontmatter{}, start, false, nil
	}

	// First non-blank line after "---" must look like a known frontmatter key.
	j := i + 1
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}

	if j >= len(lines) {
		return SlideFrontmatter{}, start, false, nil
	}

	if !isFrontmatterStartKey(lines[j]) {
		return SlideFrontmatter{}, start, false, nil
	}

	// Find closing "---" before normal body content begins. This intentionally
	// keeps "Problem: hard\n\nbody\n---" as slide content, not frontmatter.
	closeIdx := -1

	for k := j; k < len(lines); k++ {
		if isBareDelimiter(lines[k]) {
			closeIdx = k

			break
		}

		trimmed := strings.TrimSpace(lines[k])
		if trimmed == "" {
			return SlideFrontmatter{}, start, false, nil
		}

		if !isFrontmatterStartKey(lines[k]) {
			return SlideFrontmatter{}, start, false, nil
		}
	}

	if closeIdx == -1 {
		return SlideFrontmatter{}, start, false, nil
	}

	yamlText := strings.Join(lines[i+1:closeIdx], "\n")

	fm, err := parseSlideFrontmatter(yamlText)
	if err != nil {
		return SlideFrontmatter{}, start, false, fmt.Errorf("frontmatter at line %d: %w", i+1, err)
	}

	return fm, closeIdx + 1, true, nil
}

func parseSlideFrontmatter(yamlText string) (SlideFrontmatter, error) {
	raw := map[string]string{}

	if strings.TrimSpace(yamlText) != "" {
		var m map[string]any
		if err := yaml.Unmarshal([]byte(yamlText), &m); err != nil {
			return SlideFrontmatter{}, fmt.Errorf("decode YAML: %w", err)
		}

		for k, v := range m {
			raw[k] = fmt.Sprintf("%v", v)
		}
	}

	return SlideFrontmatter{
		Layout:  raw["layout"],
		Content: raw["content"],
		Raw:     raw,
	}, nil
}

func isFrontmatterStartKey(line string) bool {
	trimmed := strings.TrimSpace(line)
	return yamlKeyLineRE.MatchString(trimmed)
}

func isBareDelimiter(line string) bool {
	return strings.TrimSpace(line) == markdownTripleDash
}

func updateMarkdownFenceState(line string, fenceChar byte, fenceLen int) (byte, int) {
	char, length, ok := markdownFenceMarker(line)
	if !ok {
		return fenceChar, fenceLen
	}

	if fenceLen == 0 {
		return char, length
	}

	if char == fenceChar && length >= fenceLen && markdownFenceCloser(line, length) {
		return 0, 0
	}

	return fenceChar, fenceLen
}

func markdownFenceMarker(line string) (byte, int, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return 0, 0, false
	}

	char := trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, false
	}

	length := 0
	for length < len(trimmed) && trimmed[length] == char {
		length++
	}

	if length < 3 {
		return 0, 0, false
	}

	return char, length, true
}

func markdownFenceCloser(line string, markerLen int) bool {
	trimmed := strings.TrimSpace(line)
	if markerLen > len(trimmed) {
		return false
	}

	return strings.TrimSpace(trimmed[markerLen:]) == ""
}
