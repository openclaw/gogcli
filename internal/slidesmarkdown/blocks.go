package slidesmarkdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	gtext "github.com/yuin/goldmark/text"
)

var headingRE = regexp.MustCompile(`^(#{1,6})(?:\s+(.*))?$`)

type blockIDGenerator struct {
	next uint64
}

func (g *blockIDGenerator) nextBlockID() string {
	if g == nil {
		g = &blockIDGenerator{}
	}

	g.next++

	return fmt.Sprintf("block-%d", g.next)
}

const (
	colsOpen     = "::cols::"
	colsClose    = "::/cols::"
	colMarker2   = "::col2::"
	colMarker3   = "::col3::"
	colMarkerAlt = "::right::" // synonym for col2
	boxesOpen    = "::boxes::"
	boxesClose   = "::/boxes::"
	arrowsOpen   = "::arrows::"
	arrowsClose  = "::/arrows::"
)

// parseBlocks turns body markdown into top-level blocks. CommonMark parsing
// is delegated to goldmark; slidey-specific directive blocks are recognized
// by a thin line scanner before each ordinary markdown chunk is parsed.
func parseBlocks(body string) []Block {
	return parseBlocksWithIDs(body, "solid", &blockIDGenerator{})
}

func parseBlocksWithIDs(body string, defaultFAStyle string, ids *blockIDGenerator) []Block {
	lines := strings.Split(body, "\n")
	var out []Block
	var chunk []string
	flushChunk := func() {
		text := strings.Join(chunk, "\n")
		chunk = nil

		if strings.TrimSpace(text) == "" {
			return
		}

		out = append(out, parseGoldmarkBlocks(text, defaultFAStyle, ids)...)
	}

	i := 0
	var fenceChar byte
	fenceLen := 0

	for i < len(lines) {
		line := lines[i]

		trimmed := strings.TrimSpace(line)
		if isFenceDelimiter(line) {
			fenceChar, fenceLen = updateMarkdownFenceState(line, fenceChar, fenceLen)
			chunk = append(chunk, line)
			i++

			continue
		}

		if fenceLen == 0 {
			// Columns block.
			if trimmed == colsOpen {
				flushChunk()

				i++

				cols, consumed := consumeColumnsBlock(lines[i:], defaultFAStyle, ids)
				i += consumed

				out = append(out, cols)

				continue
			}

			if trimmed == boxesOpen || trimmed == arrowsOpen {
				flushChunk()
				kind := "boxes"
				closeMarker := boxesClose

				if trimmed == arrowsOpen {
					kind = "arrows"
					closeMarker = arrowsClose
				}

				rows, consumed := consumeIconRowsBlock(lines[i+1:], kind, closeMarker, defaultFAStyle)
				i += consumed + 1

				out = append(out, rows)

				continue
			}
		}

		chunk = append(chunk, line)
		i++
	}

	flushChunk()

	return out
}

func parseGoldmarkBlocks(markdown string, defaultFAStyle string, ids *blockIDGenerator) []Block {
	source := []byte(markdown)
	doc := goldmark.DefaultParser().Parse(gtext.NewReader(source))
	var out []Block

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		out = append(out, goldmarkBlockToBlocks(n, source, defaultFAStyle, ids)...)
	}

	return out
}

func goldmarkBlockToBlocks(n gast.Node, source []byte, defaultFAStyle string, ids *blockIDGenerator) []Block {
	switch v := n.(type) {
	case *gast.Heading:
		return []Block{HeadingBlock{Level: v.Level, Inlines: goldmarkInlines(v, source, defaultFAStyle, false, false)}}
	case *gast.Paragraph:
		return []Block{ParagraphBlock{Inlines: goldmarkInlines(v, source, defaultFAStyle, false, false)}}
	case *gast.TextBlock:
		return []Block{ParagraphBlock{Inlines: goldmarkInlines(v, source, defaultFAStyle, false, false)}}
	case *gast.FencedCodeBlock:
		lang := string(v.Language(source))
		code := strings.TrimSuffix(string(v.Lines().Value(source)), "\n")

		if lang == "mermaid" {
			return []Block{DiagramBlock{Kind: "mermaid", Source: code, ID: ids.nextBlockID()}}
		}

		return []Block{CodeBlock{Lang: lang, Source: code}}
	case *gast.CodeBlock:
		return []Block{CodeBlock{Source: strings.TrimSuffix(string(v.Lines().Value(source)), "\n")}}
	case *gast.List:
		return []Block{goldmarkListToBlock(v, source, defaultFAStyle)}
	case *gast.Blockquote:
		var out []Block
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			out = append(out, goldmarkBlockToBlocks(c, source, defaultFAStyle, ids)...)
		}

		return out
	case *gast.ThematicBreak:
		return nil
	default:
		if n.HasChildren() {
			var out []Block

			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				out = append(out, goldmarkBlockToBlocks(c, source, defaultFAStyle, ids)...)
			}

			return out
		}

		if n.Lines() != nil && n.Lines().Len() > 0 {
			text := strings.TrimSpace(string(n.Lines().Value(source)))
			if text != "" {
				return []Block{ParagraphBlock{Inlines: parseInlines(text, defaultFAStyle)}}
			}
		}

		return nil
	}
}

func goldmarkListToBlock(list *gast.List, source []byte, defaultFAStyle string) BulletsBlock {
	var items []BulletItem
	appendGoldmarkListItems(list, source, defaultFAStyle, 0, &items)

	return BulletsBlock{Ordered: list.IsOrdered(), Items: items}
}

func appendGoldmarkListItems(list *gast.List, source []byte, defaultFAStyle string, indent int, items *[]BulletItem) {
	for n := list.FirstChild(); n != nil; n = n.NextSibling() {
		item, ok := n.(*gast.ListItem)
		if !ok {
			continue
		}

		var inlines []Inline
		var nested []*gast.List

		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *gast.Paragraph:
				if len(inlines) > 0 {
					inlines = append(inlines, TextRun{Text: " "})
				}
				inlines = append(inlines, goldmarkInlines(v, source, defaultFAStyle, false, false)...)
			case *gast.TextBlock:
				if len(inlines) > 0 {
					inlines = append(inlines, TextRun{Text: " "})
				}
				inlines = append(inlines, goldmarkInlines(v, source, defaultFAStyle, false, false)...)
			case *gast.List:
				nested = append(nested, v)
			}
		}

		if len(inlines) > 0 {
			*items = append(*items, BulletItem{Indent: indent, Inlines: inlines})
		}

		for _, nestedList := range nested {
			appendGoldmarkListItems(nestedList, source, defaultFAStyle, indent+1, items)
		}
	}
}

func goldmarkInlines(parent gast.Node, source []byte, defaultFAStyle string, bold, italic bool) []Inline {
	var out []Inline

	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch v := n.(type) {
		case *gast.Text:
			text := string(v.Value(source))
			if v.SoftLineBreak() {
				text += " "
			} else if v.HardLineBreak() {
				text += "\n"
			}

			out = append(out, styleTextRuns(parseInlines(text, defaultFAStyle), bold, italic, false)...)
		case *gast.String:
			out = append(out, styleTextRuns(parseInlines(string(v.Value), defaultFAStyle), bold, italic, v.IsCode())...)
		case *gast.CodeSpan:
			out = append(out, TextRun{Text: goldmarkInlinePlainText(v, source), Code: true})
		case *gast.Emphasis:
			out = append(out, goldmarkInlines(v, source, defaultFAStyle, bold || v.Level >= 2, italic || v.Level == 1)...)
		default:
			if n.HasChildren() {
				out = append(out, goldmarkInlines(n, source, defaultFAStyle, bold, italic)...)
			}
		}
	}

	return out
}

func goldmarkInlinePlainText(parent gast.Node, source []byte) string {
	var b strings.Builder
	_ = gast.Walk(parent, func(n gast.Node, entering bool) (gast.WalkStatus, error) {
		if !entering {
			return gast.WalkContinue, nil
		}

		switch v := n.(type) {
		case *gast.Text:
			b.Write(v.Value(source))
		case *gast.String:
			b.Write(v.Value)
		}

		return gast.WalkContinue, nil
	})

	return b.String()
}

func styleTextRuns(in []Inline, bold, italic, code bool) []Inline {
	if !bold && !italic && !code {
		return in
	}

	out := make([]Inline, 0, len(in))
	for _, item := range in {
		if tr, ok := item.(TextRun); ok {
			tr.Bold = tr.Bold || bold
			tr.Italic = tr.Italic || italic
			tr.Code = tr.Code || code
			out = append(out, tr)

			continue
		}

		out = append(out, item)
	}

	return out
}

func consumeColumnsBlock(lines []string, defaultFAStyle string, ids *blockIDGenerator) (ColumnsBlock, int) {
	var current []string
	var columns [][]string
	flush := func() {
		columns = append(columns, append([]string(nil), current...))
		current = nil
	}

	consumed := 0
	var fenceChar byte
	fenceLen := 0

	for consumed < len(lines) {
		line := lines[consumed]
		trimmed := strings.TrimSpace(line)

		if isFenceDelimiter(line) {
			fenceChar, fenceLen = updateMarkdownFenceState(line, fenceChar, fenceLen)
		}

		if fenceLen == 0 {
			switch trimmed {
			case colsClose:
				flush()

				consumed++

				return columnsBlockFromRaw(columns, defaultFAStyle, ids), consumed
			case colMarker2, colMarker3, colMarkerAlt:
				flush()

				consumed++

				continue
			}
		}

		current = append(current, line)
		consumed++
	}

	// EOF without close — still flush what we have.
	flush()

	return columnsBlockFromRaw(columns, defaultFAStyle, ids), consumed
}

func columnsBlockFromRaw(raw [][]string, defaultFAStyle string, ids *blockIDGenerator) ColumnsBlock {
	cb := ColumnsBlock{}

	for _, col := range raw {
		body := strings.Join(col, "\n")
		cb.Columns = append(cb.Columns, parseBlocksWithIDs(body, defaultFAStyle, ids))
	}

	return cb
}

func consumeIconRowsBlock(lines []string, kind, closeMarker, defaultFAStyle string) (IconRowsBlock, int) {
	block := IconRowsBlock{Kind: kind}
	consumed := 0

	for consumed < len(lines) {
		line := strings.TrimSpace(lines[consumed])
		consumed++

		if line == "" {
			continue
		}

		if line == closeMarker {
			return block, consumed
		}

		block.Rows = append(block.Rows, parseIconRow(line, defaultFAStyle))
	}

	return block, consumed
}

func parseIconRow(line, defaultFAStyle string) IconRow {
	line = strings.TrimSpace(line)

	if m := headingRE.FindStringSubmatch(line); m != nil {
		line = strings.TrimSpace(m[2])
	}

	inlines := parseInlines(line, defaultFAStyle)
	if len(inlines) == 0 {
		return IconRow{Text: line}
	}

	if icon, ok := inlines[0].(IconRef); ok {
		return IconRow{Icon: &icon, Text: strings.TrimSpace(InlineText(inlines[1:]))}
	}

	return IconRow{Text: strings.TrimSpace(InlineText(inlines))}
}
