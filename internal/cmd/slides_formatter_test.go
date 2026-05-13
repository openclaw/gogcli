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
