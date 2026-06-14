package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/steipete/gogcli/internal/slidesmarkdown"
)

func TestSlideyFixture_ParsesAndRenders(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "slidey", "index.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	parsed, err := slidesmarkdown.Parse(string(data), slidesmarkdown.ParseOptions{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(parsed), 30, "fixture should produce ~30+ slides")

	// At least one hero/title/statement, one two-cols, one three-cols.
	var sawHero, sawTwoCols, sawThreeCols, sawNotes, sawIcon, sawDiagram bool
	for _, s := range parsed {
		switch s.Frontmatter.Layout {
		case "hero", "title", "statement":
			sawHero = true
		case "two-cols":
			sawTwoCols = true
		case "three-cols":
			sawThreeCols = true
		}
		if s.Notes != "" {
			sawNotes = true
		}
		var walk func([]slidesmarkdown.Block)
		walk = func(blocks []slidesmarkdown.Block) {
			for _, b := range blocks {
				switch v := b.(type) {
				case slidesmarkdown.ParagraphBlock:
					for _, in := range v.Inlines {
						if _, ok := in.(slidesmarkdown.IconRef); ok {
							sawIcon = true
						}
					}
				case slidesmarkdown.BulletsBlock:
					for _, item := range v.Items {
						for _, in := range item.Inlines {
							if _, ok := in.(slidesmarkdown.IconRef); ok {
								sawIcon = true
							}
						}
					}
				case slidesmarkdown.IconRowsBlock:
					for _, row := range v.Rows {
						if row.Icon != nil {
							sawIcon = true
						}
					}
				case slidesmarkdown.ColumnsBlock:
					for _, col := range v.Columns {
						walk(col)
					}
				case slidesmarkdown.DiagramBlock:
					sawDiagram = true
				}
			}
		}
		walk(s.Body)
	}
	assert.True(t, sawHero, "fixture should contain a hero/title/statement slide")
	assert.True(t, sawTwoCols, "fixture should contain a two-cols slide")
	assert.True(t, sawThreeCols, "fixture should contain a three-cols slide")
	assert.True(t, sawNotes, "fixture should contain ## Notes sections")
	assert.True(t, sawIcon, "fixture should contain FA shortcodes")
	assert.True(t, sawDiagram, "fixture should contain mermaid blocks")

	// Renderer should produce a non-empty BatchUpdate plan with a fake asset map.
	am := NewAssetMap()
	for ref := range collectIconRefs(parsed) {
		am.Icons[ref] = ImageRef{DriveFileID: "x", PublicURL: "https://example/x"}
	}
	for id := range collectDiagrams(parsed) {
		am.Diagrams[id] = ImageRef{DriveFileID: "y", PublicURL: "https://example/y"}
	}
	reqs, notes := RenderSlides(parsed, am, defaultPageGeometry())
	assert.NotEmpty(t, reqs)
	assert.NotEmpty(t, notes)
	_ = context.Background() // reserved for future
}
