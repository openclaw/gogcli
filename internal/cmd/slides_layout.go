package cmd

// LayoutKind enumerates the renderer's internal layout categories.
type LayoutKind int

const (
	LayoutKindDefault LayoutKind = iota
	LayoutKindCenter
	LayoutKindSectionHeader // title / hero / statement
	LayoutKindTwoCols
	LayoutKindThreeCols
)

const slideyLayoutTitle = "title"

// MapSlideyLayout maps a slidey frontmatter layout name to a LayoutKind.
// Unknown values fall back to LayoutKindDefault.
func MapSlideyLayout(name string) LayoutKind {
	switch name {
	case "center":
		return LayoutKindCenter
	case slideyLayoutTitle, "hero", "statement":
		return LayoutKindSectionHeader
	case "two-cols":
		return LayoutKindTwoCols
	case "three-cols":
		return LayoutKindThreeCols
	default:
		return LayoutKindDefault
	}
}

// LayoutGeometry holds the per-presentation geometry constants used to
// position text and image boxes. Sizes are in points (PT).
type LayoutGeometry struct {
	PageWidthPT  float64
	PageHeightPT float64
	MarginPT     float64
	GutterPT     float64
	BodyTopPT    float64 // top edge of the body area (below the title)
}

// BoxRect is a positioned rectangle in points.
type BoxRect struct {
	LeftPT, TopPT, WidthPT, HeightPT float64
}

// ColumnBoxes returns N side-by-side body box rectangles using the
// page geometry. Heights are clamped to (pageHeight - bodyTop - margin).
func ColumnBoxes(g LayoutGeometry, n int) []BoxRect {
	if n < 1 {
		return nil
	}
	innerWidth := g.PageWidthPT - 2*g.MarginPT - float64(n-1)*g.GutterPT
	colWidth := innerWidth / float64(n)
	height := g.PageHeightPT - g.BodyTopPT - g.MarginPT

	out := make([]BoxRect, n)
	for i := 0; i < n; i++ {
		out[i] = BoxRect{
			LeftPT:   g.MarginPT + float64(i)*(colWidth+g.GutterPT),
			TopPT:    g.BodyTopPT,
			WidthPT:  colWidth,
			HeightPT: height,
		}
	}
	return out
}

// SingleBodyBox returns one full-width body box at the body-top.
func SingleBodyBox(g LayoutGeometry) BoxRect {
	return BoxRect{
		LeftPT:   g.MarginPT,
		TopPT:    g.BodyTopPT,
		WidthPT:  g.PageWidthPT - 2*g.MarginPT,
		HeightPT: g.PageHeightPT - g.BodyTopPT - g.MarginPT,
	}
}

// TitleBox returns the title-bar box at the top of the slide.
func TitleBox(g LayoutGeometry) BoxRect {
	return BoxRect{
		LeftPT:   g.MarginPT,
		TopPT:    g.MarginPT,
		WidthPT:  g.PageWidthPT - 2*g.MarginPT,
		HeightPT: g.BodyTopPT - g.MarginPT,
	}
}
