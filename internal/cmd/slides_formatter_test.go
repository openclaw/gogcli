package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultGeometry() LayoutGeometry {
	return LayoutGeometry{PageWidthPT: 720, PageHeightPT: 405, MarginPT: 36, GutterPT: 24, BodyTopPT: 108}
}

func TestRenderSlide_DefaultLayout_TitlePlusBody(t *testing.T) {
	s := Slide{
		Title: "Hello",
		Body: []Block{
			ParagraphBlock{Inlines: []Inline{TextRun{Text: "World"}}},
		},
	}
	reqs, _ := RenderSlides([]Slide{s}, NewAssetMap(), defaultGeometry())

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
	s := Slide{Title: "T", Notes: "speaker hint"}
	_, notesPlan := RenderSlides([]Slide{s}, NewAssetMap(), defaultGeometry())

	// notesPlan is a slice of {SlideIndex int, Text string} we feed into
	// the second BatchUpdate after discovering notes object IDs.
	require.Equal(t, 1, len(notesPlan))
	assert.Equal(t, 0, notesPlan[0].SlideIndex)
	assert.Equal(t, "speaker hint", notesPlan[0].Text)
}

func TestRenderSlide_HeroLayoutLargeTitleNoTitleBox(t *testing.T) {
	s := Slide{
		Frontmatter: SlideFrontmatter{Layout: "hero"},
		Body: []Block{
			HeadingBlock{Level: 1, Inlines: []Inline{TextRun{Text: "Big Wordmark"}}},
		},
	}
	reqs, _ := RenderSlides([]Slide{s}, NewAssetMap(), defaultGeometry())

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

func TestRenderSlide_TwoColumnsCreateTwoBodyBoxes(t *testing.T) {
	s := Slide{
		Frontmatter: SlideFrontmatter{Layout: "two-cols"},
		Title:       "T",
		Body: []Block{
			ColumnsBlock{Columns: [][]Block{
				{ParagraphBlock{Inlines: []Inline{TextRun{Text: "left"}}}},
				{ParagraphBlock{Inlines: []Inline{TextRun{Text: "right"}}}},
			}},
		},
	}
	reqs, _ := RenderSlides([]Slide{s}, NewAssetMap(), defaultGeometry())
	// Expect a CreateShape per column (in addition to title shape).
	shapeCount := 0
	for _, r := range reqs {
		if r.CreateShape != nil {
			shapeCount++
		}
	}
	assert.GreaterOrEqual(t, shapeCount, 3, "title + 2 column body boxes")
}

func TestRenderSlide_ThreeColumnsCreateThreeBodyBoxes(t *testing.T) {
	s := Slide{
		Frontmatter: SlideFrontmatter{Layout: "three-cols"},
		Title:       "T",
		Body: []Block{
			ColumnsBlock{Columns: [][]Block{
				{ParagraphBlock{Inlines: []Inline{TextRun{Text: "A"}}}},
				{ParagraphBlock{Inlines: []Inline{TextRun{Text: "B"}}}},
				{ParagraphBlock{Inlines: []Inline{TextRun{Text: "C"}}}},
			}},
		},
	}
	reqs, _ := RenderSlides([]Slide{s}, NewAssetMap(), defaultGeometry())
	shapeCount := 0
	for _, r := range reqs {
		if r.CreateShape != nil {
			shapeCount++
		}
	}
	assert.GreaterOrEqual(t, shapeCount, 4, "title + 3 column body boxes")
}
