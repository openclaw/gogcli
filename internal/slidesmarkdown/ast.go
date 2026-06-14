package slidesmarkdown

// SlideFrontmatter holds per-slide YAML frontmatter values.
type SlideFrontmatter struct {
	Layout  string            // "title"|"hero"|"center"|"default"|"two-cols"|"three-cols"|"statement"|""
	Content string            // "wide"|"narrow"|"" — parsed but not rendered this PR
	Raw     map[string]string // forward-compat for unknown keys
}

// Slide is the parsed form of one markdown slide. Replaces the legacy
// flat-Element shape used by the original parser.
type Slide struct {
	Frontmatter SlideFrontmatter
	Title       string  // hoisted h1 (or h2 fallback); empty for title/hero/statement layouts
	Body        []Block // ordered top-level blocks
	Notes       string  // resolved speaker-notes text (raw, FA stripped)
}

// Block is a top-level body block.
type Block interface{ isBlock() }

type ParagraphBlock struct {
	Inlines []Inline
}

type BulletItem struct {
	Inlines []Inline
	Indent  int // number of leading 2-space indents (0 = top level)
}

type BulletsBlock struct {
	Items   []BulletItem
	Ordered bool
}

type CodeBlock struct {
	Lang   string
	Source string
}

type HeadingBlock struct {
	Level   int
	Inlines []Inline
}

type ColumnsBlock struct {
	Columns [][]Block // 2 or 3 element outer slice
}

type IconRow struct {
	Icon *IconRef // nil if line had no shortcode
	Text string
}

type IconRowsBlock struct {
	Kind string // "boxes" | "arrows"
	Rows []IconRow
}

type DiagramBlock struct {
	Kind   string // "mermaid" only for now
	Source string
	ID     string // stable ID assigned by the parser; used as AssetMap.Diagrams key
}

func (ParagraphBlock) isBlock() {}
func (BulletsBlock) isBlock()   {}
func (CodeBlock) isBlock()      {}
func (HeadingBlock) isBlock()   {}
func (ColumnsBlock) isBlock()   {}
func (IconRowsBlock) isBlock()  {}
func (DiagramBlock) isBlock()   {}

// Inline is an inline run inside text.
type Inline interface{ isInline() }

type TextRun struct {
	Text   string
	Bold   bool
	Italic bool
	Code   bool
}

// IconRef is an unresolved Font Awesome shortcode (style+name).
// After the asset pipeline runs, an ImageRef is looked up by this value
// from AssetMap.Icons.
type IconRef struct {
	Style string // "solid"|"regular"|"brands"
	Name  string
}

func (TextRun) isInline() {}
func (IconRef) isInline() {}
