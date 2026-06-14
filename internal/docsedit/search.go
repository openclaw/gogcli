package docsedit

import (
	"html"
	"strings"
	"unicode"

	"google.golang.org/api/docs/v1"
)

// SearchOptions controls document text normalization and range constraints.
type SearchOptions struct {
	MatchCase            bool
	NormalizeWhitespace  bool
	TabID                string
	PreserveHTMLEntities bool
	RequireTextSegment   bool
}

// TextRange is one matched Google Docs UTF-16 range.
//
//nolint:tagliatelle // Existing command JSON output uses camelCase fields.
type TextRange struct {
	StartIndex     int64  `json:"startIndex"`
	EndIndex       int64  `json:"endIndex"`
	ParagraphIndex int    `json:"paragraphIndex"`
	TabID          string `json:"tabId"`
	InTable        bool   `json:"inTable,omitempty"`
}

type textUnit struct {
	startByte      int
	endByte        int
	startIndex     int64
	endIndex       int64
	paragraphIndex int
	segment        int
	inTable        bool
}

// FindTextRanges returns non-overlapping matches in document source order.
func FindTextRanges(
	document *docs.Document,
	searchText string,
	options SearchOptions,
) []TextRange {
	if document == nil || document.Body == nil {
		return nil
	}

	needle := prepareSearchNeedle(searchText, options)
	if needle == "" {
		return nil
	}

	text, units := buildComparableDocumentText(document.Body.Content, options)
	if text == "" {
		return nil
	}

	var matches []TextRange
	offset := 0

	for {
		index := strings.Index(text[offset:], needle)
		if index < 0 {
			return matches
		}

		startByte := offset + index
		endByte := startByte + len(needle)

		match, ok := originalRangeForComparableBytes(
			units,
			startByte,
			endByte,
			options.RequireTextSegment,
			options.TabID,
		)
		if ok {
			matches = append(matches, match)
			offset = endByte

			continue
		}

		offset = startByte + 1
	}
}

func prepareSearchNeedle(text string, options SearchOptions) string {
	if !options.PreserveHTMLEntities {
		text = html.UnescapeString(text)
	}

	var builder strings.Builder
	lastWasWhitespace := false

	for _, character := range text {
		if options.NormalizeWhitespace && unicode.IsSpace(character) {
			if !lastWasWhitespace {
				builder.WriteRune(' ')
				lastWasWhitespace = true
			}

			continue
		}

		if !options.MatchCase {
			character = unicode.ToLower(character)
		}

		builder.WriteRune(character)
		lastWasWhitespace = false
	}

	return builder.String()
}

func buildComparableDocumentText(
	elements []*docs.StructuralElement,
	options SearchOptions,
) (string, []textUnit) {
	var builder strings.Builder
	var units []textUnit

	lastWhitespaceUnit := -1
	paragraphIndex := 0
	segment := 0

	var walk func([]*docs.StructuralElement, bool)
	walk = func(elements []*docs.StructuralElement, inTable bool) {
		for _, element := range elements {
			if element == nil {
				continue
			}

			switch {
			case element.Paragraph != nil:
				segment++

				if options.RequireTextSegment {
					lastWhitespaceUnit = -1
				}

				appendComparableParagraphText(
					&builder,
					&units,
					element.Paragraph,
					options,
					paragraphIndex,
					&lastWhitespaceUnit,
					&segment,
					inTable,
				)
				paragraphIndex++
			case element.Table != nil:
				for _, row := range element.Table.TableRows {
					if row == nil {
						continue
					}

					for _, cell := range row.TableCells {
						if cell != nil {
							walk(cell.Content, true)
						}
					}
				}
			}
		}
	}

	walk(elements, false)

	return builder.String(), units
}

func appendComparableParagraphText(
	builder *strings.Builder,
	units *[]textUnit,
	paragraph *docs.Paragraph,
	options SearchOptions,
	paragraphIndex int,
	lastWhitespaceUnit *int,
	segment *int,
	inTable bool,
) {
	if paragraph == nil {
		return
	}

	for _, element := range paragraph.Elements {
		if element == nil || element.TextRun == nil {
			(*segment)++

			if options.RequireTextSegment {
				*lastWhitespaceUnit = -1
			}

			continue
		}

		runOffset := int64(0)
		for _, character := range element.TextRun.Content {
			startIndex := element.StartIndex + runOffset
			endIndex := startIndex + utf16RuneLength(character)
			runOffset = endIndex - element.StartIndex

			if options.NormalizeWhitespace && unicode.IsSpace(character) {
				if *lastWhitespaceUnit >= 0 {
					(*units)[*lastWhitespaceUnit].endIndex = endIndex
					continue
				}

				appendTextUnit(
					builder,
					units,
					' ',
					startIndex,
					endIndex,
					paragraphIndex,
					*segment,
					inTable,
				)
				*lastWhitespaceUnit = len(*units) - 1

				continue
			}

			if !options.MatchCase {
				character = unicode.ToLower(character)
			}

			appendTextUnit(
				builder,
				units,
				character,
				startIndex,
				endIndex,
				paragraphIndex,
				*segment,
				inTable,
			)
			*lastWhitespaceUnit = -1
		}
	}
}

func appendTextUnit(
	builder *strings.Builder,
	units *[]textUnit,
	character rune,
	startIndex int64,
	endIndex int64,
	paragraphIndex int,
	segment int,
	inTable bool,
) {
	startByte := builder.Len()
	builder.WriteRune(character)
	*units = append(*units, textUnit{
		startByte:      startByte,
		endByte:        builder.Len(),
		startIndex:     startIndex,
		endIndex:       endIndex,
		paragraphIndex: paragraphIndex,
		segment:        segment,
		inTable:        inTable,
	})
}

func originalRangeForComparableBytes(
	units []textUnit,
	startByte int,
	endByte int,
	requireTextSegment bool,
	tabID string,
) (TextRange, bool) {
	first := -1
	last := -1

	for index, unit := range units {
		if first < 0 && unit.endByte > startByte {
			first = index
		}

		if unit.startByte < endByte {
			last = index
			continue
		}

		break
	}

	if first < 0 || last < first {
		return TextRange{}, false
	}

	if requireTextSegment {
		segment := units[first].segment
		for index := first; index <= last; index++ {
			if units[index].segment != segment {
				return TextRange{}, false
			}

			if index > first && units[index-1].endIndex != units[index].startIndex {
				return TextRange{}, false
			}
		}
	}

	return TextRange{
		StartIndex:     units[first].startIndex,
		EndIndex:       units[last].endIndex,
		ParagraphIndex: units[first].paragraphIndex,
		TabID:          tabID,
		InTable:        units[first].inTable,
	}, true
}

func utf16RuneLength(character rune) int64 {
	if character >= 0x10000 {
		return 2
	}

	return 1
}
