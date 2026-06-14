package slidesmarkdown

import (
	"strings"
)

const layoutTitle = "title"

// ParseOptions configures the markdown parser.
type ParseOptions struct {
	DefaultFAStyle string // "solid"|"regular"|"brands"; empty → "solid"
}

// Parse parses a slidey-flavored markdown deck into Slide AST nodes.
func Parse(markdown string, opts ParseOptions) ([]Slide, error) {
	if opts.DefaultFAStyle == "" {
		opts.DefaultFAStyle = "solid"
	}

	blocks, err := splitMarkdownIntoSlideBlocks(markdown)
	if err != nil {
		return nil, err
	}
	out := make([]Slide, 0, len(blocks))

	ids := &blockIDGenerator{}
	for _, b := range blocks {
		out = append(out, parseSlideFromBlock(b, opts, ids))
	}

	return out, nil
}

func parseSlideFromBlock(b slideBlock, opts ParseOptions, ids *blockIDGenerator) Slide {
	body, notesText := splitOutNotes(b.Body)
	body = normalizeShorthandColumns(body, b.Frontmatter.Layout)
	parsed := parseBlocksWithIDs(body, opts.DefaultFAStyle, ids)

	slide := Slide{
		Frontmatter: b.Frontmatter,
		Body:        parsed,
		Notes:       stripFAShortcodes(notesText),
	}

	if !layoutSkipsTitleHoist(b.Frontmatter.Layout) {
		title, remaining := hoistTitle(parsed)
		slide.Title = title
		slide.Body = remaining
	}

	return slide
}

// splitOutNotes scans body lines for an exact "## Notes" or "### Notes"
// heading (case-sensitive). Everything from that heading to the end is
// returned as raw notes text (without the heading itself); the body
// returned is everything before.
func splitOutNotes(body string) (newBody string, notes string) {
	lines := strings.Split(body, "\n")
	var fenceChar byte
	fenceLen := 0

	for i, line := range lines {
		if isFenceDelimiter(line) {
			fenceChar, fenceLen = updateMarkdownFenceState(line, fenceChar, fenceLen)
			continue
		}

		if fenceLen > 0 {
			continue
		}

		t := strings.TrimSpace(line)
		if t == "## Notes" || t == "### Notes" {
			b := strings.Join(lines[:i], "\n")
			n := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))

			return b, n
		}
	}

	return body, ""
}

// hoistTitle returns the first h1 (or h2 fallback) inline text and the
// blocks with that heading removed.
func hoistTitle(blocks []Block) (string, []Block) {
	// First pass: look for h1.
	for i, b := range blocks {
		if h, ok := b.(HeadingBlock); ok && h.Level == 1 {
			return InlineText(h.Inlines), removeIndex(blocks, i)
		}
	}
	// Fallback: first h2.
	for i, b := range blocks {
		if h, ok := b.(HeadingBlock); ok && h.Level == 2 {
			return InlineText(h.Inlines), removeIndex(blocks, i)
		}
	}

	return "", blocks
}

func removeIndex(s []Block, i int) []Block {
	out := make([]Block, 0, len(s)-1)
	out = append(out, s[:i]...)
	out = append(out, s[i+1:]...)

	return out
}

// InlineText returns the textual content of inline runs, excluding icons.
func InlineText(inlines []Inline) string {
	var b strings.Builder

	for _, in := range inlines {
		if tr, ok := in.(TextRun); ok {
			b.WriteString(tr.Text)
		}
	}

	return b.String()
}

func layoutSkipsTitleHoist(layout string) bool {
	switch layout {
	case layoutTitle, "hero", "statement":
		return true
	}

	return false
}

func normalizeShorthandColumns(body, layout string) string {
	if layout != "two-cols" && layout != "three-cols" {
		return body
	}

	if hasExplicitColumnsBlock(body) || !hasShorthandColumnMarker(body) {
		return body
	}

	lines := strings.Split(body, "\n")

	columnStart := 0
	for columnStart < len(lines) && strings.TrimSpace(lines[columnStart]) == "" {
		columnStart++
	}

	if columnStart < len(lines) && headingRE.MatchString(lines[columnStart]) {
		columnStart++
		for columnStart < len(lines) && strings.TrimSpace(lines[columnStart]) == "" {
			columnStart++
		}
	}

	var out []string
	out = append(out, lines[:columnStart]...)
	out = append(out, colsOpen)

	out = append(out, lines[columnStart:]...)
	if len(out) == 0 || strings.TrimSpace(out[len(out)-1]) != "" {
		out = append(out, "")
	}
	out = append(out, colsClose)

	return strings.Join(out, "\n")
}

func hasExplicitColumnsBlock(body string) bool {
	var fenceChar byte
	fenceLen := 0

	for _, line := range strings.Split(body, "\n") {
		if isFenceDelimiter(line) {
			fenceChar, fenceLen = updateMarkdownFenceState(line, fenceChar, fenceLen)
			continue
		}

		if fenceLen > 0 {
			continue
		}

		if strings.TrimSpace(line) == colsOpen {
			return true
		}
	}

	return false
}

func hasShorthandColumnMarker(body string) bool {
	var fenceChar byte
	fenceLen := 0

	for _, line := range strings.Split(body, "\n") {
		if isFenceDelimiter(line) {
			fenceChar, fenceLen = updateMarkdownFenceState(line, fenceChar, fenceLen)
			continue
		}

		if fenceLen > 0 {
			continue
		}

		switch strings.TrimSpace(line) {
		case colMarker2, colMarker3, colMarkerAlt:
			return true
		}
	}

	return false
}

func isFenceDelimiter(line string) bool {
	_, _, ok := markdownFenceMarker(line)
	return ok
}
