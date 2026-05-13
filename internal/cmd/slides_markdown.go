package cmd

import (
	"strings"
)

// ParseOptions configures the markdown parser.
type ParseOptions struct {
	DefaultFAStyle string // "solid"|"regular"|"brands"; empty → "solid"
}

// ParseMarkdownToSlides parses a slidey-flavored markdown deck into a
// slice of Slide AST nodes. Returns an error if frontmatter is malformed.
func ParseMarkdownToSlides(markdown string, opts ParseOptions) ([]Slide, error) {
	if opts.DefaultFAStyle == "" {
		opts.DefaultFAStyle = "solid"
	}
	blocks, err := splitMarkdownIntoSlideBlocks(markdown)
	if err != nil {
		return nil, err
	}
	out := make([]Slide, 0, len(blocks))
	for _, b := range blocks {
		s, err := parseSlideFromBlock(b, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func parseSlideFromBlock(b slideBlock, opts ParseOptions) (Slide, error) {
	body, notesText := splitOutNotes(b.Body)
	parsed := parseBlocks(body, opts.DefaultFAStyle)

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
	return slide, nil
}

// splitOutNotes scans body lines for an exact "## Notes" or "### Notes"
// heading (case-sensitive). Everything from that heading to the end is
// returned as raw notes text (without the heading itself); the body
// returned is everything before.
func splitOutNotes(body string) (newBody string, notes string) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
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
			return inlinesToText(h.Inlines), removeIndex(blocks, i)
		}
	}
	// Fallback: first h2.
	for i, b := range blocks {
		if h, ok := b.(HeadingBlock); ok && h.Level == 2 {
			return inlinesToText(h.Inlines), removeIndex(blocks, i)
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

func inlinesToText(inlines []Inline) string {
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
	case "title", "hero", "statement":
		return true
	}
	return false
}
