package slidesmarkdown

import (
	"fmt"
	"strings"
)

func blocksToPlainText(blocks []Block) string {
	var b strings.Builder

	for i, block := range blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}

		switch value := block.(type) {
		case ParagraphBlock:
			b.WriteString(InlineText(value.Inlines))
		case HeadingBlock:
			b.WriteString(InlineText(value.Inlines))
		case BulletsBlock:
			for j, item := range value.Items {
				if j > 0 {
					b.WriteByte('\n')
				}

				if value.Ordered {
					fmt.Fprintf(&b, "%d. ", j+1)
				}

				b.WriteString(InlineText(item.Inlines))
			}
		case CodeBlock:
			b.WriteString(value.Source)
		case ColumnsBlock:
			for j, column := range value.Columns {
				if j > 0 {
					b.WriteString("\n\n")
				}

				b.WriteString(blocksToPlainText(column))
			}
		case IconRowsBlock:
			for j, row := range value.Rows {
				if j > 0 {
					b.WriteByte('\n')
				}

				b.WriteString(row.Text)
			}
		}
	}

	return b.String()
}

func collectDiagrams(slides []Slide) map[string]string {
	out := map[string]string{}
	var walk func([]Block)

	walk = func(blocks []Block) {
		for _, block := range blocks {
			switch value := block.(type) {
			case DiagramBlock:
				out[value.ID] = value.Source
			case ColumnsBlock:
				for _, column := range value.Columns {
					walk(column)
				}
			}
		}
	}
	for _, slide := range slides {
		walk(slide.Body)
	}

	return out
}
