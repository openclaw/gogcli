package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"
)

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

		// Emit CreateImage for any diagram blocks on this slide.
		for _, b := range slide.Body {
			if d, ok := b.(DiagramBlock); ok {
				if ir, ok := assets.Diagrams[d.ID]; ok {
					reqs = append(reqs, &slides.Request{
						CreateImage: &slides.CreateImageRequest{
							Url: ir.PublicURL,
							ElementProperties: &slides.PageElementProperties{
								PageObjectId: slideID,
								Transform: &slides.AffineTransform{
									ScaleX: 1, ScaleY: 1,
									TranslateX: g.MarginPT, TranslateY: g.BodyTopPT,
									Unit: "PT",
								},
								Size: &slides.Size{
									Width:  &slides.Dimension{Magnitude: g.PageWidthPT - 2*g.MarginPT, Unit: "PT"},
									Height: &slides.Dimension{Magnitude: g.PageHeightPT - g.BodyTopPT - g.MarginPT, Unit: "PT"},
								},
							},
						},
					})
				}
			}
			// Inline icons that lead bullet items: emit a small CreateImage
			// at the left margin of the body area.
			if bb, ok := b.(BulletsBlock); ok {
				for j, item := range bb.Items {
					if len(item.Inlines) == 0 {
						continue
					}
					ir, isIcon := item.Inlines[0].(IconRef)
					if !isIcon {
						continue
					}
					img, ok := assets.Icons[ir]
					if !ok {
						continue
					}
					top := g.BodyTopPT + float64(j)*22.0 // approx 22pt per bullet line
					reqs = append(reqs, &slides.Request{
						CreateImage: &slides.CreateImageRequest{
							Url: img.PublicURL,
							ElementProperties: &slides.PageElementProperties{
								PageObjectId: slideID,
								Transform: &slides.AffineTransform{
									ScaleX: 1, ScaleY: 1,
									TranslateX: g.MarginPT, TranslateY: top,
									Unit: "PT",
								},
								Size: &slides.Size{
									Width:  &slides.Dimension{Magnitude: 18, Unit: "PT"},
									Height: &slides.Dimension{Magnitude: 18, Unit: "PT"},
								},
							},
						},
					})
				}
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

// CreatePresentationFromMarkdownOptions controls the slidey-aware
// orchestrator. Wired from SlidesCreateFromMarkdownCmd in slides.go.
type CreatePresentationFromMarkdownOptions struct {
	Title         string
	Parent        string
	Slides        []Slide
	SlidesService *slides.Service
	DriveService  *drive.Service
	Pipeline      AssetPipelineConfig
	NoNotes       bool
	DryRun        bool
}

// CreatePresentationFromMarkdownV2 is the slidey orchestrator. It:
//
//  1. Creates the presentation,
//  2. Reads its page size to derive LayoutGeometry,
//  3. Runs the asset pipeline (uploads icons + diagrams to Drive),
//  4. Renders the first BatchUpdate (slides + content + image refs),
//  5. Re-fetches the presentation, finds notes object IDs,
//  6. Renders the second BatchUpdate (speaker notes),
//  7. Cleans up the temp Drive files.
func CreatePresentationFromMarkdownV2(ctx context.Context, opts CreatePresentationFromMarkdownOptions) (*slides.Presentation, error) {
	if opts.DryRun {
		return dryRunPresentation(ctx, opts)
	}

	created, err := opts.SlidesService.Presentations.Create(&slides.Presentation{Title: opts.Title}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create presentation: %w", err)
	}

	if opts.Parent != "" && opts.DriveService != nil {
		if _, err := opts.DriveService.Files.Update(created.PresentationId, &drive.File{}).
			AddParents(opts.Parent).Context(ctx).Do(); err != nil {
			return nil, fmt.Errorf("move to parent: %w", err)
		}
	}

	g := geometryFromPresentation(created)

	pipeline := &AssetPipeline{
		Config:   opts.Pipeline,
		Uploader: &DriveUploader{Svc: opts.DriveService},
	}
	defer func() {
		if err := pipeline.Cleanup(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: asset cleanup: %v\n", err)
		}
	}()

	assets, err := pipeline.Resolve(ctx, opts.Slides)
	if err != nil {
		return nil, fmt.Errorf("resolve assets: %w", err)
	}

	mainReqs, notesPlan := RenderSlides(opts.Slides, assets, g)
	if len(mainReqs) > 0 {
		if _, err := opts.SlidesService.Presentations.BatchUpdate(
			created.PresentationId,
			&slides.BatchUpdatePresentationRequest{Requests: mainReqs},
		).Context(ctx).Do(); err != nil {
			return nil, fmt.Errorf("populate slides: %w", err)
		}
	}

	if !opts.NoNotes && len(notesPlan) > 0 {
		populated, err := opts.SlidesService.Presentations.Get(created.PresentationId).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("re-fetch presentation: %w", err)
		}
		notesReqs := buildNotesRequests(populated, notesPlan)
		if len(notesReqs) > 0 {
			if _, err := opts.SlidesService.Presentations.BatchUpdate(
				created.PresentationId,
				&slides.BatchUpdatePresentationRequest{Requests: notesReqs},
			).Context(ctx).Do(); err != nil {
				return nil, fmt.Errorf("apply notes: %w", err)
			}
		}
	}

	return created, nil
}

func geometryFromPresentation(p *slides.Presentation) LayoutGeometry {
	if p == nil || p.PageSize == nil {
		return defaultPageGeometry()
	}
	// Slides PageSize is in EMU; 1pt = 12700 EMU.
	w := float64(p.PageSize.Width.Magnitude) / 12700.0
	h := float64(p.PageSize.Height.Magnitude) / 12700.0
	if p.PageSize.Width.Unit == "PT" {
		w = float64(p.PageSize.Width.Magnitude)
		h = float64(p.PageSize.Height.Magnitude)
	}
	return LayoutGeometry{PageWidthPT: w, PageHeightPT: h, MarginPT: 36, GutterPT: 24, BodyTopPT: 108}
}

func buildNotesRequests(p *slides.Presentation, plan []SlideNotesPlan) []*slides.Request {
	var reqs []*slides.Request
	for _, np := range plan {
		page, _ := findSlidesPageByID(p, np.SlideID)
		if page == nil {
			continue
		}
		notesID := findSpeakerNotesObjectID(page)
		if notesID == "" {
			continue
		}
		// Freshly-created slides have empty notes boxes; a DeleteText{ALL}
		// against an empty box errors out with "startIndex 0 must be less
		// than endIndex 0", so just InsertText.
		if np.Text == "" {
			continue
		}
		reqs = append(reqs, &slides.Request{
			InsertText: &slides.InsertTextRequest{ObjectId: notesID, Text: np.Text},
		})
	}
	return reqs
}

func dryRunPresentation(ctx context.Context, opts CreatePresentationFromMarkdownOptions) (*slides.Presentation, error) {
	g := defaultPageGeometry()
	assets := NewAssetMap()
	// Stub asset map: every IconRef gets a placeholder URL; same for diagrams.
	for ref := range collectIconRefs(opts.Slides) {
		assets.Icons[ref] = ImageRef{
			DriveFileID: "dryrun",
			PublicURL:   fmt.Sprintf("gogcli://pending/fa-%s-%s", ref.Style, ref.Name),
		}
	}
	for id := range collectDiagrams(opts.Slides) {
		assets.Diagrams[id] = ImageRef{
			DriveFileID: "dryrun",
			PublicURL:   fmt.Sprintf("gogcli://pending/diagram-%s", id),
		}
	}
	mainReqs, _ := RenderSlides(opts.Slides, assets, g)
	body := &slides.BatchUpdatePresentationRequest{Requests: mainReqs}
	if err := writeSlidesBatchUpdateDryRun(ctx, body); err != nil {
		return nil, err
	}
	return nil, nil
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
