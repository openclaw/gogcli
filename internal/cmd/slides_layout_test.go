package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapSlideyLayout(t *testing.T) {
	cases := map[string]LayoutKind{
		"":            LayoutKindDefault,
		"default":     LayoutKindDefault,
		"center":      LayoutKindCenter,
		"title":       LayoutKindSectionHeader,
		"hero":        LayoutKindSectionHeader,
		"statement":   LayoutKindSectionHeader,
		"two-cols":    LayoutKindTwoCols,
		"three-cols":  LayoutKindThreeCols,
		"unknown-lay": LayoutKindDefault,
	}
	for in, want := range cases {
		assert.Equal(t, want, MapSlideyLayout(in), "layout=%q", in)
	}
}

func TestColumnBoxes_TwoColumns(t *testing.T) {
	g := LayoutGeometry{PageWidthPT: 720, PageHeightPT: 405, MarginPT: 36, GutterPT: 24, BodyTopPT: 108}
	boxes := ColumnBoxes(g, 2)
	assert.Equal(t, 2, len(boxes))
	// width = (720 - 2*36 - (2-1)*24) / 2 = (720 - 72 - 24)/2 = 624/2 = 312
	assert.InDelta(t, 36, boxes[0].LeftPT, 0.001)
	assert.InDelta(t, 312, boxes[0].WidthPT, 0.001)
	assert.InDelta(t, 312, boxes[1].WidthPT, 0.001)
	assert.InDelta(t, 36+312+24, boxes[1].LeftPT, 0.001)
}

func TestColumnBoxes_ThreeColumns(t *testing.T) {
	g := LayoutGeometry{PageWidthPT: 720, PageHeightPT: 405, MarginPT: 36, GutterPT: 24, BodyTopPT: 108}
	boxes := ColumnBoxes(g, 3)
	assert.Equal(t, 3, len(boxes))
	// width = (720 - 72 - 48) / 3 = 600/3 = 200
	assert.InDelta(t, 200, boxes[0].WidthPT, 0.001)
	assert.InDelta(t, 200, boxes[1].WidthPT, 0.001)
	assert.InDelta(t, 200, boxes[2].WidthPT, 0.001)
}
