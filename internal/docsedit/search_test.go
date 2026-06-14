package docsedit

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestFindTextRangesAllOccurrenceCaseAndUTF16(t *testing.T) {
	t.Parallel()

	document := searchDocument(
		searchParagraph(1, "Hi ", "😀 needle ", "NEEDLE", "\n"),
	)

	matches := FindTextRanges(document, "needle", SearchOptions{})
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2: %#v", len(matches), matches)
	}

	if matches[0].StartIndex != 7 || matches[0].EndIndex != 13 {
		t.Fatalf("first range = %d..%d, want 7..13", matches[0].StartIndex, matches[0].EndIndex)
	}

	if matches[1].StartIndex != 14 || matches[1].EndIndex != 20 {
		t.Fatalf("second range = %d..%d, want 14..20", matches[1].StartIndex, matches[1].EndIndex)
	}

	caseMatches := FindTextRanges(document, "needle", SearchOptions{MatchCase: true})
	if len(caseMatches) != 1 || caseMatches[0].StartIndex != 7 {
		t.Fatalf("case-sensitive matches = %#v, want first match only", caseMatches)
	}
}

func TestFindTextRangesNormalizesWhitespaceAndEntities(t *testing.T) {
	t.Parallel()

	document := searchDocument(
		searchParagraph(1, "Tom", "\t  & Jerry\n", "next"),
	)

	matches := FindTextRanges(document, "Tom &amp; Jerry next", SearchOptions{
		NormalizeWhitespace: true,
	})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1: %#v", len(matches), matches)
	}

	if got := matches[0]; got.StartIndex != 1 || got.EndIndex != 19 {
		t.Fatalf("range = %d..%d, want 1..19", got.StartIndex, got.EndIndex)
	}

	noNormalize := FindTextRanges(
		document,
		"Tom & Jerry next",
		SearchOptions{NormalizeWhitespace: false},
	)
	if len(noNormalize) != 0 {
		t.Fatalf("no-normalize matches = %#v, want none", noNormalize)
	}

	preserved := FindTextRanges(document, "Tom &amp; Jerry", SearchOptions{
		NormalizeWhitespace:  true,
		PreserveHTMLEntities: true,
	})
	if len(preserved) != 0 {
		t.Fatalf("preserved entity matches = %#v, want none", preserved)
	}
}

func TestFindTextRangesTablesAndParagraphIndex(t *testing.T) {
	t.Parallel()

	document := &docs.Document{Body: &docs.Body{Content: []*docs.StructuralElement{
		searchParagraph(1, "before\n"),
		{
			StartIndex: 8,
			EndIndex:   40,
			Table: &docs.Table{TableRows: []*docs.TableRow{{
				TableCells: []*docs.TableCell{{
					Content: []*docs.StructuralElement{
						searchParagraph(10, "cell target\n"),
					},
				}},
			}}},
		},
		searchParagraph(40, "after target\n"),
	}}}

	matches := FindTextRanges(document, "target", SearchOptions{})
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want 2", matches)
	}

	if matches[0].ParagraphIndex != 1 || matches[0].StartIndex != 15 || !matches[0].InTable {
		t.Fatalf("table match = %#v, want paragraphIndex=1 start=15 inTable", matches[0])
	}

	if matches[1].ParagraphIndex != 2 || matches[1].StartIndex != 46 || matches[1].InTable {
		t.Fatalf("body match = %#v, want paragraphIndex=2 start=46", matches[1])
	}
}

func TestFindTextRangesAcrossParagraphsUnlessSegmentScoped(t *testing.T) {
	t.Parallel()

	document := searchDocument(
		searchParagraph(1, "First paragraph\n"),
		searchParagraph(17, "Second paragraph\n"),
	)
	needle := "paragraph Second paragraph"

	matches := FindTextRanges(document, needle, SearchOptions{NormalizeWhitespace: true})
	if len(matches) != 1 {
		t.Fatalf("matches = %#v, want 1", matches)
	}

	if got := matches[0]; got.StartIndex != 7 || got.EndIndex != 33 || got.ParagraphIndex != 0 {
		t.Fatalf("match = %#v, want start=7 end=33 paragraphIndex=0", got)
	}

	scoped := FindTextRanges(document, needle, SearchOptions{
		NormalizeWhitespace: true,
		RequireTextSegment:  true,
	})
	if len(scoped) != 0 {
		t.Fatalf("segment-scoped matches = %#v, want none", scoped)
	}
}

func TestFindTextRangesPreservesNonOverlappingOrder(t *testing.T) {
	t.Parallel()

	document := searchDocument(searchParagraph(1, "aaaa\n"))

	matches := FindTextRanges(document, "aa", SearchOptions{RequireTextSegment: true})
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want 2", matches)
	}

	if matches[0].StartIndex != 1 || matches[0].EndIndex != 3 ||
		matches[1].StartIndex != 3 || matches[1].EndIndex != 5 {
		t.Fatalf("matches = %#v, want 1..3 then 3..5", matches)
	}
}

func searchDocument(elements ...*docs.StructuralElement) *docs.Document {
	return &docs.Document{Body: &docs.Body{Content: elements}}
}

func searchParagraph(start int64, parts ...string) *docs.StructuralElement {
	paragraph := &docs.Paragraph{}
	index := start

	for _, part := range parts {
		end := index
		for _, character := range part {
			end += utf16RuneLength(character)
		}
		paragraph.Elements = append(paragraph.Elements, &docs.ParagraphElement{
			StartIndex: index,
			EndIndex:   end,
			TextRun:    &docs.TextRun{Content: part},
		})
		index = end
	}

	return &docs.StructuralElement{
		StartIndex: start,
		EndIndex:   index,
		Paragraph:  paragraph,
	}
}
