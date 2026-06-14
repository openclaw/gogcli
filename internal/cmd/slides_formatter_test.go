package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/slides/v1"

	sm "github.com/steipete/gogcli/internal/slidesmarkdown"
)

func defaultGeometry() LayoutGeometry {
	return LayoutGeometry{PageWidthPT: 720, PageHeightPT: 405, MarginPT: 36, GutterPT: 24, BodyTopPT: 108}
}

func TestRenderSlide_DefaultLayout_TitlePlusBody(t *testing.T) {
	s := sm.Slide{
		Title: "Hello",
		Body: []sm.Block{
			sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "World"}}},
		},
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())

	// Expect: CreateSlide, CreateShape (title), InsertText (title),
	// UpdateTextStyle (title bold), CreateShape (body), InsertText (body).
	require.GreaterOrEqual(t, len(reqs), 6)
	assert.NotNil(t, reqs[0].CreateSlide)
	// Find at least one InsertText with "Hello" and one with "World".
	var sawHello, sawWorld bool
	for _, r := range reqs {
		if r.InsertText != nil {
			if r.InsertText.Text == "Hello" {
				sawHello = true
			}
			if r.InsertText.Text == "World" {
				sawWorld = true
			}
		}
	}
	assert.True(t, sawHello)
	assert.True(t, sawWorld)
}

func TestRenderSlide_NotesRequestsReturned(t *testing.T) {
	s := sm.Slide{Title: "T", Notes: "speaker hint"}
	_, notesPlan := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())

	// notesPlan is a slice of {SlideIndex int, Text string} we feed into
	// the second BatchUpdate after discovering notes object IDs.
	require.Equal(t, 1, len(notesPlan))
	assert.Equal(t, 0, notesPlan[0].SlideIndex)
	assert.Equal(t, "speaker hint", notesPlan[0].Text)
}

func TestRenderSlide_HeroLayoutLargeTitleNoTitleBox(t *testing.T) {
	s := sm.Slide{
		Frontmatter: sm.SlideFrontmatter{Layout: "hero"},
		Body: []sm.Block{
			sm.HeadingBlock{Level: 1, Inlines: []sm.Inline{sm.TextRun{Text: "Big Wordmark"}}},
		},
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())

	// No separate title text box — find the body insert and the 44pt style.
	var sawLargeStyle bool
	for _, r := range reqs {
		if r.UpdateTextStyle != nil && r.UpdateTextStyle.Style != nil &&
			r.UpdateTextStyle.Style.FontSize != nil &&
			r.UpdateTextStyle.Style.FontSize.Magnitude == 44 {
			sawLargeStyle = true
		}
	}
	assert.True(t, sawLargeStyle, "hero h1 should be styled at 44pt")
}

func TestRenderSlide_CenterLayoutWithOnlyTitleDoesNotStyleEmptyBody(t *testing.T) {
	s := sm.Slide{
		Frontmatter: sm.SlideFrontmatter{Layout: "center"},
		Title:       "Only title",
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())

	for _, r := range reqs {
		if r.UpdateParagraphStyle != nil && r.UpdateParagraphStyle.ObjectId == "body_1" {
			t.Fatal("must not style an empty body text box")
		}
	}
}

func TestRenderSlide_HeroStyleRangeUsesUTF16(t *testing.T) {
	s := sm.Slide{
		Frontmatter: sm.SlideFrontmatter{Layout: "hero"},
		Body: []sm.Block{
			sm.HeadingBlock{Level: 1, Inlines: []sm.Inline{sm.TextRun{Text: "A 🐢"}}},
		},
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())

	for _, r := range reqs {
		if r.UpdateTextStyle != nil && r.UpdateTextStyle.TextRange != nil &&
			r.UpdateTextStyle.TextRange.Type == "FIXED_RANGE" {
			require.NotNil(t, r.UpdateTextStyle.TextRange.EndIndex)
			assert.Equal(t, int64(4), *r.UpdateTextStyle.TextRange.EndIndex)
			return
		}
	}
	t.Fatal("expected fixed-range hero text style")
}

func TestRenderSlide_TwoColumnsCreateTwoBodyBoxes(t *testing.T) {
	s := sm.Slide{
		Frontmatter: sm.SlideFrontmatter{Layout: "two-cols"},
		Title:       "T",
		Body: []sm.Block{
			sm.ColumnsBlock{Columns: [][]sm.Block{
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "left"}}}},
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "right"}}}},
			}},
		},
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())
	// Expect a CreateShape per column (in addition to title shape).
	shapeCount := 0
	for _, r := range reqs {
		if r.CreateShape != nil {
			shapeCount++
		}
	}
	assert.GreaterOrEqual(t, shapeCount, 3, "title + 2 column body boxes")
}

func TestRenderSlide_ExplicitColumnsWithoutLayoutCreateColumnBoxes(t *testing.T) {
	s := sm.Slide{
		Title: "T",
		Body: []sm.Block{
			sm.ColumnsBlock{Columns: [][]sm.Block{
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "left"}}}},
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "right"}}}},
			}},
		},
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())

	var columnShapes []string
	for _, r := range reqs {
		if r.CreateShape != nil && strings.Contains(r.CreateShape.ObjectId, "_col") {
			columnShapes = append(columnShapes, r.CreateShape.ObjectId)
		}
	}
	assert.ElementsMatch(t, []string{"body_1_col1", "body_1_col2"}, columnShapes)
}

func TestRenderSlide_ThreeColumnsCreateThreeBodyBoxes(t *testing.T) {
	s := sm.Slide{
		Frontmatter: sm.SlideFrontmatter{Layout: "three-cols"},
		Title:       "T",
		Body: []sm.Block{
			sm.ColumnsBlock{Columns: [][]sm.Block{
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "A"}}}},
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "B"}}}},
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "C"}}}},
			}},
		},
	}
	reqs, _ := RenderSlides([]sm.Slide{s}, NewAssetMap(), defaultGeometry())
	shapeCount := 0
	for _, r := range reqs {
		if r.CreateShape != nil {
			shapeCount++
		}
	}
	assert.GreaterOrEqual(t, shapeCount, 4, "title + 3 column body boxes")
}

func TestFindColumnsBlock_PreservesSurroundingContent(t *testing.T) {
	got := findColumnsBlock([]sm.Block{
		sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "Intro"}}},
		sm.ColumnsBlock{Columns: [][]sm.Block{
			{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "Left"}}}},
			{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "Right"}}}},
		}},
		sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "After"}}},
	}, 2)

	require.Equal(t, 2, len(got))
	assert.Equal(t, "Intro\n\nLeft", blocksToPlainText(got[0]))
	assert.Equal(t, "Right\n\nAfter", blocksToPlainText(got[1]))
}

func TestBuildPopulateRequests_DeleteDefaultSlideAfterCreatedSlides(t *testing.T) {
	reqs, _ := buildPopulateRequests(
		&slides.Presentation{Slides: []*slides.Page{{ObjectId: "default-slide"}}},
		[]sm.Slide{{Title: "Imported"}},
		NewAssetMap(),
		defaultGeometry(),
	)

	require.NotEmpty(t, reqs)
	assert.NotNil(t, reqs[0].CreateSlide)
	require.NotNil(t, reqs[len(reqs)-1].DeleteObject)
	assert.Equal(t, "default-slide", reqs[len(reqs)-1].DeleteObject.ObjectId)
}

func TestRenderSlide_DiagramEmitsCreateImage(t *testing.T) {
	bid := "block-test-1"
	s := sm.Slide{
		Title: "T",
		Body:  []sm.Block{sm.DiagramBlock{Kind: "mermaid", Source: "graph TD\nA-->B", ID: bid}},
	}
	am := NewAssetMap()
	am.Diagrams[bid] = ImageRef{DriveFileID: "f1", PublicURL: "https://drive.example/f1"}

	reqs, _ := RenderSlides([]sm.Slide{s}, am, defaultGeometry())
	var sawImage bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f1" {
			sawImage = true
		}
	}
	assert.True(t, sawImage)
}

func TestBlocksToPlainText_ReservesDiagramSpace(t *testing.T) {
	got := blocksToPlainText([]sm.Block{
		sm.DiagramBlock{Kind: "mermaid", Source: "graph TD\nA-->B", ID: "diagram-1"},
		sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "After"}}},
	})

	assert.Equal(t, strings.Repeat("\n", diagramVisualLines+1)+"After", got)
}

func TestRenderSlide_BulletWithLeadingIconEmitsImage(t *testing.T) {
	icon := sm.IconRef{Style: "solid", Name: "truck-fast"}
	s := sm.Slide{
		Title: "T",
		Body: []sm.Block{
			sm.BulletsBlock{Items: []sm.BulletItem{
				{Inlines: []sm.Inline{icon, sm.TextRun{Text: " Fulfilment"}}},
			}},
		},
	}
	am := NewAssetMap()
	am.Icons[icon] = ImageRef{DriveFileID: "f2", PublicURL: "https://drive.example/f2"}

	reqs, _ := RenderSlides([]sm.Slide{s}, am, defaultGeometry())
	var sawIcon bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f2" {
			sawIcon = true
			assert.Less(t, r.CreateImage.ElementProperties.Transform.TranslateX, SingleBodyBox(defaultGeometry()).LeftPT)
		}
	}
	assert.True(t, sawIcon)
}

func TestBlocksToPlainText_PreservesOrderedAndNestedLists(t *testing.T) {
	got := blocksToPlainText([]sm.Block{
		sm.BulletsBlock{Ordered: true, Items: []sm.BulletItem{
			{Inlines: []sm.Inline{sm.TextRun{Text: "first"}}},
			{Indent: 1, Inlines: []sm.Inline{sm.TextRun{Text: "second"}}},
		}},
		sm.BulletsBlock{Items: []sm.BulletItem{
			{Indent: 2, Inlines: []sm.Inline{sm.TextRun{Text: "nested"}}},
		}},
	})

	assert.Equal(t, "1. first\n  2. second\n\n    • nested", got)
}

func TestRenderSlide_IconRowsEmitImages(t *testing.T) {
	icon := sm.IconRef{Style: "solid", Name: "headset"}
	s := sm.Slide{
		Title: "T",
		Body: []sm.Block{
			sm.IconRowsBlock{Kind: "boxes", Rows: []sm.IconRow{{Icon: &icon, Text: "Support"}}},
		},
	}
	am := NewAssetMap()
	am.Icons[icon] = ImageRef{DriveFileID: "f3", PublicURL: "https://drive.example/f3"}

	reqs, _ := RenderSlides([]sm.Slide{s}, am, defaultGeometry())
	var sawIcon bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f3" {
			sawIcon = true
			assert.Less(t, r.CreateImage.ElementProperties.Transform.TranslateX, SingleBodyBox(defaultGeometry()).LeftPT)
		}
	}
	assert.True(t, sawIcon)
}

func TestRenderSlide_HeadingLeadingIconEmitsImage(t *testing.T) {
	icon := sm.IconRef{Style: "solid", Name: "file"}
	s := sm.Slide{
		Title: "T",
		Body: []sm.Block{
			sm.HeadingBlock{Level: 2, Inlines: []sm.Inline{icon, sm.TextRun{Text: " Rethink"}}},
		},
	}
	am := NewAssetMap()
	am.Icons[icon] = ImageRef{DriveFileID: "f5", PublicURL: "https://drive.example/f5"}

	reqs, _ := RenderSlides([]sm.Slide{s}, am, defaultGeometry())
	var sawIcon bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f5" {
			sawIcon = true
		}
	}
	assert.True(t, sawIcon)
}

func TestRenderSlide_IconImagePositionAccountsForBlankLinesBetweenBlocks(t *testing.T) {
	icon := sm.IconRef{Style: "solid", Name: "file"}
	s := sm.Slide{
		Title: "T",
		Body: []sm.Block{
			sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "Intro"}}},
			sm.HeadingBlock{Level: 2, Inlines: []sm.Inline{icon, sm.TextRun{Text: " Rethink"}}},
		},
	}
	am := NewAssetMap()
	am.Icons[icon] = ImageRef{DriveFileID: "f5", PublicURL: "https://drive.example/f5"}

	reqs, _ := RenderSlides([]sm.Slide{s}, am, defaultGeometry())
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f5" {
			assert.Equal(t, float64(152), r.CreateImage.ElementProperties.Transform.TranslateY)
			return
		}
	}
	t.Fatal("expected icon image request")
}

func TestRenderSlide_ColumnDiagramEmitsCreateImage(t *testing.T) {
	bid := "block-column-1"
	s := sm.Slide{
		Frontmatter: sm.SlideFrontmatter{Layout: "two-cols"},
		Title:       "T",
		Body: []sm.Block{
			sm.ColumnsBlock{Columns: [][]sm.Block{
				{sm.ParagraphBlock{Inlines: []sm.Inline{sm.TextRun{Text: "left"}}}},
				{sm.DiagramBlock{Kind: "mermaid", Source: "graph TD\nA-->B", ID: bid}},
			}},
		},
	}
	am := NewAssetMap()
	am.Diagrams[bid] = ImageRef{DriveFileID: "f4", PublicURL: "https://drive.example/f4"}

	reqs, _ := RenderSlides([]sm.Slide{s}, am, defaultGeometry())
	var sawImage bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f4" {
			sawImage = true
		}
	}
	assert.True(t, sawImage)
}

func TestDeleteExistingSlideRequests(t *testing.T) {
	reqs := deleteExistingSlideRequests(&slides.Presentation{Slides: []*slides.Page{
		{ObjectId: "p"},
		nil,
		{ObjectId: "slide_existing"},
	}})

	require.Equal(t, 2, len(reqs))
	assert.Equal(t, "p", reqs[0].DeleteObject.ObjectId)
	assert.Equal(t, "slide_existing", reqs[1].DeleteObject.ObjectId)
}
