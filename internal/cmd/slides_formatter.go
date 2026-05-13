package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/slides/v1"
)

const slideElementTitle = "title" // legacy const kept for any external callers

// SlideNotesPlan tells the second BatchUpdate which slide gets which
// speaker-notes text. SlideIndex maps to the i-th slide created.
type SlideNotesPlan struct {
	SlideIndex int
	SlideID    string
	Text       string
}

// RenderSlides converts a parsed Slide AST plus an AssetMap into the
// initial BatchUpdate requests AND a notes plan to apply after the
// presentation is created.
func RenderSlides(in []Slide, assets AssetMap, g LayoutGeometry) ([]*slides.Request, []SlideNotesPlan) {
	var reqs []*slides.Request
	var notes []SlideNotesPlan

	for i, slide := range in {
		slideID := fmt.Sprintf("slide_%d", i+1)
		reqs = append(reqs, &slides.Request{
			CreateSlide: &slides.CreateSlideRequest{
				ObjectId:             slideID,
				SlideLayoutReference: &slides.LayoutReference{PredefinedLayout: "BLANK"},
			},
		})

		layout := MapSlideyLayout(slide.Frontmatter.Layout)

		// Title box (skipped for SectionHeader layouts — those put the
		// title in the body box at large size; see Task 16).
		if layout != LayoutKindSectionHeader && slide.Title != "" {
			reqs = append(reqs, renderTitleBox(slideID, i+1, slide.Title, g)...)
		}

		switch layout {
		case LayoutKindSectionHeader:
			// Body box is one large centered text box. Title is rendered
			// inline at 44pt; everything else at the standard size.
			bodyID := fmt.Sprintf("body_%d", i+1)
			reqs = append(reqs, createTextBox(bodyID, slideID, SingleBodyBox(g)))
			text := blocksToPlainText(slide.Body)
			if text != "" {
				reqs = append(reqs, &slides.Request{
					InsertText: &slides.InsertTextRequest{ObjectId: bodyID, Text: text},
				})
			}
			// Style first paragraph (the h1 line) at 44pt bold.
			if firstLineLen := len(strings.SplitN(text, "\n", 2)[0]); firstLineLen > 0 {
				reqs = append(reqs, &slides.Request{
					UpdateTextStyle: &slides.UpdateTextStyleRequest{
						ObjectId: bodyID,
						TextRange: &slides.Range{
							Type:       "FIXED_RANGE",
							StartIndex: int64Ptr(0),
							EndIndex:   int64Ptr(int64(firstLineLen)),
						},
						Style: &slides.TextStyle{
							Bold:     true,
							FontSize: &slides.Dimension{Magnitude: 44, Unit: "PT"},
						},
						Fields: "bold,fontSize",
					},
				})
			}
			reqs = append(reqs, &slides.Request{
				UpdateParagraphStyle: &slides.UpdateParagraphStyleRequest{
					ObjectId:  bodyID,
					TextRange: &slides.Range{Type: "ALL"},
					Style:     &slides.ParagraphStyle{Alignment: "CENTER"},
					Fields:    "alignment",
				},
			})
		case LayoutKindTwoCols, LayoutKindThreeCols:
			n := 2
			if layout == LayoutKindThreeCols {
				n = 3
			}
			boxes := ColumnBoxes(g, n)
			// Find the first ColumnsBlock; if absent, fall back to splitting body evenly.
			cols := findColumnsBlock(slide.Body, n)
			for ci := 0; ci < n; ci++ {
				colID := fmt.Sprintf("body_%d_col%d", i+1, ci+1)
				reqs = append(reqs, createTextBox(colID, slideID, boxes[ci]))
				text := blocksToPlainText(cols[ci])
				if text != "" {
					reqs = append(reqs, &slides.Request{
						InsertText: &slides.InsertTextRequest{ObjectId: colID, Text: text},
					})
				}
			}
		default:
			// LayoutKindDefault, LayoutKindCenter — single body box.
			bodyText := blocksToPlainText(slide.Body)
			bodyID := fmt.Sprintf("body_%d", i+1)
			reqs = append(reqs, createTextBox(bodyID, slideID, SingleBodyBox(g)))
			if bodyText != "" {
				reqs = append(reqs, &slides.Request{
					InsertText: &slides.InsertTextRequest{ObjectId: bodyID, Text: bodyText},
				})
			}
			if layout == LayoutKindCenter {
				reqs = append(reqs, &slides.Request{
					UpdateParagraphStyle: &slides.UpdateParagraphStyleRequest{
						ObjectId:  bodyID,
						TextRange: &slides.Range{Type: "ALL"},
						Style:     &slides.ParagraphStyle{Alignment: "CENTER"},
						Fields:    "alignment",
					},
				})
			}
		}

		if slide.Notes != "" {
			notes = append(notes, SlideNotesPlan{SlideIndex: i, SlideID: slideID, Text: slide.Notes})
		}
	}
	return reqs, notes
}

func renderTitleBox(slideID string, oneBased int, title string, g LayoutGeometry) []*slides.Request {
	titleID := fmt.Sprintf("title_%d", oneBased)
	box := TitleBox(g)
	return []*slides.Request{
		createTextBox(titleID, slideID, box),
		{InsertText: &slides.InsertTextRequest{ObjectId: titleID, Text: title}},
		{UpdateTextStyle: &slides.UpdateTextStyleRequest{
			ObjectId:  titleID,
			TextRange: &slides.Range{Type: "ALL"},
			Style: &slides.TextStyle{
				Bold:     true,
				FontSize: &slides.Dimension{Magnitude: 28, Unit: "PT"},
			},
			Fields: "bold,fontSize",
		}},
	}
}

func createTextBox(objectID, slideID string, box BoxRect) *slides.Request {
	return &slides.Request{
		CreateShape: &slides.CreateShapeRequest{
			ObjectId:  objectID,
			ShapeType: "TEXT_BOX",
			ElementProperties: &slides.PageElementProperties{
				PageObjectId: slideID,
				Transform: &slides.AffineTransform{
					ScaleX: 1, ScaleY: 1,
					TranslateX: box.LeftPT, TranslateY: box.TopPT,
					Unit: "PT",
				},
				Size: &slides.Size{
					Width:  &slides.Dimension{Magnitude: box.WidthPT, Unit: "PT"},
					Height: &slides.Dimension{Magnitude: box.HeightPT, Unit: "PT"},
				},
			},
		},
	}
}

// blocksToPlainText is the simplest body-text extraction: paragraphs
// joined by blank lines, bullets prefixed with "• ", code blocks shown
// verbatim. Inline icons are skipped (Task 17 emits separate image
// requests for them); diagrams are skipped (Task 17 emits CreateImage).
func blocksToPlainText(blocks []Block) string {
	var b strings.Builder
	for i, blk := range blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch v := blk.(type) {
		case ParagraphBlock:
			b.WriteString(inlinesToText(v.Inlines))
		case HeadingBlock:
			b.WriteString(inlinesToText(v.Inlines))
		case BulletsBlock:
			for j, item := range v.Items {
				if j > 0 {
					b.WriteString("\n")
				}
				b.WriteString("• ")
				b.WriteString(inlinesToText(item.Inlines))
			}
		case CodeBlock:
			b.WriteString(v.Source)
		case ColumnsBlock:
			// Tasks 16/17 render columns as separate boxes; here we
			// flatten so the renderer still produces output.
			for ci, col := range v.Columns {
				if ci > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(blocksToPlainText(col))
			}
		case IconRowsBlock:
			for j, row := range v.Rows {
				if j > 0 {
					b.WriteString("\n")
				}
				if v.Kind == "arrows" {
					b.WriteString("→ ")
				} else {
					b.WriteString("• ")
				}
				b.WriteString(row.Text)
			}
		case DiagramBlock:
			// Skipped here; image insertion happens in Task 17.
		}
	}
	return b.String()
}

// CreatePresentationFromMarkdown is the full orchestrator the CLI calls.
// Replaced in Task 18; kept as a stub-with-real-signature here so the
// package still compiles for Tasks 15–17.
func CreatePresentationFromMarkdown(title string, markdown string, service *slides.Service) (*slides.Presentation, error) {
	_ = context.Background // reserved for Task 18
	return nil, fmt.Errorf("not yet wired; see Task 18")
}

// SlidesToAPIRequests is retained as a thin wrapper for any legacy caller.
func SlidesToAPIRequests(in []Slide) ([]*slides.Request, map[int]string) {
	reqs, _ := RenderSlides(in, NewAssetMap(), defaultPageGeometry())
	ids := map[int]string{}
	for i := range in {
		ids[i] = fmt.Sprintf("slide_%d", i+1)
	}
	return reqs, ids
}

func defaultPageGeometry() LayoutGeometry {
	// Standard 16:9 Slides page = 10in x 5.625in = 720pt x 405pt.
	return LayoutGeometry{
		PageWidthPT: 720, PageHeightPT: 405,
		MarginPT: 36, GutterPT: 24, BodyTopPT: 108,
	}
}

func int64Ptr(v int64) *int64 { return &v }

// findColumnsBlock returns the column contents from the first ColumnsBlock,
// padded/truncated to exactly n columns.
func findColumnsBlock(blocks []Block, n int) [][]Block {
	for _, b := range blocks {
		if c, ok := b.(ColumnsBlock); ok {
			out := make([][]Block, n)
			for i := 0; i < n; i++ {
				if i < len(c.Columns) {
					out[i] = c.Columns[i]
				} else {
					out[i] = nil
				}
			}
			return out
		}
	}
	// No explicit ColumnsBlock — split top-level body roughly evenly.
	out := make([][]Block, n)
	for i, b := range blocks {
		out[i%n] = append(out[i%n], b)
	}
	return out
}
