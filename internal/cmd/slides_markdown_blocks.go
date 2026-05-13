package cmd

import (
	"regexp"
	"strings"
)

var (
	bulletRE  = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	orderedRE = regexp.MustCompile(`^(\s*)\d+\.\s+(.*)$`)
	headingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
)

const (
	colsOpen     = "::cols::"
	colsClose    = "::/cols::"
	colMarker2   = "::col2::"
	colMarker3   = "::col3::"
	colMarkerAlt = "::right::" // synonym for col2
)

// parseBlocks turns body markdown into top-level blocks. It handles
// paragraphs, bullets (- or *), ordered lists (1.), fenced code blocks,
// and headings. Column / boxes / arrows / mermaid markers are recognized
// in later tasks (5, 6, 7); this parser delegates to helpers from those
// tasks once they exist.
func parseBlocks(body string, defaultFAStyle string) []Block {
	lines := strings.Split(body, "\n")
	var out []Block

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip blank lines between blocks.
		if trimmed == "" {
			i++
			continue
		}

		// Fenced code block.
		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimPrefix(trimmed, "```")
			var src strings.Builder
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				if src.Len() > 0 {
					src.WriteString("\n")
				}
				src.WriteString(lines[i])
				i++
			}
			if i < len(lines) {
				i++ // consume closing ```
			}
			out = append(out, CodeBlock{Lang: lang, Source: src.String()})
			continue
		}

		// Columns block.
		if trimmed == colsOpen {
			i++
			cols, consumed := consumeColumnsBlock(lines[i:], defaultFAStyle)
			i += consumed
			out = append(out, cols)
			continue
		}

		// Heading.
		if m := headingRE.FindStringSubmatch(line); m != nil {
			out = append(out, HeadingBlock{
				Level:   len(m[1]),
				Inlines: parseInlines(strings.TrimSpace(m[2]), defaultFAStyle),
			})
			i++
			continue
		}

		// Bullet list (consume run of bullet lines).
		if bulletRE.MatchString(line) {
			var items []BulletItem
			for i < len(lines) {
				m := bulletRE.FindStringSubmatch(lines[i])
				if m == nil {
					break
				}
				items = append(items, BulletItem{
					Indent:  len(m[1]) / 2,
					Inlines: parseInlines(strings.TrimSpace(m[2]), defaultFAStyle),
				})
				i++
			}
			out = append(out, BulletsBlock{Items: items})
			continue
		}

		// Ordered list.
		if orderedRE.MatchString(line) {
			var items []BulletItem
			for i < len(lines) {
				m := orderedRE.FindStringSubmatch(lines[i])
				if m == nil {
					break
				}
				items = append(items, BulletItem{
					Indent:  len(m[1]) / 2,
					Inlines: parseInlines(strings.TrimSpace(m[2]), defaultFAStyle),
				})
				i++
			}
			out = append(out, BulletsBlock{Ordered: true, Items: items})
			continue
		}

		// Paragraph: consume contiguous non-blank, non-special lines.
		var paraLines []string
		for i < len(lines) {
			pl := lines[i]
			pt := strings.TrimSpace(pl)
			if pt == "" || strings.HasPrefix(pt, "```") || bulletRE.MatchString(pl) ||
				orderedRE.MatchString(pl) || headingRE.MatchString(pl) {
				break
			}
			paraLines = append(paraLines, pt)
			i++
		}
		if len(paraLines) > 0 {
			out = append(out, ParagraphBlock{
				Inlines: parseInlines(strings.Join(paraLines, " "), defaultFAStyle),
			})
		}
	}

	return out
}

func consumeColumnsBlock(lines []string, defaultFAStyle string) (ColumnsBlock, int) {
	var current []string
	var columns [][]string
	flush := func() {
		columns = append(columns, append([]string(nil), current...))
		current = nil
	}

	consumed := 0
	for consumed < len(lines) {
		line := lines[consumed]
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case colsClose:
			flush()
			consumed++
			return columnsBlockFromRaw(columns, defaultFAStyle), consumed
		case colMarker2, colMarker3, colMarkerAlt:
			flush()
			consumed++
			continue
		}
		current = append(current, line)
		consumed++
	}
	// EOF without close — still flush what we have.
	flush()
	return columnsBlockFromRaw(columns, defaultFAStyle), consumed
}

func columnsBlockFromRaw(raw [][]string, defaultFAStyle string) ColumnsBlock {
	cb := ColumnsBlock{}
	for _, col := range raw {
		body := strings.Join(col, "\n")
		cb.Columns = append(cb.Columns, parseBlocks(body, defaultFAStyle))
	}
	return cb
}
