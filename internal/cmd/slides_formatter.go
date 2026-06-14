package cmd

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/slidesmarkdown"
)

const (
	slideLineHeightPT  = 22
	iconImageSizePT    = 18
	iconImageGutterPT  = 24
	diagramVisualLines = 6
	maxDiagramHeightPT = 160
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
func RenderSlides(in []slidesmarkdown.Slide, assets AssetMap, g LayoutGeometry) ([]*slides.Request, []SlideNotesPlan) {
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

		explicitColumnCount := explicitColumnsCount(slide.Body)
		if (layout == LayoutKindDefault || layout == LayoutKindCenter) && explicitColumnCount > 0 {
			if explicitColumnCount == 2 {
				layout = LayoutKindTwoCols
			} else {
				layout = LayoutKindThreeCols
			}
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
			if firstLineLen := utf16CodeUnits(strings.SplitN(text, "\n", 2)[0]); firstLineLen > 0 {
				reqs = append(reqs, &slides.Request{
					UpdateTextStyle: &slides.UpdateTextStyleRequest{
						ObjectId: bodyID,
						TextRange: &slides.Range{
							Type:       "FIXED_RANGE",
							StartIndex: int64Ptr(0),
							EndIndex:   int64Ptr(firstLineLen),
						},
						Style: &slides.TextStyle{
							Bold:     true,
							FontSize: &slides.Dimension{Magnitude: 44, Unit: "PT"},
						},
						Fields: "bold,fontSize",
					},
				})
			}
			if text != "" {
				reqs = append(reqs, &slides.Request{
					UpdateParagraphStyle: &slides.UpdateParagraphStyleRequest{
						ObjectId:  bodyID,
						TextRange: &slides.Range{Type: "ALL"},
						Style:     &slides.ParagraphStyle{Alignment: "CENTER"},
						Fields:    "alignment",
					},
				})
			}
			reqs = appendBlockImages(reqs, slide.Body, slideID, assets, SingleBodyBox(g))
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
				reqs = appendBlockImages(reqs, cols[ci], slideID, assets, boxes[ci])
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
			if layout == LayoutKindCenter && bodyText != "" {
				reqs = append(reqs, &slides.Request{
					UpdateParagraphStyle: &slides.UpdateParagraphStyleRequest{
						ObjectId:  bodyID,
						TextRange: &slides.Range{Type: "ALL"},
						Style:     &slides.ParagraphStyle{Alignment: "CENTER"},
						Fields:    "alignment",
					},
				})
			}
			reqs = appendBlockImages(reqs, slide.Body, slideID, assets, SingleBodyBox(g))
		}

		if slide.Notes != "" {
			notes = append(notes, SlideNotesPlan{SlideIndex: i, SlideID: slideID, Text: slide.Notes})
		}
	}
	return reqs, notes
}

func appendBlockImages(reqs []*slides.Request, blocks []slidesmarkdown.Block, slideID string, assets AssetMap, box BoxRect) []*slides.Request {
	line := 0
	for i, b := range blocks {
		switch v := b.(type) {
		case slidesmarkdown.ParagraphBlock:
			if icon, ok := leadingIcon(v.Inlines); ok {
				if img, ok := assets.Icons[icon]; ok {
					reqs = append(reqs, createIconImageRequest(slideID, img.PublicURL, box, line))
				}
			}
		case slidesmarkdown.HeadingBlock:
			if icon, ok := leadingIcon(v.Inlines); ok {
				if img, ok := assets.Icons[icon]; ok {
					reqs = append(reqs, createIconImageRequest(slideID, img.PublicURL, box, line))
				}
			}
		case slidesmarkdown.DiagramBlock:
			if ir, ok := assets.Diagrams[v.ID]; ok {
				reqs = append(reqs, createImageRequest(slideID, ir.PublicURL, box.LeftPT, box.TopPT+float64(line)*slideLineHeightPT, box.WidthPT, minFloat(box.HeightPT, maxDiagramHeightPT)))
			}
		case slidesmarkdown.BulletsBlock:
			for j, item := range v.Items {
				if len(item.Inlines) == 0 {
					continue
				}
				icon, ok := item.Inlines[0].(slidesmarkdown.IconRef)
				if !ok {
					continue
				}
				if img, ok := assets.Icons[icon]; ok {
					reqs = append(reqs, createIconImageRequest(slideID, img.PublicURL, box, line+j))
				}
			}
		case slidesmarkdown.IconRowsBlock:
			for j, row := range v.Rows {
				if row.Icon == nil {
					continue
				}
				if img, ok := assets.Icons[*row.Icon]; ok {
					reqs = append(reqs, createIconImageRequest(slideID, img.PublicURL, box, line+j))
				}
			}
		case slidesmarkdown.ColumnsBlock:
			// Columns are rendered by the column-layout branch with real box positions.
		}
		line += blockVisualLines(b)
		if i < len(blocks)-1 {
			line++
		}
	}
	return reqs
}

func blockVisualLines(block slidesmarkdown.Block) int {
	switch v := block.(type) {
	case slidesmarkdown.ParagraphBlock:
		return textVisualLines(slidesmarkdown.InlineText(v.Inlines))
	case slidesmarkdown.HeadingBlock:
		return textVisualLines(slidesmarkdown.InlineText(v.Inlines))
	case slidesmarkdown.BulletsBlock:
		return maxInt(1, len(v.Items))
	case slidesmarkdown.CodeBlock:
		return textVisualLines(v.Source)
	case slidesmarkdown.IconRowsBlock:
		return maxInt(1, len(v.Rows))
	case slidesmarkdown.DiagramBlock:
		return diagramVisualLines
	case slidesmarkdown.ColumnsBlock:
		return textVisualLines(blocksToPlainText([]slidesmarkdown.Block{v}))
	default:
		return 1
	}
}

func textVisualLines(s string) int {
	if s == "" {
		return 1
	}
	return strings.Count(s, "\n") + 1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func createImageRequest(slideID, url string, left, top, width, height float64) *slides.Request {
	return &slides.Request{
		CreateImage: &slides.CreateImageRequest{
			Url: url,
			ElementProperties: &slides.PageElementProperties{
				PageObjectId: slideID,
				Transform: &slides.AffineTransform{
					ScaleX: 1, ScaleY: 1,
					TranslateX: left, TranslateY: top,
					Unit: "PT",
				},
				Size: &slides.Size{
					Width:  &slides.Dimension{Magnitude: width, Unit: "PT"},
					Height: &slides.Dimension{Magnitude: height, Unit: "PT"},
				},
			},
		},
	}
}

func createIconImageRequest(slideID, url string, box BoxRect, line int) *slides.Request {
	return createImageRequest(
		slideID,
		url,
		box.LeftPT-iconImageGutterPT,
		box.TopPT+float64(line)*slideLineHeightPT,
		iconImageSizePT,
		iconImageSizePT,
	)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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
// verbatim. Inline icons are skipped; diagrams reserve blank text lines
// so later text does not overlap the CreateImage request.
func blocksToPlainText(blocks []slidesmarkdown.Block) string {
	var b strings.Builder
	for i, blk := range blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch v := blk.(type) {
		case slidesmarkdown.ParagraphBlock:
			b.WriteString(slidesmarkdown.InlineText(v.Inlines))
		case slidesmarkdown.HeadingBlock:
			b.WriteString(slidesmarkdown.InlineText(v.Inlines))
		case slidesmarkdown.BulletsBlock:
			for j, item := range v.Items {
				if j > 0 {
					b.WriteString("\n")
				}
				b.WriteString(strings.Repeat("  ", item.Indent))
				if v.Ordered {
					fmt.Fprintf(&b, "%d. ", j+1)
				} else {
					b.WriteString("• ")
				}
				b.WriteString(slidesmarkdown.InlineText(item.Inlines))
			}
		case slidesmarkdown.CodeBlock:
			b.WriteString(v.Source)
		case slidesmarkdown.ColumnsBlock:
			// Tasks 16/17 render columns as separate boxes; here we
			// flatten so the renderer still produces output.
			for ci, col := range v.Columns {
				if ci > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(blocksToPlainText(col))
			}
		case slidesmarkdown.IconRowsBlock:
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
		case slidesmarkdown.DiagramBlock:
			b.WriteString(strings.Repeat("\n", diagramVisualLines-1))
		}
	}
	return b.String()
}

// CreatePresentationFromMarkdownOptions controls the slidey-aware
// orchestrator. Wired from SlidesCreateFromMarkdownCmd in slides.go.
type CreatePresentationFromMarkdownOptions struct {
	Title         string
	Parent        string
	Slides        []slidesmarkdown.Slide
	SlidesService *slides.Service
	DriveService  *drive.Service
	Pipeline      AssetPipelineConfig
	NoNotes       bool
}

// CreatePresentationFromMarkdownV2 is the slidey orchestrator. It:
//
//  1. Runs the asset pipeline (uploads icons + diagrams to Drive),
//  2. Creates the presentation,
//  3. Reads its page size to derive LayoutGeometry,
//  4. Renders the first BatchUpdate (slides + content + image refs),
//  5. Re-fetches the presentation, finds notes object IDs,
//  6. Renders the second BatchUpdate (speaker notes),
//  7. Cleans up the temp Drive files.
func CreatePresentationFromMarkdownV2(ctx context.Context, opts CreatePresentationFromMarkdownOptions) (*slides.Presentation, error) {
	pipeline := &AssetPipeline{
		Config:   opts.Pipeline,
		Uploader: &DriveUploader{Svc: opts.DriveService},
	}
	defer func() {
		if cleanupErr := pipeline.Cleanup(ctx); cleanupErr != nil {
			fmt.Fprintf(stderrWriter(ctx), "warning: asset cleanup: %v\n", cleanupErr)
		}
	}()

	assets, err := pipeline.Resolve(ctx, opts.Slides)
	if err != nil {
		return nil, fmt.Errorf("resolve assets: %w", err)
	}

	created, err := opts.SlidesService.Presentations.Create(&slides.Presentation{Title: opts.Title}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create presentation: %w", err)
	}

	if opts.Parent != "" && opts.DriveService != nil {
		_, moveErr := opts.DriveService.Files.Update(created.PresentationId, &drive.File{}).
			AddParents(opts.Parent).
			SupportsAllDrives(true).
			Context(ctx).
			Do()
		if moveErr != nil {
			return nil, fmt.Errorf("move to parent: %w", moveErr)
		}
	}

	g := geometryFromPresentation(created)

	mainReqs, notesPlan := buildPopulateRequests(created, opts.Slides, assets, g)
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

func buildPopulateRequests(created *slides.Presentation, in []slidesmarkdown.Slide, assets AssetMap, g LayoutGeometry) ([]*slides.Request, []SlideNotesPlan) {
	mainReqs, notesPlan := RenderSlides(in, assets, g)
	mainReqs = append(mainReqs, deleteExistingSlideRequests(created)...)
	return mainReqs, notesPlan
}

func deleteExistingSlideRequests(p *slides.Presentation) []*slides.Request {
	if p == nil {
		return nil
	}
	reqs := make([]*slides.Request, 0, len(p.Slides))
	for _, s := range p.Slides {
		if s == nil || s.ObjectId == "" {
			continue
		}
		reqs = append(reqs, &slides.Request{
			DeleteObject: &slides.DeleteObjectRequest{ObjectId: s.ObjectId},
		})
	}
	return reqs
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

func buildSlideyDryRunBatchUpdate(slideData []slidesmarkdown.Slide) *slides.BatchUpdatePresentationRequest {
	g := defaultPageGeometry()
	assets := NewAssetMap()
	// Stub asset map: every IconRef gets a placeholder URL; same for diagrams.
	for ref := range collectIconRefs(slideData) {
		assets.Icons[ref] = ImageRef{
			DriveFileID: "dryrun",
			PublicURL:   fmt.Sprintf("gogcli://pending/fa-%s-%s", ref.Style, ref.Name),
		}
	}
	for id := range collectDiagrams(slideData) {
		assets.Diagrams[id] = ImageRef{
			DriveFileID: "dryrun",
			PublicURL:   fmt.Sprintf("gogcli://pending/diagram-%s", id),
		}
	}
	mainReqs, _ := RenderSlides(slideData, assets, g)
	return &slides.BatchUpdatePresentationRequest{Requests: mainReqs}
}

func defaultPageGeometry() LayoutGeometry {
	// Standard 16:9 Slides page = 10in x 5.625in = 720pt x 405pt.
	return LayoutGeometry{
		PageWidthPT: 720, PageHeightPT: 405,
		MarginPT: 36, GutterPT: 24, BodyTopPT: 108,
	}
}

func int64Ptr(v int64) *int64 { return &v }

func utf16CodeUnits(s string) int64 {
	return int64(len(utf16.Encode([]rune(s))))
}

// findColumnsBlock returns the column contents from the first ColumnsBlock,
// padded/truncated to exactly n columns. Surrounding blocks are preserved:
// prefix content is prepended to the first column, suffix content appended to
// the last column.
func findColumnsBlock(blocks []slidesmarkdown.Block, n int) [][]slidesmarkdown.Block {
	out := make([][]slidesmarkdown.Block, n)
	found := false
	for _, b := range blocks {
		if c, ok := b.(slidesmarkdown.ColumnsBlock); ok {
			if !found {
				for i := 0; i < n; i++ {
					if i < len(c.Columns) {
						out[i] = append(out[i], c.Columns[i]...)
					}
				}
				found = true
				continue
			}
		}
		if found {
			out[n-1] = append(out[n-1], b)
		} else {
			out[0] = append(out[0], b)
		}
	}
	if found {
		return out
	}
	// No explicit ColumnsBlock — split top-level body roughly evenly.
	for i, b := range blocks {
		out[i%n] = append(out[i%n], b)
	}
	return out
}

func explicitColumnsCount(blocks []slidesmarkdown.Block) int {
	for _, b := range blocks {
		c, ok := b.(slidesmarkdown.ColumnsBlock)
		if !ok {
			continue
		}
		switch len(c.Columns) {
		case 2:
			return 2
		case 3:
			return 3
		}
	}
	return 0
}
