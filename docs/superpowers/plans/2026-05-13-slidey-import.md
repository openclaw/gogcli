# Slidey-flavored markdown import — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `gog slides create-from-markdown` to import slidey-flavored markdown decks (per-slide YAML frontmatter, `## Notes` speaker notes, Font Awesome icon shortcodes, mermaid diagrams, multi-column layouts, `::boxes::` / `::arrows::` blocks).

**Architecture:** Two-pass design: a pure parser turns markdown into a typed `Slide` AST; an asset pipeline fetches FA SVGs from jsDelivr and renders mermaid via local `mmdc`, uploading both to Drive; a renderer turns `(AST, asset map)` → `[]*slides.Request`. The CLI calls all three with new flags for FA/mmdc behavior.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, `google.golang.org/api/slides/v1`, `google.golang.org/api/drive/v3`, standard library `net/http` for jsDelivr, `os/exec` for `mmdc`. Tests use `github.com/stretchr/testify`.

**Spec:** `docs/superpowers/specs/2026-05-13-slidey-import-design.md` (read this first).

---

## File map

**New files:**

| Path | Purpose |
|------|---------|
| `internal/cmd/slides_markdown_ast.go` | AST type definitions (Slide, Block, Inline, IconRef, ImageRef). |
| `internal/cmd/slides_markdown_frontmatter.go` | Per-slide YAML frontmatter splitter and parser. |
| `internal/cmd/slides_markdown_blocks.go` | Body block parsing (paragraph, bullets, code, columns, boxes/arrows, mermaid). |
| `internal/cmd/slides_markdown_inlines.go` | Inline parser for FA shortcodes and emphasis. |
| `internal/cmd/slides_layout.go` | Layout enum + geometry math for column boxes. |
| `internal/cmd/slides_assets.go` | Asset pipeline (FA fetch + mmdc + Drive upload + cleanup). |
| `internal/cmd/slides_markdown_test.go` | Parser end-to-end tests (orchestrator). |
| `internal/cmd/slides_markdown_frontmatter_test.go` | Frontmatter splitter/parser tests. |
| `internal/cmd/slides_markdown_blocks_test.go` | Body block parser tests. |
| `internal/cmd/slides_markdown_inlines_test.go` | Inline parser tests. |
| `internal/cmd/slides_layout_test.go` | Layout helpers tests. |
| `internal/cmd/slides_assets_test.go` | Asset pipeline tests (no live network). |
| `internal/cmd/slides_formatter_test.go` | Renderer tests with fake asset map. |
| `testdata/slidey/index.md` | Fixture deck (copy of `../univrs/slidey/slides/index.md`). |

**Modified files:**

| Path | Change |
|------|--------|
| `internal/cmd/slides_markdown.go` | Replace parser entrypoint; keep `ParseMarkdownToSlides` exported. |
| `internal/cmd/slides_formatter.go` | Replace `SlidesToAPIRequests` and `CreatePresentationFromMarkdown`. |
| `internal/cmd/slides.go` | Add new flags to `SlidesCreateFromMarkdownCmd`; wire into command. |
| `docs/slides-markdown.md` | Document new grammar. |
| `docs/commands/gog-slides-create-from-markdown.md` | Flag table update. |
| `CHANGELOG.md` | Entry under 0.17.0 / Unreleased. |

---

## Coding conventions for this plan

- **TDD strictly.** For every behavior, write the failing test first, then the minimal code to make it pass. Run the tests between every step.
- **Run tests with:** `go test ./internal/cmd/... -run <TestName>` for focused runs; `go test ./internal/cmd/...` for the full package.
- **Run vet/build between tasks:** `go build ./... && go vet ./...` to catch breakage early.
- **Commit cadence:** one commit per task using the existing repo style: `feat(slides): ...`, `test(slides): ...`, `docs(slides): ...`. Use the heredoc commit pattern from `AGENTS.md` (no `git add -A`).
- **Test framework:** the repo already uses `github.com/stretchr/testify/require`. Use `require` for setup invariants and `assert` for behavioral assertions.
- **Service injection:** existing pattern is package-level vars (`var newSlidesService = googleapi.NewSlides`). Mock by reassignment in tests; restore with `t.Cleanup`.
- **No `interface{}` boxing for AST nodes** — the spec uses an `isBlock()` / `isInline()` marker-method pattern. Implement with empty receiver methods.

---

## Task 1: Add AST types (compile-only scaffold)

**Spec coverage:** §5 (Internal AST).

**Files:**
- Create: `internal/cmd/slides_markdown_ast.go`
- Test: `internal/cmd/slides_markdown_ast_test.go`

- [ ] **Step 1: Write the failing test that pins the marker-method discipline**

`internal/cmd/slides_markdown_ast_test.go`:

```go
package cmd

import "testing"

func TestBlockMarkerMethods(t *testing.T) {
	var _ Block = ParagraphBlock{}
	var _ Block = BulletsBlock{}
	var _ Block = CodeBlock{}
	var _ Block = HeadingBlock{}
	var _ Block = ColumnsBlock{}
	var _ Block = IconRowsBlock{}
	var _ Block = DiagramBlock{}

	var _ Inline = TextRun{}
	var _ Inline = IconRef{}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `go test ./internal/cmd/ -run TestBlockMarkerMethods`
Expected: FAIL with "undefined: Block" or similar compile error.

- [ ] **Step 3: Add the AST types**

`internal/cmd/slides_markdown_ast.go`:

```go
package cmd

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
	BlockID string // stable ID assigned by the parser; used as AssetMap key
}

func (ParagraphBlock) isBlock()  {}
func (BulletsBlock) isBlock()    {}
func (CodeBlock) isBlock()       {}
func (HeadingBlock) isBlock()    {}
func (ColumnsBlock) isBlock()    {}
func (IconRowsBlock) isBlock()   {}
func (DiagramBlock) isBlock()    {}

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

// ImageRef is the result of uploading an asset (icon SVG or rendered
// diagram PNG) to Drive.
type ImageRef struct {
	DriveFileID string
	PublicURL   string
}
```

**Note:** the existing `SlideElement`, `SlideLayout`, and constants `LayoutTitleOnly`/etc. in `slides_markdown.go` will be replaced in Task 3. For now they coexist; this file only adds new types.

- [ ] **Step 4: Run test to verify it compiles and passes**

Run: `go build ./... && go test ./internal/cmd/ -run TestBlockMarkerMethods`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_ast.go internal/cmd/slides_markdown_ast_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add slidey AST type definitions

Introduces Slide, Block, Inline, IconRef, ImageRef and per-block types.
Coexists with the legacy SlideElement parser until Task 3 swaps it out.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Per-slide frontmatter splitter

**Spec coverage:** §4.1 (per-slide frontmatter, disambiguation rule).

**Files:**
- Create: `internal/cmd/slides_markdown_frontmatter.go`
- Test: `internal/cmd/slides_markdown_frontmatter_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/cmd/slides_markdown_frontmatter_test.go`:

```go
package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitMarkdownIntoSlideBlocks(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []slideBlock
	}{
		{
			name:  "single slide no frontmatter",
			input: "# Hello\n\nbody\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# Hello\n\nbody\n"},
			},
		},
		{
			name:  "two slides separated by ---",
			input: "# A\n\n---\n\n# B\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# B\n"},
			},
		},
		{
			name:  "leading frontmatter then content",
			input: "---\nlayout: hero\n---\n\n# Title\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Layout: "hero", Raw: map[string]string{"layout": "hero"}}, Body: "# Title\n"},
			},
		},
		{
			name:  "frontmatter on second slide",
			input: "# A\n\n---\nlayout: center\n---\n\n# B\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
				{Frontmatter: SlideFrontmatter{Layout: "center", Raw: map[string]string{"layout": "center"}}, Body: "# B\n"},
			},
		},
		{
			name:  "frontmatter with content key",
			input: "---\nlayout: center\ncontent: wide\n---\n\nbody\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{
					Layout:  "center",
					Content: "wide",
					Raw:     map[string]string{"layout": "center", "content": "wide"},
				}, Body: "body\n"},
			},
		},
		{
			name:  "bare --- at slide start is separator not frontmatter",
			input: "# A\n\n---\n\nplain text body\n",
			expected: []slideBlock{
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "# A\n"},
				{Frontmatter: SlideFrontmatter{Raw: map[string]string{}}, Body: "plain text body\n"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitMarkdownIntoSlideBlocks(tc.input)
			require.NoError(t, err)
			require.Equal(t, len(tc.expected), len(got))
			for i := range tc.expected {
				assert.Equal(t, tc.expected[i].Frontmatter, got[i].Frontmatter, "slide %d frontmatter", i)
				assert.Equal(t, tc.expected[i].Body, got[i].Body, "slide %d body", i)
			}
		})
	}
}

func TestSplitMarkdownIntoSlideBlocks_UnclosedFrontmatter(t *testing.T) {
	_, err := splitMarkdownIntoSlideBlocks("---\nlayout: hero\n\n# never closed\n")
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "frontmatter")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestSplitMarkdownIntoSlideBlocks`
Expected: FAIL with "undefined: splitMarkdownIntoSlideBlocks" or "undefined: slideBlock".

- [ ] **Step 3: Implement the splitter**

`internal/cmd/slides_markdown_frontmatter.go`:

```go
package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// slideBlock is the intermediate form between raw markdown and the parsed
// Slide AST: per-slide frontmatter + the raw body markdown for that slide.
type slideBlock struct {
	Frontmatter SlideFrontmatter
	Body        string
}

var yamlKeyLineRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*:\s`)

// splitMarkdownIntoSlideBlocks walks markdown line by line, splits on bare
// "---" separators, and detects per-slide frontmatter using the rule from
// the design spec (§4.1):
//
//   1. A "---" at file start, or immediately following another "---" separator
//      (only blank lines between), opens a frontmatter candidate.
//   2. The next non-blank line must match a YAML key (^[A-Za-z_][\w-]*:\s).
//      If not, the original "---" is a separator and the candidate is abandoned.
//   3. Scan forward; the first line that trims to "---" closes the frontmatter.
//      No closing → fatal error.
func splitMarkdownIntoSlideBlocks(markdown string) ([]slideBlock, error) {
	lines := strings.Split(markdown, "\n")
	var blocks []slideBlock

	i := 0
	for i < len(lines) {
		// Try to consume a frontmatter block at the current position.
		fm, after, ok, err := tryConsumeFrontmatter(lines, i)
		if err != nil {
			return nil, err
		}
		if ok {
			i = after
		} else {
			fm = SlideFrontmatter{Raw: map[string]string{}}
		}

		// Consume body lines until the next bare "---" separator or EOF.
		bodyStart := i
		for i < len(lines) {
			if isBareDelimiter(lines[i]) {
				break
			}
			i++
		}
		bodyLines := lines[bodyStart:i]
		body := strings.Join(bodyLines, "\n")
		blocks = append(blocks, slideBlock{Frontmatter: fm, Body: body})

		// Skip the separator "---" line.
		if i < len(lines) && isBareDelimiter(lines[i]) {
			i++
		}
		// Skip blank lines after the separator.
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
	}

	return blocks, nil
}

func tryConsumeFrontmatter(lines []string, start int) (SlideFrontmatter, int, bool, error) {
	// Skip leading blank lines.
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || !isBareDelimiter(lines[i]) {
		return SlideFrontmatter{}, start, false, nil
	}

	// First non-blank line after "---" must look like a YAML key.
	j := i + 1
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}
	if j >= len(lines) || !yamlKeyLineRE.MatchString(lines[j]) {
		return SlideFrontmatter{}, start, false, nil
	}

	// Find closing "---".
	closeIdx := -1
	for k := j; k < len(lines); k++ {
		if isBareDelimiter(lines[k]) {
			closeIdx = k
			break
		}
	}
	if closeIdx == -1 {
		return SlideFrontmatter{}, start, false, fmt.Errorf("unclosed frontmatter starting at line %d", i+1)
	}

	yamlText := strings.Join(lines[i+1:closeIdx], "\n")
	fm, err := parseSlideFrontmatter(yamlText)
	if err != nil {
		return SlideFrontmatter{}, start, false, fmt.Errorf("frontmatter at line %d: %w", i+1, err)
	}
	return fm, closeIdx + 1, true, nil
}

func parseSlideFrontmatter(yamlText string) (SlideFrontmatter, error) {
	raw := map[string]string{}
	if strings.TrimSpace(yamlText) != "" {
		// yaml.v3 into a flat map (all values stringified).
		var m map[string]any
		if err := yaml.Unmarshal([]byte(yamlText), &m); err != nil {
			return SlideFrontmatter{}, err
		}
		for k, v := range m {
			raw[k] = fmt.Sprintf("%v", v)
		}
	}
	return SlideFrontmatter{
		Layout:  raw["layout"],
		Content: raw["content"],
		Raw:     raw,
	}, nil
}

func isBareDelimiter(line string) bool {
	return strings.TrimSpace(line) == literalMarkdownTripleDash
}
```

**Note:** `literalMarkdownTripleDash` is already defined in the package (used by `slides_markdown.go` and `drive_markdown_frontmatter.go`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestSplitMarkdownIntoSlideBlocks -v`
Expected: PASS for all 7 sub-tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_frontmatter.go internal/cmd/slides_markdown_frontmatter_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add per-slide frontmatter splitter

Implements the §4.1 disambiguation rule: a "---" opens frontmatter only
when followed by a YAML key line; otherwise it is a slide separator.
Unclosed frontmatter is a fatal error with a line-numbered message.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Inline parser (FA shortcodes + emphasis)

**Spec coverage:** §4.6 (Font Awesome shortcodes), partial §5 (Inlines).

**Files:**
- Create: `internal/cmd/slides_markdown_inlines.go`
- Test: `internal/cmd/slides_markdown_inlines_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/cmd/slides_markdown_inlines_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseInlines_PlainText(t *testing.T) {
	got := parseInlines("hello world", "solid")
	assert.Equal(t, []Inline{TextRun{Text: "hello world"}}, got)
}

func TestParseInlines_Emphasis(t *testing.T) {
	got := parseInlines("plain **bold** _ital_ `code` end", "solid")
	assert.Equal(t, []Inline{
		TextRun{Text: "plain "},
		TextRun{Text: "bold", Bold: true},
		TextRun{Text: " "},
		TextRun{Text: "ital", Italic: true},
		TextRun{Text: " "},
		TextRun{Text: "code", Code: true},
		TextRun{Text: " end"},
	}, got)
}

func TestParseInlines_FAShortcodes(t *testing.T) {
	got := parseInlines("Welcome :fa-truck-fast: to :fab-github: here", "solid")
	assert.Equal(t, []Inline{
		TextRun{Text: "Welcome "},
		IconRef{Style: "solid", Name: "truck-fast"},
		TextRun{Text: " to "},
		IconRef{Style: "brands", Name: "github"},
		TextRun{Text: " here"},
	}, got)
}

func TestParseInlines_FAStyleDerivation(t *testing.T) {
	cases := []struct {
		shortcode      string
		defaultStyle   string
		expectedStyle  string
		expectedName   string
	}{
		{":fa-database:", "solid", "solid", "database"},
		{":fas-headset:", "solid", "solid", "headset"},
		{":far-clock:", "solid", "regular", "clock"},
		{":fab-github:", "solid", "brands", "github"},
		{":fal-flask:", "solid", "solid", "flask"}, // free-tier substitution
		{":fad-bug:", "solid", "solid", "bug"},     // free-tier substitution
		{":fa-database:", "regular", "regular", "database"}, // default override
	}
	for _, tc := range cases {
		t.Run(tc.shortcode, func(t *testing.T) {
			got := parseInlines(tc.shortcode, tc.defaultStyle)
			assert.Equal(t, []Inline{IconRef{Style: tc.expectedStyle, Name: tc.expectedName}}, got)
		})
	}
}

func TestStripFAShortcodes(t *testing.T) {
	got := stripFAShortcodes(":fa-truck-fast: Orders and :fab-github: GitHub")
	assert.Equal(t, "Orders and  GitHub", got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestParseInlines|TestStripFAShortcodes"`
Expected: FAIL with undefined `parseInlines` / `stripFAShortcodes`.

- [ ] **Step 3: Implement the inline parser**

`internal/cmd/slides_markdown_inlines.go`:

```go
package cmd

import (
	"regexp"
	"strings"
)

// faShortcodeRE matches :fa-name:, :fas-name:, :far-name:, :fab-name:,
// :fal-name:, :fad-name:.
var faShortcodeRE = regexp.MustCompile(`:fa([srlbd])?-([a-z0-9][a-z0-9-]*):`)

// emphasisRE matches **bold**, __bold__, _italic_, *italic*, `code`.
// Greedy, non-nested. We process emphasis on text spans between FA shortcodes.
var emphasisRE = regexp.MustCompile(
	"(\\*\\*[^*\\n]+\\*\\*)|(__[^_\\n]+__)|(\\*[^*\\n]+\\*)|(_[^_\\n]+_)|(`[^`\\n]+`)",
)

// parseInlines tokenizes a single line of markdown text into Inline runs.
// FA shortcodes are extracted first (so emphasis processing doesn't see
// the colons inside them), then emphasis is applied to the remaining text.
func parseInlines(text string, defaultFAStyle string) []Inline {
	var out []Inline

	idxs := faShortcodeRE.FindAllStringSubmatchIndex(text, -1)
	cursor := 0
	for _, m := range idxs {
		// Append text before the icon.
		if m[0] > cursor {
			out = append(out, parseEmphasis(text[cursor:m[0]])...)
		}
		stylePrefix := ""
		if m[2] != -1 {
			stylePrefix = text[m[2]:m[3]]
		}
		name := text[m[4]:m[5]]
		out = append(out, IconRef{Style: faStyleFromPrefix(stylePrefix, defaultFAStyle), Name: name})
		cursor = m[1]
	}
	if cursor < len(text) {
		out = append(out, parseEmphasis(text[cursor:])...)
	}
	return out
}

func faStyleFromPrefix(prefix, defaultStyle string) string {
	switch prefix {
	case "":
		return defaultStyle
	case "s":
		return "solid"
	case "r":
		return "regular"
	case "b":
		return "brands"
	case "l", "d":
		// FA Free has no light or duotone; substitute with solid.
		return "solid"
	default:
		return defaultStyle
	}
}

func parseEmphasis(s string) []Inline {
	var out []Inline
	cursor := 0
	for _, m := range emphasisRE.FindAllStringIndex(s, -1) {
		if m[0] > cursor {
			out = append(out, TextRun{Text: s[cursor:m[0]]})
		}
		token := s[m[0]:m[1]]
		switch {
		case strings.HasPrefix(token, "**") && strings.HasSuffix(token, "**"):
			out = append(out, TextRun{Text: token[2 : len(token)-2], Bold: true})
		case strings.HasPrefix(token, "__") && strings.HasSuffix(token, "__"):
			out = append(out, TextRun{Text: token[2 : len(token)-2], Bold: true})
		case strings.HasPrefix(token, "`") && strings.HasSuffix(token, "`"):
			out = append(out, TextRun{Text: token[1 : len(token)-1], Code: true})
		case strings.HasPrefix(token, "*") && strings.HasSuffix(token, "*"):
			out = append(out, TextRun{Text: token[1 : len(token)-1], Italic: true})
		case strings.HasPrefix(token, "_") && strings.HasSuffix(token, "_"):
			out = append(out, TextRun{Text: token[1 : len(token)-1], Italic: true})
		}
		cursor = m[1]
	}
	if cursor < len(s) {
		out = append(out, TextRun{Text: s[cursor:]})
	}
	return out
}

// stripFAShortcodes removes :fa*-name: tokens from text (used for speaker
// notes which can't render images).
func stripFAShortcodes(text string) string {
	return faShortcodeRE.ReplaceAllString(text, "")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run "TestParseInlines|TestStripFAShortcodes" -v`
Expected: PASS for all sub-tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_inlines.go internal/cmd/slides_markdown_inlines_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add inline parser for FA shortcodes and emphasis

Tokenizes a line of markdown into TextRun and IconRef inlines. Style
derivation matches §4.6 (fa→default, fas→solid, far→regular, fab→brands,
fal/fad→solid with free-tier substitution).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Body block parser (paragraphs, bullets, code, headings)

**Spec coverage:** §4.x base (block-level body parsing prerequisite for §4.4/4.5/4.7).

**Files:**
- Create: `internal/cmd/slides_markdown_blocks.go`
- Test: `internal/cmd/slides_markdown_blocks_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/cmd/slides_markdown_blocks_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBlocks_Paragraph(t *testing.T) {
	got := parseBlocks("Hello world.\n", "solid")
	assert.Equal(t, []Block{
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Hello world."}}},
	}, got)
}

func TestParseBlocks_BulletList(t *testing.T) {
	got := parseBlocks("- one\n- two **bold**\n- three\n", "solid")
	assert.Equal(t, []Block{
		BulletsBlock{Items: []BulletItem{
			{Indent: 0, Inlines: []Inline{TextRun{Text: "one"}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "two "}, TextRun{Text: "bold", Bold: true}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "three"}}},
		}},
	}, got)
}

func TestParseBlocks_OrderedList(t *testing.T) {
	got := parseBlocks("1. first\n2. second\n", "solid")
	assert.Equal(t, []Block{
		BulletsBlock{Ordered: true, Items: []BulletItem{
			{Indent: 0, Inlines: []Inline{TextRun{Text: "first"}}},
			{Indent: 0, Inlines: []Inline{TextRun{Text: "second"}}},
		}},
	}, got)
}

func TestParseBlocks_CodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		CodeBlock{Lang: "go", Source: "func main() {}"},
	}, got)
}

func TestParseBlocks_Heading(t *testing.T) {
	got := parseBlocks("### Subsection\n", "solid")
	assert.Equal(t, []Block{
		HeadingBlock{Level: 3, Inlines: []Inline{TextRun{Text: "Subsection"}}},
	}, got)
}

func TestParseBlocks_Mixed(t *testing.T) {
	input := "## Topic\n\nIntro paragraph.\n\n- bullet 1\n- bullet 2\n\nFollowup.\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		HeadingBlock{Level: 2, Inlines: []Inline{TextRun{Text: "Topic"}}},
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Intro paragraph."}}},
		BulletsBlock{Items: []BulletItem{
			{Inlines: []Inline{TextRun{Text: "bullet 1"}}},
			{Inlines: []Inline{TextRun{Text: "bullet 2"}}},
		}},
		ParagraphBlock{Inlines: []Inline{TextRun{Text: "Followup."}}},
	}, got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestParseBlocks`
Expected: FAIL with undefined `parseBlocks`.

- [ ] **Step 3: Implement the block parser**

`internal/cmd/slides_markdown_blocks.go`:

```go
package cmd

import (
	"regexp"
	"strings"
)

var (
	bulletRE  = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	orderedRE = regexp.MustCompile(`^(\s*)\d+\.\s+(.*)$`)
	headingRE = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
)

// parseBlocks turns body markdown into top-level blocks. It handles
// paragraphs, bullets (- or *), ordered lists (1.), fenced code blocks,
// and headings. Column / boxes / arrows / mermaid markers are recognized
// in later tasks (5, 6, 7); this parser delegates to helpers from those
// tasks once they exist.
func parseBlocks(body string, defaultFAStyle string) []Block {
	lines := strings.Split(body, "\n")
	var out []Block

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip blank lines between blocks.
		if trimmed == "" {
			i++
			continue
		}

		// Fenced code block.
		if strings.HasPrefix(trimmed, "```") {
			lang := strings.TrimPrefix(trimmed, "```")
			var src strings.Builder
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				if src.Len() > 0 {
					src.WriteString("\n")
				}
				src.WriteString(lines[i])
				i++
			}
			if i < len(lines) {
				i++ // consume closing ```
			}
			out = append(out, CodeBlock{Lang: lang, Source: src.String()})
			continue
		}

		// Heading.
		if m := headingRE.FindStringSubmatch(line); m != nil {
			out = append(out, HeadingBlock{
				Level:   len(m[1]),
				Inlines: parseInlines(strings.TrimSpace(m[2]), defaultFAStyle),
			})
			i++
			continue
		}

		// Bullet list (consume run of bullet lines).
		if bulletRE.MatchString(line) {
			var items []BulletItem
			for i < len(lines) {
				m := bulletRE.FindStringSubmatch(lines[i])
				if m == nil {
					break
				}
				items = append(items, BulletItem{
					Indent:  len(m[1]) / 2,
					Inlines: parseInlines(strings.TrimSpace(m[2]), defaultFAStyle),
				})
				i++
			}
			out = append(out, BulletsBlock{Items: items})
			continue
		}

		// Ordered list.
		if orderedRE.MatchString(line) {
			var items []BulletItem
			for i < len(lines) {
				m := orderedRE.FindStringSubmatch(lines[i])
				if m == nil {
					break
				}
				items = append(items, BulletItem{
					Indent:  len(m[1]) / 2,
					Inlines: parseInlines(strings.TrimSpace(m[2]), defaultFAStyle),
				})
				i++
			}
			out = append(out, BulletsBlock{Ordered: true, Items: items})
			continue
		}

		// Paragraph: consume contiguous non-blank, non-special lines.
		var paraLines []string
		for i < len(lines) {
			pl := lines[i]
			pt := strings.TrimSpace(pl)
			if pt == "" || strings.HasPrefix(pt, "```") || bulletRE.MatchString(pl) ||
				orderedRE.MatchString(pl) || headingRE.MatchString(pl) {
				break
			}
			paraLines = append(paraLines, pt)
			i++
		}
		if len(paraLines) > 0 {
			out = append(out, ParagraphBlock{
				Inlines: parseInlines(strings.Join(paraLines, " "), defaultFAStyle),
			})
		}
	}

	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestParseBlocks -v`
Expected: PASS for all 6 sub-tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_blocks.go internal/cmd/slides_markdown_blocks_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add body block parser for paragraph/bullet/code/heading

Walks slide body line-by-line and emits Block AST nodes. Inline parsing
delegates to parseInlines from Task 3. Column, boxes/arrows, and mermaid
marker handling is added in subsequent tasks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: ::cols:: column block parser

**Spec coverage:** §4.4 (Columns).

**Files:**
- Modify: `internal/cmd/slides_markdown_blocks.go`
- Modify: `internal/cmd/slides_markdown_blocks_test.go`

- [ ] **Step 1: Add the failing tests**

Append to `internal/cmd/slides_markdown_blocks_test.go`:

```go
func TestParseBlocks_TwoColumns(t *testing.T) {
	input := "::cols::\n\nleft side text\n\n::col2::\n\nright side text\n\n::/cols::\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		ColumnsBlock{Columns: [][]Block{
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "left side text"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "right side text"}}}},
		}},
	}, got)
}

func TestParseBlocks_ThreeColumns(t *testing.T) {
	input := "::cols::\n\nA\n\n::col2::\n\nB\n\n::col3::\n\nC\n\n::/cols::\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		ColumnsBlock{Columns: [][]Block{
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "A"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "B"}}}},
			{ParagraphBlock{Inlines: []Inline{TextRun{Text: "C"}}}},
		}},
	}, got)
}

func TestParseBlocks_RightSynonymForCol2(t *testing.T) {
	input := "::cols::\n\nA\n\n::right::\n\nB\n\n::/cols::\n"
	got := parseBlocks(input, "solid")
	require.Equal(t, 1, len(got))
	col, ok := got[0].(ColumnsBlock)
	assert.True(t, ok)
	assert.Equal(t, 2, len(col.Columns))
}
```

Add `"github.com/stretchr/testify/require"` to the test file imports (alongside the existing `"github.com/stretchr/testify/assert"`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestParseBlocks_TwoColumns`
Expected: FAIL — no column handling yet.

- [ ] **Step 3: Implement column parsing**

In `internal/cmd/slides_markdown_blocks.go`, add at top:

```go
const (
	colsOpen     = "::cols::"
	colsClose    = "::/cols::"
	colMarker2   = "::col2::"
	colMarker3   = "::col3::"
	colMarkerAlt = "::right::" // synonym for col2
)
```

Inside `parseBlocks`, before the heading branch, add:

```go
		// Columns block.
		if trimmed == colsOpen {
			i++
			cols, consumed := consumeColumnsBlock(lines[i:], defaultFAStyle)
			i += consumed
			out = append(out, cols)
			continue
		}
```

Then add the helper function:

```go
func consumeColumnsBlock(lines []string, defaultFAStyle string) (ColumnsBlock, int) {
	var current []string
	var columns [][]string
	flush := func() {
		columns = append(columns, append([]string(nil), current...))
		current = nil
	}

	consumed := 0
	for consumed < len(lines) {
		line := lines[consumed]
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case colsClose:
			flush()
			consumed++
			return columnsBlockFromRaw(columns, defaultFAStyle), consumed
		case colMarker2, colMarker3, colMarkerAlt:
			flush()
			consumed++
			continue
		}
		current = append(current, line)
		consumed++
	}
	// EOF without close — still flush what we have.
	flush()
	return columnsBlockFromRaw(columns, defaultFAStyle), consumed
}

func columnsBlockFromRaw(raw [][]string, defaultFAStyle string) ColumnsBlock {
	cb := ColumnsBlock{}
	for _, col := range raw {
		body := strings.Join(col, "\n")
		cb.Columns = append(cb.Columns, parseBlocks(body, defaultFAStyle))
	}
	return cb
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestParseBlocks -v`
Expected: PASS for all sub-tests including new column ones.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_blocks.go internal/cmd/slides_markdown_blocks_test.go
git commit -m "$(cat <<'EOF'
feat(slides): parse ::cols::/::col2::/::col3::/::right::/::/cols:: markers

Recursive call into parseBlocks per column body. Accepts ::right:: as a
synonym for ::col2:: (slidey allows both for the 2-column case).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: ::boxes:: and ::arrows:: block parser

**Spec coverage:** §4.5 (boxes/arrows).

**Files:**
- Modify: `internal/cmd/slides_markdown_blocks.go`
- Modify: `internal/cmd/slides_markdown_blocks_test.go`

- [ ] **Step 1: Add the failing tests**

```go
func TestParseBlocks_BoxesBlock(t *testing.T) {
	input := "::boxes::\n:fa-rectangle-ad: Campaigns\n:fa-headset: Support Tickets\nNo Icon Row\n::/boxes::\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		IconRowsBlock{Kind: "boxes", Rows: []IconRow{
			{Icon: &IconRef{Style: "solid", Name: "rectangle-ad"}, Text: "Campaigns"},
			{Icon: &IconRef{Style: "solid", Name: "headset"}, Text: "Support Tickets"},
			{Icon: nil, Text: "No Icon Row"},
		}},
	}, got)
}

func TestParseBlocks_ArrowsBlock(t *testing.T) {
	input := "::arrows::\n\n### Step One\n\n### Step Two\n\n::/arrows::\n"
	got := parseBlocks(input, "solid")
	assert.Equal(t, []Block{
		IconRowsBlock{Kind: "arrows", Rows: []IconRow{
			{Icon: nil, Text: "Step One"},
			{Icon: nil, Text: "Step Two"},
		}},
	}, got)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestParseBlocks_BoxesBlock|TestParseBlocks_ArrowsBlock"`
Expected: FAIL — no boxes/arrows handling yet.

- [ ] **Step 3: Implement boxes/arrows parsing**

In `internal/cmd/slides_markdown_blocks.go`, add constants:

```go
const (
	boxesOpen   = "::boxes::"
	boxesClose  = "::/boxes::"
	arrowsOpen  = "::arrows::"
	arrowsClose = "::/arrows::"
)
```

Inside `parseBlocks`, before the heading branch, add:

```go
		if trimmed == boxesOpen {
			i++
			block, consumed := consumeIconRowsBlock(lines[i:], "boxes", boxesClose, defaultFAStyle)
			i += consumed
			out = append(out, block)
			continue
		}
		if trimmed == arrowsOpen {
			i++
			block, consumed := consumeIconRowsBlock(lines[i:], "arrows", arrowsClose, defaultFAStyle)
			i += consumed
			out = append(out, block)
			continue
		}
```

Helper:

```go
func consumeIconRowsBlock(lines []string, kind, closeMarker, defaultFAStyle string) (IconRowsBlock, int) {
	block := IconRowsBlock{Kind: kind}
	consumed := 0
	for consumed < len(lines) {
		line := lines[consumed]
		trimmed := strings.TrimSpace(line)
		consumed++
		if trimmed == closeMarker {
			return block, consumed
		}
		if trimmed == "" {
			continue
		}
		// Strip leading heading marks (### Step) for arrows-style content.
		if m := headingRE.FindStringSubmatch(trimmed); m != nil {
			trimmed = strings.TrimSpace(m[2])
		}
		row := IconRow{}
		// Try to extract a leading FA shortcode.
		if m := faShortcodeRE.FindStringSubmatchIndex(trimmed); m != nil && m[0] == 0 {
			stylePrefix := ""
			if m[2] != -1 {
				stylePrefix = trimmed[m[2]:m[3]]
			}
			name := trimmed[m[4]:m[5]]
			ref := IconRef{Style: faStyleFromPrefix(stylePrefix, defaultFAStyle), Name: name}
			row.Icon = &ref
			row.Text = strings.TrimSpace(trimmed[m[1]:])
		} else {
			row.Text = trimmed
		}
		block.Rows = append(block.Rows, row)
	}
	return block, consumed
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestParseBlocks -v`
Expected: PASS for all sub-tests including new boxes/arrows ones.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_blocks.go internal/cmd/slides_markdown_blocks_test.go
git commit -m "$(cat <<'EOF'
feat(slides): parse ::boxes::/::arrows:: icon-row blocks

Each row may begin with a leading FA shortcode (boxes) or a heading
prefix (arrows). Trailing text becomes the row label.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Mermaid fenced-code recognition

**Spec coverage:** §4.7 (Mermaid fenced blocks).

**Files:**
- Modify: `internal/cmd/slides_markdown_blocks.go`
- Modify: `internal/cmd/slides_markdown_blocks_test.go`

- [ ] **Step 1: Add the failing test**

```go
func TestParseBlocks_MermaidBlock(t *testing.T) {
	input := "```mermaid\nflowchart LR\n  A --> B\n```\n"
	got := parseBlocks(input, "solid")
	require.Equal(t, 1, len(got))
	d, ok := got[0].(DiagramBlock)
	require.True(t, ok)
	assert.Equal(t, "mermaid", d.Kind)
	assert.Equal(t, "flowchart LR\n  A --> B", d.Source)
	assert.NotEmpty(t, d.BlockID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestParseBlocks_MermaidBlock`
Expected: FAIL — currently returns `CodeBlock{Lang:"mermaid"}` instead of `DiagramBlock`.

- [ ] **Step 3: Update the code-block branch in `parseBlocks`**

In the existing fenced-code-block branch, replace the final `out = append(...)` with:

```go
			lang := strings.TrimPrefix(trimmed, "```")
			// (existing source-collection loop unchanged)
			if lang == "mermaid" {
				out = append(out, DiagramBlock{
					Kind:    "mermaid",
					Source:  src.String(),
					BlockID: nextBlockID(),
				})
			} else {
				out = append(out, CodeBlock{Lang: lang, Source: src.String()})
			}
			continue
```

Add a package-level monotonic ID generator (used by parseSlideBody to assign unique IDs):

```go
import "sync/atomic"

var blockIDCounter atomic.Uint64

func nextBlockID() string {
	return fmt.Sprintf("block-%d", blockIDCounter.Add(1))
}
```

(Add `"fmt"` and `"sync/atomic"` to the imports if not already present.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestParseBlocks -v`
Expected: PASS for all sub-tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown_blocks.go internal/cmd/slides_markdown_blocks_test.go
git commit -m "$(cat <<'EOF'
feat(slides): emit DiagramBlock for ```mermaid fenced code

Other languages remain CodeBlock. Each diagram gets a stable BlockID so
the asset pipeline (Task 11) can pair it with an uploaded image.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Slide orchestrator (title hoist + Notes split + replace legacy parser)

**Spec coverage:** §4.2 (title hoisting), §4.3 (`## Notes` extraction), wires Tasks 2–7 together.

**Files:**
- Modify: `internal/cmd/slides_markdown.go` (full replacement of legacy types/parser)
- Create: `internal/cmd/slides_markdown_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/cmd/slides_markdown_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownToSlides_TitleHoistFromH1(t *testing.T) {
	input := "# Hello\n\nbody text\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Hello", got[0].Title)
	require.Equal(t, 1, len(got[0].Body))
	assert.IsType(t, ParagraphBlock{}, got[0].Body[0])
}

func TestParseMarkdownToSlides_TitleFallbackToH2(t *testing.T) {
	input := "## Topic Heading\n\n- a\n- b\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "Topic Heading", got[0].Title)
}

func TestParseMarkdownToSlides_HeroLayoutKeepsH1InBody(t *testing.T) {
	input := "---\nlayout: hero\n---\n\n# Big Wordmark\n\nsubline\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Equal(t, "", got[0].Title, "title should not be hoisted on hero")
	require.GreaterOrEqual(t, len(got[0].Body), 1)
	first, ok := got[0].Body[0].(HeadingBlock)
	require.True(t, ok)
	assert.Equal(t, 1, first.Level)
}

func TestParseMarkdownToSlides_NotesExtraction(t *testing.T) {
	input := "## Topic\n\nbody\n\n## Notes\n\n- speaker note one\n- speaker note two\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.Contains(t, got[0].Notes, "speaker note one")
	assert.Contains(t, got[0].Notes, "speaker note two")
	for _, b := range got[0].Body {
		if h, ok := b.(HeadingBlock); ok && len(h.Inlines) > 0 {
			if tr, ok := h.Inlines[0].(TextRun); ok {
				assert.NotEqual(t, "Notes", tr.Text, "Notes heading should be removed from body")
			}
		}
	}
}

func TestParseMarkdownToSlides_NotesStripsFAShortcodes(t *testing.T) {
	input := "## Topic\n\nbody\n\n## Notes\n\n:fa-truck-fast: Orders matter\n"
	got, err := ParseMarkdownToSlides(input, ParseOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	assert.NotContains(t, got[0].Notes, ":fa-truck-fast:")
	assert.Contains(t, got[0].Notes, "Orders matter")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestParseMarkdownToSlides`
Expected: FAIL — current `ParseMarkdownToSlides` has the legacy signature `[]Slide` (no error) and the legacy `Slide` type.

- [ ] **Step 3: Replace the legacy parser**

Replace the **entire contents** of `internal/cmd/slides_markdown.go` with:

```go
package cmd

import (
	"strings"
)

// ParseOptions configures the markdown parser.
type ParseOptions struct {
	DefaultFAStyle string // "solid"|"regular"|"brands"; empty → "solid"
}

// ParseMarkdownToSlides parses a slidey-flavored markdown deck into a
// slice of Slide AST nodes. Returns an error if frontmatter is malformed.
func ParseMarkdownToSlides(markdown string, opts ParseOptions) ([]Slide, error) {
	if opts.DefaultFAStyle == "" {
		opts.DefaultFAStyle = "solid"
	}
	blocks, err := splitMarkdownIntoSlideBlocks(markdown)
	if err != nil {
		return nil, err
	}
	out := make([]Slide, 0, len(blocks))
	for _, b := range blocks {
		s, err := parseSlideFromBlock(b, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func parseSlideFromBlock(b slideBlock, opts ParseOptions) (Slide, error) {
	body, notesText := splitOutNotes(b.Body)
	parsed := parseBlocks(body, opts.DefaultFAStyle)

	slide := Slide{
		Frontmatter: b.Frontmatter,
		Body:        parsed,
		Notes:       stripFAShortcodes(notesText),
	}

	if !layoutSkipsTitleHoist(b.Frontmatter.Layout) {
		title, remaining := hoistTitle(parsed)
		slide.Title = title
		slide.Body = remaining
	}
	return slide, nil
}

// splitOutNotes scans body lines for an exact "## Notes" or "### Notes"
// heading (case-sensitive). Everything from that heading to the end is
// returned as raw notes text (without the heading itself); the body
// returned is everything before.
func splitOutNotes(body string) (newBody string, notes string) {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "## Notes" || t == "### Notes" {
			b := strings.Join(lines[:i], "\n")
			n := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			return b, n
		}
	}
	return body, ""
}

// hoistTitle returns the first h1 (or h2 fallback) inline text and the
// blocks with that heading removed.
func hoistTitle(blocks []Block) (string, []Block) {
	// First pass: look for h1.
	for i, b := range blocks {
		if h, ok := b.(HeadingBlock); ok && h.Level == 1 {
			return inlinesToText(h.Inlines), removeIndex(blocks, i)
		}
	}
	// Fallback: first h2.
	for i, b := range blocks {
		if h, ok := b.(HeadingBlock); ok && h.Level == 2 {
			return inlinesToText(h.Inlines), removeIndex(blocks, i)
		}
	}
	return "", blocks
}

func removeIndex(s []Block, i int) []Block {
	out := make([]Block, 0, len(s)-1)
	out = append(out, s[:i]...)
	out = append(out, s[i+1:]...)
	return out
}

func inlinesToText(inlines []Inline) string {
	var b strings.Builder
	for _, in := range inlines {
		if tr, ok := in.(TextRun); ok {
			b.WriteString(tr.Text)
		}
	}
	return b.String()
}

func layoutSkipsTitleHoist(layout string) bool {
	switch layout {
	case "title", "hero", "statement":
		return true
	}
	return false
}

// literalMarkdownTripleDash is the slide separator and frontmatter delimiter.
const literalMarkdownTripleDash = "---"
```

**Note:** this deletes the legacy `SlideElement`, `SlideLayout`, `LayoutTitleOnly`/etc., `parseSlide`, `stripInlineFormatting`, and `determineLayout`. The renderer in `slides_formatter.go` will break compilation; Task 13 fixes it. To keep this commit compilable, **also do Step 3a below** before running tests.

- [ ] **Step 3a: Stub the renderer so the package still compiles**

Replace the **entire contents** of `internal/cmd/slides_formatter.go` with a temporary stub:

```go
package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/slides/v1"
)

// SlidesToAPIRequests is replaced in Task 13. This stub exists so the
// package compiles between the parser swap (Task 8) and the new renderer.
func SlidesToAPIRequests(_ []Slide) ([]*slides.Request, map[int]string) {
	return nil, map[int]string{}
}

// CreatePresentationFromMarkdown is the orchestrator the CLI calls. The
// real version (Task 14) wires in the asset pipeline. For now it returns
// a clear "not yet implemented" error so the CLI can still build.
func CreatePresentationFromMarkdown(
	_ context.Context,
	_ *slides.Service,
	_ string,
	_ string,
	_ []Slide,
) (*slides.Presentation, error) {
	return nil, fmt.Errorf("slidey renderer not yet wired (Task 13/14)")
}
```

(Adjust the parameter list of `CreatePresentationFromMarkdown` to match the **existing** function signature in `slides_formatter.go` — read it once and copy. Tasks 13/14 will replace this stub.)

- [ ] **Step 4: Run tests to verify they pass and build still works**

Run: `go build ./... && go test ./internal/cmd/ -run TestParseMarkdownToSlides -v`
Expected: BUILD PASS, all `TestParseMarkdownToSlides_*` PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_markdown.go internal/cmd/slides_markdown_test.go internal/cmd/slides_formatter.go
git commit -m "$(cat <<'EOF'
feat(slides): replace legacy markdown parser with slidey AST orchestrator

ParseMarkdownToSlides now returns ([]Slide, error). Title is hoisted from
the first h1 (or h2 fallback) for non-title/hero/statement layouts.
"## Notes" trailing section becomes Slide.Notes (FA shortcodes stripped).

Renderer is stubbed; Tasks 13/14 reimplement it on the new AST.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Layout helpers + geometry

**Spec coverage:** §6 (Layout mapping + geometry).

**Files:**
- Create: `internal/cmd/slides_layout.go`
- Create: `internal/cmd/slides_layout_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/cmd/slides_layout_test.go`:

```go
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
	// Both columns same width, side by side, body height = pageHeight - bodyTop - margin.
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestMapSlideyLayout|TestColumnBoxes"`
Expected: FAIL with undefined `MapSlideyLayout` / `ColumnBoxes` / `LayoutKind` / `LayoutGeometry` / `BoxRect`.

- [ ] **Step 3: Implement the helpers**

`internal/cmd/slides_layout.go`:

```go
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

// MapSlideyLayout maps a slidey frontmatter layout name to a LayoutKind.
// Unknown values fall back to LayoutKindDefault.
func MapSlideyLayout(name string) LayoutKind {
	switch name {
	case "center":
		return LayoutKindCenter
	case "title", "hero", "statement":
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run "TestMapSlideyLayout|TestColumnBoxes" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_layout.go internal/cmd/slides_layout_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add layout-mapping and column-geometry helpers

MapSlideyLayout collapses the slidey frontmatter values into a small
LayoutKind enum. ColumnBoxes/SingleBodyBox/TitleBox compute box
rectangles in points from the presentation page size.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: FA URL builder + asset map types

**Spec coverage:** §7.1 (FA fetch URL shape), partial §7 (AssetMap).

**Files:**
- Create: `internal/cmd/slides_assets.go`
- Create: `internal/cmd/slides_assets_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/cmd/slides_assets_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFASVGURL(t *testing.T) {
	cases := []struct {
		style, name, expected string
	}{
		{"solid", "truck-fast", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/solid/truck-fast.svg"},
		{"brands", "github", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/brands/github.svg"},
		{"regular", "clock", "https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/regular/clock.svg"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, faSVGURL(tc.style, tc.name))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestFASVGURL`
Expected: FAIL — undefined `faSVGURL`.

- [ ] **Step 3: Implement asset types and the URL builder**

`internal/cmd/slides_assets.go`:

```go
package cmd

import (
	"fmt"
	"net/http"
	"time"
)

// AssetMap pairs parsed AST references with uploaded Drive ImageRefs.
// Icons is keyed by IconRef value (Style+Name); Diagrams is keyed by
// DiagramBlock.BlockID.
type AssetMap struct {
	Icons    map[IconRef]ImageRef
	Diagrams map[string]ImageRef
}

// NewAssetMap returns an empty initialized AssetMap.
func NewAssetMap() AssetMap {
	return AssetMap{
		Icons:    map[IconRef]ImageRef{},
		Diagrams: map[string]ImageRef{},
	}
}

// AssetPipelineConfig holds the runtime knobs for the pipeline.
type AssetPipelineConfig struct {
	HTTPClient     *http.Client
	MMDCPath       string
	Strict         bool
	KeepTempImages bool
	DefaultFAStyle string
}

// DefaultAssetPipelineConfig returns a config with sane defaults: 30s
// HTTP timeout, mmdc on PATH, non-strict, no image retention.
func DefaultAssetPipelineConfig() AssetPipelineConfig {
	return AssetPipelineConfig{
		HTTPClient:     &http.Client{Timeout: 30 * time.Second},
		MMDCPath:       "mmdc",
		Strict:         false,
		KeepTempImages: false,
		DefaultFAStyle: "solid",
	}
}

func faSVGURL(style, name string) string {
	return fmt.Sprintf(
		"https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/%s/%s.svg",
		style, name,
	)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cmd/ -run TestFASVGURL -v && go build ./...`
Expected: PASS, build PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_assets.go internal/cmd/slides_assets_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add asset map types and FA URL builder

AssetMap pairs AST IconRefs/DiagramBlocks with their Drive ImageRefs.
DefaultAssetPipelineConfig sets a 30s HTTP timeout and the standard
mmdc binary. The FA URL builder targets the FA Free 6.x jsDelivr path.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: FA fetch + Drive upload

**Spec coverage:** §7.1 (FA pipeline).

**Files:**
- Modify: `internal/cmd/slides_assets.go`
- Modify: `internal/cmd/slides_assets_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/cmd/slides_assets_test.go`:

```go
import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	// existing imports retained
)

func TestFetchFAIcon_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<svg/>")
	}))
	t.Cleanup(srv.Close)

	body, err := fetchFAIconFromURL(context.Background(), srv.Client(), srv.URL+"/x.svg")
	require.NoError(t, err)
	assert.Equal(t, "<svg/>", string(body))
}

func TestFetchFAIcon_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	_, err := fetchFAIconFromURL(context.Background(), srv.Client(), srv.URL+"/x.svg")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "404"))
}
```

(Add `"io"`, `"net/http/httptest"`, `"context"`, `"strings"`, `"github.com/stretchr/testify/require"` to the imports if not already present.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestFetchFAIcon`
Expected: FAIL — undefined `fetchFAIconFromURL`.

- [ ] **Step 3: Implement the fetcher**

Append to `internal/cmd/slides_assets.go`:

```go
import (
	"context"
	"io"
	"net/http"
	// existing imports retained
)

func fetchFAIconFromURL(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestFetchFAIcon -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_assets.go internal/cmd/slides_assets_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add Font Awesome SVG fetcher

fetchFAIconFromURL performs a GET against jsDelivr (or any URL injected
in tests via httptest) and returns the SVG bytes. 404/non-200 are wrapped
errors.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Mermaid render via mmdc

**Spec coverage:** §7.2 (Mermaid pipeline).

**Files:**
- Modify: `internal/cmd/slides_assets.go`
- Modify: `internal/cmd/slides_assets_test.go`

- [ ] **Step 1: Write the failing tests**

Append:

```go
func TestMMDCCommandArgs(t *testing.T) {
	args := mmdcCommandArgs("/usr/bin/mmdc", "/tmp/in.mmd", "/tmp/out.png")
	assert.Equal(t, []string{"/usr/bin/mmdc", "-i", "/tmp/in.mmd", "-o", "/tmp/out.png", "-b", "transparent", "--scale", "2"}, args)
}

func TestRenderMermaid_BinaryMissing(t *testing.T) {
	_, err := renderMermaidWithBinary(context.Background(), "/nonexistent/mmdc-binary", "graph TD\nA-->B")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestMMDCCommandArgs|TestRenderMermaid"`
Expected: FAIL with undefined functions.

- [ ] **Step 3: Implement**

Append to `internal/cmd/slides_assets.go`:

```go
import (
	"os"
	"os/exec"
	"path/filepath"
	// existing imports retained
)

func mmdcCommandArgs(mmdcPath, in, out string) []string {
	return []string{mmdcPath, "-i", in, "-o", out, "-b", "transparent", "--scale", "2"}
}

// renderMermaidWithBinary writes source to a temp .mmd, runs mmdc, and
// returns the rendered PNG bytes. The temp files are cleaned up.
func renderMermaidWithBinary(ctx context.Context, mmdcPath, source string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "gogcli-mermaid-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "in.mmd")
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(in, []byte(source), 0o600); err != nil {
		return nil, err
	}
	args := mmdcCommandArgs(mmdcPath, in, out)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) // #nosec G204 — args constructed from validated config
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("mmdc failed: %w", err)
	}
	return os.ReadFile(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run "TestMMDCCommandArgs|TestRenderMermaid" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_assets.go internal/cmd/slides_assets_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add mmdc-backed mermaid renderer

mmdcCommandArgs builds the standardized invocation; renderMermaidWithBinary
writes a temp .mmd, runs mmdc with a transparent background and 2x scale,
and returns the rendered PNG. Temp files are cleaned up.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Asset pipeline orchestrator + Drive upload + cleanup

**Spec coverage:** §7.1, §7.2, §7.3 (Drive cleanup, orchestration).

**Files:**
- Modify: `internal/cmd/slides_assets.go`
- Modify: `internal/cmd/slides_assets_test.go`

- [ ] **Step 1: Write the failing tests**

Append:

```go
type fakeDriveUploader struct {
	uploaded []string // file IDs in upload order
	deleted  []string
}

func (f *fakeDriveUploader) UploadAsset(ctx context.Context, name, mime string, body []byte) (ImageRef, error) {
	id := fmt.Sprintf("file-%d", len(f.uploaded)+1)
	f.uploaded = append(f.uploaded, id)
	return ImageRef{DriveFileID: id, PublicURL: "https://drive.example/" + id}, nil
}
func (f *fakeDriveUploader) DeleteAsset(ctx context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestAssetPipeline_CollectsUniqueIcons(t *testing.T) {
	cfg := DefaultAssetPipelineConfig()
	cfg.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) *http.Response {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("<svg/>")), Header: http.Header{}}
	})}
	cfg.MMDCPath = "" // disable mmdc; no diagrams in test

	uploader := &fakeDriveUploader{}
	p := &AssetPipeline{Config: cfg, Uploader: uploader}

	slides := []Slide{
		{Body: []Block{ParagraphBlock{Inlines: []Inline{
			IconRef{Style: "solid", Name: "truck-fast"},
			TextRun{Text: " hello "},
			IconRef{Style: "solid", Name: "truck-fast"}, // duplicate, should not re-upload
		}}}},
		{Body: []Block{IconRowsBlock{Kind: "boxes", Rows: []IconRow{
			{Icon: &IconRef{Style: "brands", Name: "github"}, Text: "GitHub"},
		}}}},
	}

	am, err := p.Resolve(context.Background(), slides)
	require.NoError(t, err)
	assert.Equal(t, 2, len(am.Icons), "two unique icons, no duplicates")
	assert.Equal(t, 2, len(uploader.uploaded), "exactly two Drive uploads")
}

func TestAssetPipeline_Cleanup(t *testing.T) {
	uploader := &fakeDriveUploader{}
	p := &AssetPipeline{Config: DefaultAssetPipelineConfig(), Uploader: uploader}
	uploader.uploaded = []string{"file-1", "file-2"}
	p.uploaded = []string{"file-1", "file-2"}

	require.NoError(t, p.Cleanup(context.Background()))
	assert.Equal(t, []string{"file-1", "file-2"}, uploader.deleted)
}

type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run "TestAssetPipeline"`
Expected: FAIL — `AssetPipeline`, `Resolve`, `Cleanup`, `Uploader` undefined.

- [ ] **Step 3: Implement the orchestrator**

Append to `internal/cmd/slides_assets.go`:

```go
// Uploader abstracts the Drive operations the pipeline needs. Real impl
// (Task 14) wraps drive.Service; tests use fakeDriveUploader.
type Uploader interface {
	UploadAsset(ctx context.Context, name, mime string, body []byte) (ImageRef, error)
	DeleteAsset(ctx context.Context, fileID string) error
}

// AssetPipeline resolves all FA icon and mermaid diagram references in a
// slice of Slides into ImageRefs by fetching/rendering them and uploading
// to Drive via the Uploader.
type AssetPipeline struct {
	Config   AssetPipelineConfig
	Uploader Uploader

	// uploaded tracks Drive file IDs created by this pipeline so Cleanup
	// can delete them when --keep-temp-images is false.
	uploaded []string
}

// Resolve walks all slides, collects unique IconRefs and DiagramBlocks,
// fetches/renders/uploads each, and returns the resulting AssetMap.
//
// Per-asset failures are logged (warn-and-skip) unless Config.Strict.
func (p *AssetPipeline) Resolve(ctx context.Context, slides []Slide) (AssetMap, error) {
	am := NewAssetMap()

	icons := collectIconRefs(slides)
	diagrams := collectDiagrams(slides)

	for ref := range icons {
		url := faSVGURL(ref.Style, ref.Name)
		body, err := fetchFAIconFromURL(ctx, p.Config.HTTPClient, url)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping FA icon :%s-%s: %v\n", ref.Style, ref.Name, err)
			continue
		}
		ir, err := p.Uploader.UploadAsset(ctx, fmt.Sprintf("fa-%s-%s.svg", ref.Style, ref.Name), "image/svg+xml", body)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping FA icon :%s-%s: upload: %v\n", ref.Style, ref.Name, err)
			continue
		}
		am.Icons[ref] = ir
		p.uploaded = append(p.uploaded, ir.DriveFileID)
	}

	for blockID, source := range diagrams {
		if p.Config.MMDCPath == "" {
			fmt.Fprintf(os.Stderr, "warning: mmdc not configured; skipping mermaid diagram %s\n", blockID)
			continue
		}
		png, err := renderMermaidWithBinary(ctx, p.Config.MMDCPath, source)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping mermaid diagram %s: %v\n", blockID, err)
			continue
		}
		ir, err := p.Uploader.UploadAsset(ctx, blockID+".png", "image/png", png)
		if err != nil {
			if p.Config.Strict {
				return am, err
			}
			fmt.Fprintf(os.Stderr, "warning: skipping mermaid diagram %s: upload: %v\n", blockID, err)
			continue
		}
		am.Diagrams[blockID] = ir
		p.uploaded = append(p.uploaded, ir.DriveFileID)
	}

	return am, nil
}

// Cleanup deletes every Drive file the pipeline uploaded, unless
// Config.KeepTempImages is true.
func (p *AssetPipeline) Cleanup(ctx context.Context) error {
	if p.Config.KeepTempImages {
		return nil
	}
	var firstErr error
	for _, id := range p.uploaded {
		if err := p.Uploader.DeleteAsset(ctx, id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// collectIconRefs walks all slides, deduping IconRef values.
func collectIconRefs(slides []Slide) map[IconRef]struct{} {
	out := map[IconRef]struct{}{}
	var walkBlocks func([]Block)
	walkBlocks = func(blocks []Block) {
		for _, b := range blocks {
			switch v := b.(type) {
			case ParagraphBlock:
				for _, in := range v.Inlines {
					if r, ok := in.(IconRef); ok {
						out[r] = struct{}{}
					}
				}
			case BulletsBlock:
				for _, item := range v.Items {
					for _, in := range item.Inlines {
						if r, ok := in.(IconRef); ok {
							out[r] = struct{}{}
						}
					}
				}
			case HeadingBlock:
				for _, in := range v.Inlines {
					if r, ok := in.(IconRef); ok {
						out[r] = struct{}{}
					}
				}
			case ColumnsBlock:
				for _, col := range v.Columns {
					walkBlocks(col)
				}
			case IconRowsBlock:
				for _, row := range v.Rows {
					if row.Icon != nil {
						out[*row.Icon] = struct{}{}
					}
				}
			}
		}
	}
	for _, s := range slides {
		walkBlocks(s.Body)
	}
	return out
}

// collectDiagrams walks all slides for DiagramBlocks, returning {BlockID: source}.
func collectDiagrams(slides []Slide) map[string]string {
	out := map[string]string{}
	var walkBlocks func([]Block)
	walkBlocks = func(blocks []Block) {
		for _, b := range blocks {
			switch v := b.(type) {
			case DiagramBlock:
				out[v.BlockID] = v.Source
			case ColumnsBlock:
				for _, col := range v.Columns {
					walkBlocks(col)
				}
			}
		}
	}
	for _, s := range slides {
		walkBlocks(s.Body)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestAssetPipeline -v && go build ./...`
Expected: PASS, build PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_assets.go internal/cmd/slides_assets_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add asset pipeline orchestrator with cleanup

AssetPipeline.Resolve walks all slides for unique IconRefs and
DiagramBlocks, fetches/renders each, and uploads via the Uploader
abstraction. Per-asset failures warn-and-skip unless Strict. Cleanup
deletes the tracked Drive files unless KeepTempImages.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Drive uploader implementation

**Spec coverage:** §7.1 step 3 (Drive upload + permissions), §7.3 (cleanup).

**Files:**
- Modify: `internal/cmd/slides_assets.go`
- Modify: `internal/cmd/slides_assets_test.go`

- [ ] **Step 1: Implement the real Uploader**

Append to `internal/cmd/slides_assets.go`:

```go
import (
	"bytes"
	"google.golang.org/api/drive/v3"
	// existing imports retained
)

// DriveUploader implements Uploader by writing temporary files to Drive,
// granting public read access, and reading the WebContentLink. Mirrors
// the pattern in slides_add_slide.go.
type DriveUploader struct {
	Svc *drive.Service
}

func (d *DriveUploader) UploadAsset(ctx context.Context, name, mime string, body []byte) (ImageRef, error) {
	created, err := d.Svc.Files.Create(&drive.File{
		Name:     name,
		MimeType: mime,
	}).Media(bytes.NewReader(body)).Fields("id, webContentLink").Context(ctx).Do()
	if err != nil {
		return ImageRef{}, fmt.Errorf("upload %s: %w", name, err)
	}
	if _, err := d.Svc.Permissions.Create(created.Id, &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}).Context(ctx).Do(); err != nil {
		return ImageRef{}, fmt.Errorf("permission %s: %w", created.Id, err)
	}
	url := created.WebContentLink
	if url == "" {
		got, err := d.Svc.Files.Get(created.Id).Fields("webContentLink").Context(ctx).Do()
		if err != nil {
			return ImageRef{}, fmt.Errorf("get url for %s: %w", created.Id, err)
		}
		url = got.WebContentLink
	}
	return ImageRef{DriveFileID: created.Id, PublicURL: url}, nil
}

func (d *DriveUploader) DeleteAsset(ctx context.Context, fileID string) error {
	return d.Svc.Files.Delete(fileID).Context(ctx).Do()
}
```

- [ ] **Step 2: Add a smoke test that the type satisfies the interface**

Append to `internal/cmd/slides_assets_test.go`:

```go
func TestDriveUploaderSatisfiesUploader(t *testing.T) {
	var _ Uploader = (*DriveUploader)(nil)
}
```

- [ ] **Step 3: Run tests + build**

Run: `go test ./internal/cmd/ -run TestDriveUploaderSatisfiesUploader && go build ./...`
Expected: PASS, build PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/cmd/slides_assets.go internal/cmd/slides_assets_test.go
git commit -m "$(cat <<'EOF'
feat(slides): add Drive-backed Uploader implementation

DriveUploader wraps drive.Service to upload bytes, set anyone-reader
permission, and return the WebContentLink — mirrors the pattern in
slides_add_slide.go.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Renderer — single-body layouts (default + center) + speaker-notes wiring

**Spec coverage:** §6 (default/center), §8 (renderer + batching).

**Files:**
- Modify: `internal/cmd/slides_formatter.go` (replace stub)
- Create: `internal/cmd/slides_formatter_test.go`

- [ ] **Step 1: Write the failing test**

`internal/cmd/slides_formatter_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestRenderSlide`
Expected: FAIL — undefined `RenderSlides` and undefined notes-plan struct.

- [ ] **Step 3: Replace the stub renderer**

Replace `internal/cmd/slides_formatter.go` with:

```go
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
				ObjectId: slideID,
				SlideLayoutReference: &slides.LayoutReference{PredefinedLayout: "BLANK"},
			},
		})

		layout := MapSlideyLayout(slide.Frontmatter.Layout)

		// Title box (skipped for SectionHeader layouts — those put the
		// title in the body box at large size; see Task 16).
		if layout != LayoutKindSectionHeader && slide.Title != "" {
			reqs = append(reqs, renderTitleBox(slideID, i+1, slide.Title, g)...)
		}

		// Body — default/center for now (Task 16/17 add SectionHeader & columns).
		bodyText := blocksToPlainText(slide.Body)
		bodyID := fmt.Sprintf("body_%d", i+1)
		box := SingleBodyBox(g)
		reqs = append(reqs, createTextBox(bodyID, slideID, box))
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
// Created in Task 18; kept as a stub-with-real-signature here so the
// package still compiles for Tasks 15–17.
func CreatePresentationFromMarkdown(
	ctx context.Context,
	svc *slides.Service,
	title string,
	parent string,
	in []Slide,
) (*slides.Presentation, error) {
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
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cmd/ -run TestRenderSlide -v && go build ./...`
Expected: PASS, build PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_formatter.go internal/cmd/slides_formatter_test.go
git commit -m "$(cat <<'EOF'
feat(slides): replace stub renderer with single-body slide layout

RenderSlides emits CreateSlide + title box + body box for default and
center layouts. Returns a SlideNotesPlan slice consumed by the second
BatchUpdate (Task 18). Body text is the legacy "flatten blocks to one
string" form; columns and image insertion follow in Tasks 16/17.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Renderer — title/hero/statement and column layouts

**Spec coverage:** §6 (SectionHeader, two-cols, three-cols), §8 (renderer per-layout shape).

**Files:**
- Modify: `internal/cmd/slides_formatter.go`
- Modify: `internal/cmd/slides_formatter_test.go`

- [ ] **Step 1: Write the failing tests**

Append:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestRenderSlide -v`
Expected: hero/columns tests FAIL.

- [ ] **Step 3: Update `RenderSlides` to branch on `LayoutKind`**

Inside `RenderSlides`, replace the body-rendering block with this layout-aware version:

```go
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
```

Add helpers at the bottom of the file:

```go
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
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cmd/ -run TestRenderSlide -v && go build ./...`
Expected: PASS, build PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_formatter.go internal/cmd/slides_formatter_test.go
git commit -m "$(cat <<'EOF'
feat(slides): render hero/title/statement and 2/3-column layouts

SectionHeader-kind layouts emit one centered body box with the first
line styled at 44pt bold. two-cols/three-cols emit N side-by-side body
boxes positioned via the geometry helpers from Task 9.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: Renderer — diagrams + inline icons as images

**Spec coverage:** §8 (steps 4 inline icons, 5 diagrams).

**Files:**
- Modify: `internal/cmd/slides_formatter.go`
- Modify: `internal/cmd/slides_formatter_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestRenderSlide_DiagramEmitsCreateImage(t *testing.T) {
	bid := "block-test-1"
	s := Slide{
		Title: "T",
		Body:  []Block{DiagramBlock{Kind: "mermaid", Source: "graph TD\nA-->B", BlockID: bid}},
	}
	am := NewAssetMap()
	am.Diagrams[bid] = ImageRef{DriveFileID: "f1", PublicURL: "https://drive.example/f1"}

	reqs, _ := RenderSlides([]Slide{s}, am, defaultGeometry())
	var sawImage bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f1" {
			sawImage = true
		}
	}
	assert.True(t, sawImage)
}

func TestRenderSlide_BulletWithLeadingIconEmitsImage(t *testing.T) {
	icon := IconRef{Style: "solid", Name: "truck-fast"}
	s := Slide{
		Title: "T",
		Body: []Block{
			BulletsBlock{Items: []BulletItem{
				{Inlines: []Inline{icon, TextRun{Text: " Fulfilment"}}},
			}},
		},
	}
	am := NewAssetMap()
	am.Icons[icon] = ImageRef{DriveFileID: "f2", PublicURL: "https://drive.example/f2"}

	reqs, _ := RenderSlides([]Slide{s}, am, defaultGeometry())
	var sawIcon bool
	for _, r := range reqs {
		if r.CreateImage != nil && r.CreateImage.Url == "https://drive.example/f2" {
			sawIcon = true
		}
	}
	assert.True(t, sawIcon)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestRenderSlide_Diagram`
Expected: FAIL — current renderer ignores diagrams and icons.

- [ ] **Step 3: Update the renderer to emit `CreateImage` requests**

Pass `assets` into the per-slide rendering and add new branches.

In `RenderSlides`, change the loop body so each slide's branch can also emit images. After the body-text branches, add a sweep:

```go
		// Emit CreateImage for any diagram blocks on this slide.
		for _, b := range slide.Body {
			if d, ok := b.(DiagramBlock); ok {
				if ir, ok := assets.Diagrams[d.BlockID]; ok {
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
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cmd/ -run TestRenderSlide -v && go build ./...`
Expected: PASS, build PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/slides_formatter.go internal/cmd/slides_formatter_test.go
git commit -m "$(cat <<'EOF'
feat(slides): emit CreateImage for diagrams and bullet-leading icons

Diagram blocks render as full-width images below the title. Bullets
whose first inline is an IconRef get a small (18pt) icon image rendered
to the left of the body box, vertically aligned to the bullet line.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 18: Orchestrator — wire parser + asset pipeline + renderer + speaker notes

**Spec coverage:** §8 (two-pass BatchUpdate), §3 (CLI surface), §9 (dry-run/debug/error).

**Files:**
- Modify: `internal/cmd/slides_formatter.go` (replace stubbed `CreatePresentationFromMarkdown`)
- Modify: `internal/cmd/slides.go` (new flags + wiring)

- [ ] **Step 1: Replace `CreatePresentationFromMarkdown` with the real orchestrator**

In `internal/cmd/slides_formatter.go`, replace the `CreatePresentationFromMarkdown` stub:

```go
import (
	"google.golang.org/api/drive/v3"
	// existing imports retained
)

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
		reqs = append(reqs, buildSlidesClearAndInsertTextRequests(notesID, np.Text)...)
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
```

- [ ] **Step 2: Update `SlidesCreateFromMarkdownCmd` with the new flags + wiring**

In `internal/cmd/slides.go`, modify `SlidesCreateFromMarkdownCmd` to add the new flags and replace the body of `Run`:

```go
type SlidesCreateFromMarkdownCmd struct {
	Title          string `arg:"" name:"title" help:"Presentation title"`
	Content        string `name:"content" help:"Markdown content (inline)"`
	ContentFile    string `name:"content-file" help:"Read markdown content from file"`
	Parent         string `name:"parent" help:"Destination folder ID"`
	Debug          bool   `name:"debug" help:"Show debug output"`
	FAStyle        string `name:"fa-style" help:"Default Font Awesome style when shortcode has no prefix" default:"solid"`
	MMDC           string `name:"mmdc" help:"Path to mermaid CLI (mmdc); empty disables diagram rendering" default:"mmdc"`
	Strict         bool   `name:"strict" help:"Treat skipped FA/diagram assets as fatal"`
	KeepTempImages bool   `name:"keep-temp-images" help:"Don't delete temporary Drive uploads after import"`
	NoNotes        bool   `name:"no-notes" help:"Discard ## Notes sections instead of inserting as speaker notes"`
}

func (c *SlidesCreateFromMarkdownCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty title")
	}

	var markdown string
	switch {
	case c.ContentFile != "":
		data, err := os.ReadFile(c.ContentFile)
		if err != nil {
			return fmt.Errorf("read content file: %w", err)
		}
		markdown = string(data)
	case c.Content != "":
		markdown = c.Content
	default:
		return usage("either --content or --content-file is required")
	}

	if c.Debug {
		debugSlides = true
	}

	parsed, err := ParseMarkdownToSlides(markdown, ParseOptions{DefaultFAStyle: c.FAStyle})
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}

	slidesSvc, err := newSlidesService(ctx, account)
	if err != nil {
		return err
	}
	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	pipelineCfg := DefaultAssetPipelineConfig()
	pipelineCfg.MMDCPath = c.MMDC
	pipelineCfg.Strict = c.Strict
	pipelineCfg.KeepTempImages = c.KeepTempImages
	pipelineCfg.DefaultFAStyle = c.FAStyle

	opts := CreatePresentationFromMarkdownOptions{
		Title:         title,
		Parent:        c.Parent,
		Slides:        parsed,
		SlidesService: slidesSvc,
		DriveService:  driveSvc,
		Pipeline:      pipelineCfg,
		NoNotes:       c.NoNotes,
		DryRun:        flags.DryRun,
	}

	created, err := CreatePresentationFromMarkdownV2(ctx, opts)
	if err != nil {
		return err
	}
	if created != nil {
		u.Out().Printf("id\t%s", created.PresentationId)
		u.Out().Printf("title\t%s", created.Title)
	}
	return nil
}
```

**Note:** `flags.DryRun` is the existing global dry-run flag from `RootFlags` (verify the exact field name by checking `internal/cmd/root.go`; if it's named differently, e.g., `flags.Dry`, use that name verbatim).

- [ ] **Step 3: Build + run existing tests**

Run: `go build ./... && go test ./internal/cmd/...`
Expected: BUILD PASS. All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/cmd/slides_formatter.go internal/cmd/slides.go
git commit -m "$(cat <<'EOF'
feat(slides): wire slidey orchestrator and CLI flags

CreatePresentationFromMarkdownV2 runs the full pipeline:
create presentation → derive geometry → resolve assets → first
BatchUpdate → re-fetch → second BatchUpdate for notes → cleanup.

SlidesCreateFromMarkdownCmd gains --fa-style, --mmdc, --strict,
--keep-temp-images, --no-notes. Existing --dry-run prints the
would-be BatchUpdate JSON without any network for fetch/render.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 19: End-to-end fixture test

**Spec coverage:** §10 (end-to-end fixture).

**Files:**
- Create: `testdata/slidey/index.md`
- Create: `internal/cmd/slides_e2e_test.go`

- [ ] **Step 1: Copy the fixture**

```bash
mkdir -p testdata/slidey
cp ../univrs/slidey/slides/index.md testdata/slidey/index.md
git add testdata/slidey/index.md
```

- [ ] **Step 2: Write the failing test**

`internal/cmd/slides_e2e_test.go`:

```go
package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlideyFixture_ParsesAndRenders(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "slidey", "index.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	parsed, err := ParseMarkdownToSlides(string(data), ParseOptions{})
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
		var walk func([]Block)
		walk = func(blocks []Block) {
			for _, b := range blocks {
				switch v := b.(type) {
				case ParagraphBlock:
					for _, in := range v.Inlines {
						if _, ok := in.(IconRef); ok {
							sawIcon = true
						}
					}
				case BulletsBlock:
					for _, item := range v.Items {
						for _, in := range item.Inlines {
							if _, ok := in.(IconRef); ok {
								sawIcon = true
							}
						}
					}
				case IconRowsBlock:
					for _, row := range v.Rows {
						if row.Icon != nil {
							sawIcon = true
						}
					}
				case ColumnsBlock:
					for _, col := range v.Columns {
						walk(col)
					}
				case DiagramBlock:
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
```

- [ ] **Step 3: Run the e2e test**

Run: `go test ./internal/cmd/ -run TestSlideyFixture -v`
Expected: PASS. If it fails, the failure is a useful signal — fix the parser/renderer to handle whatever the fixture exposes; add focused tests for the broken case before fixing.

- [ ] **Step 4: Commit**

```bash
git add testdata/slidey/index.md internal/cmd/slides_e2e_test.go
git commit -m "$(cat <<'EOF'
test(slides): add slidey fixture deck end-to-end test

Copies univrs/slidey/slides/index.md into testdata and asserts the
parser produces all expected layout kinds, notes, icons, and diagrams,
then runs the renderer with a fake asset map.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 20: Docs + CHANGELOG

**Spec coverage:** §11 (docs/CHANGELOG updates).

**Files:**
- Modify: `docs/slides-markdown.md`
- Modify: `docs/commands/gog-slides-create-from-markdown.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Replace `docs/slides-markdown.md`**

Replace the **entire contents** of `docs/slides-markdown.md` with:

```markdown
# Google Slides from Markdown

`gog slides create-from-markdown` accepts both vanilla and slidey-flavored
markdown. Slidey conventions are documented here.

## Per-slide frontmatter

Each slide may begin with a YAML frontmatter block. Recognized keys:

| Key       | Values                                                | Behavior |
|-----------|-------------------------------------------------------|----------|
| `layout`  | `title`, `hero`, `statement`, `center`, `default`, `two-cols`, `three-cols` | Picks the slide's visual treatment. Unknown values fall back to `default`. |
| `content` | `wide`, `narrow`                                       | Parsed but not yet applied (Slides has fixed text-box widths). |

```
---
layout: hero
---

# univrs

Unfolding Nested Intent · Valid · Reliable · Safe
```

A bare `---` line is a slide separator unless it opens a frontmatter block
(see the design spec §4.1 for the exact disambiguation rule).

## Speaker notes

A trailing `## Notes` (or `### Notes`) section becomes the slide's speaker
notes. The heading and everything after it are removed from the body. FA
icon shortcodes inside notes are stripped to plain text.

```
## Topic

body

## Notes

- speaker hint one
- speaker hint two
```

## Font Awesome icons

Inline shortcodes `:fa-name:`, `:fas-name:`, `:far-name:`, `:fab-name:`,
`:fal-name:`, `:fad-name:` resolve to FA Free SVGs fetched from
`cdn.jsdelivr.net` and inserted as images. Style derivation:

| Prefix  | Resolved style |
|---------|----------------|
| `fa-`   | `--fa-style` (default `solid`) |
| `fas-`  | `solid`        |
| `far-`  | `regular`      |
| `fab-`  | `brands`       |
| `fal-`, `fad-` | `solid` (FA Free has no light/duotone) |

Icons placed at the start of a bullet item render as a small inline image
to the left of the bullet text. Mid-paragraph icons are dropped.

## Mermaid diagrams

Fenced code blocks tagged `mermaid` are rendered to PNG via the local
`mmdc` binary (configurable with `--mmdc`) and inserted as a full-width
image. If `mmdc` is missing, the diagram is skipped with a warning;
`--strict` makes it fatal.

## Multi-column layouts

```
::cols::

left column markdown

::col2::

middle / right column markdown

::col3::

third column markdown

::/cols::
```

`::right::` is accepted as a synonym for `::col2::` (slidey-style).

## ::boxes:: and ::arrows::

```
::boxes::
:fa-rectangle-ad: Campaigns
:fa-headset: Support Tickets
::/boxes::

::arrows::

### Step One

### Step Two

::/arrows::
```

Both render as bulleted lists in the body. Boxes use bullet glyphs;
arrows use `→`.
```

- [ ] **Step 2: Update flag table in `docs/commands/gog-slides-create-from-markdown.md`**

Append the new flag rows to the existing flag table (preserve the existing rows; add these at the appropriate alphabetical position):

```markdown
| `--fa-style` | `string` | solid | Default Font Awesome style when shortcode has no prefix |
| `--keep-temp-images` | `bool` |  | Don't delete temporary Drive uploads after import |
| `--mmdc` | `string` | mmdc | Path to mermaid CLI (mmdc); empty disables diagram rendering |
| `--no-notes` | `bool` |  | Discard `## Notes` sections instead of inserting as speaker notes |
| `--strict` | `bool` |  | Treat skipped FA/diagram assets as fatal |
```

- [ ] **Step 3: Add a CHANGELOG entry**

In `CHANGELOG.md`, under `## 0.17.0 - Unreleased` → `### Added`, append:

```markdown
- `slides create-from-markdown`: import slidey-flavored decks — per-slide
  YAML frontmatter (`layout:`, `content:`), `## Notes` speaker notes,
  Font Awesome icon shortcodes (jsDelivr CDN), mermaid diagrams (local
  `mmdc`), `::cols::`/`::col2::`/`::col3::`/`::right::` columns, and
  `::boxes::`/`::arrows::` icon-row blocks. New flags: `--fa-style`,
  `--mmdc`, `--strict`, `--keep-temp-images`, `--no-notes`.
```

- [ ] **Step 4: Build + final test sweep**

Run: `go build ./... && go vet ./... && go test ./internal/cmd/...`
Expected: BUILD PASS, VET PASS, all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/slides-markdown.md docs/commands/gog-slides-create-from-markdown.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
docs(slides): document slidey-flavored markdown import

Adds reference for per-slide frontmatter, ## Notes, FA shortcodes,
mermaid blocks, columns, and ::boxes::/::arrows::. Updates the flag
table for the new --fa-style/--mmdc/--strict/--keep-temp-images/
--no-notes flags. CHANGELOG entry under 0.17.0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review pass (verify before declaring complete)

After all 20 tasks complete, run this checklist before opening the PR:

1. `go build ./...` — passes.
2. `go vet ./...` — clean.
3. `go test ./internal/cmd/...` — all pass.
4. Manually run a dry-run end-to-end:

   ```bash
   ./gogcli slides create-from-markdown "Test Deck" \
     --content-file testdata/slidey/index.md --dry-run
   ```

   Expected: a JSON `BatchUpdatePresentationRequest` is printed to stdout with
   `gogcli://pending/...` placeholder URLs for icons and diagrams. No network.
5. Spec coverage check (every spec section maps to ≥1 task):
   - §3 CLI surface → Task 18
   - §4.1 frontmatter → Task 2
   - §4.2 title hoist → Task 8
   - §4.3 Notes → Task 8
   - §4.4 columns → Task 5
   - §4.5 boxes/arrows → Task 6
   - §4.6 FA shortcodes → Task 3
   - §4.7 mermaid → Task 7
   - §5 AST → Task 1
   - §6 layout mapping/geometry → Task 9
   - §7.1 FA pipeline → Tasks 10, 11
   - §7.2 mermaid pipeline → Task 12
   - §7.3 Drive cleanup → Task 13
   - §8 renderer + batching → Tasks 15, 16, 17, 18
   - §9 dry-run/debug/error → Task 18
   - §10 tests → Tasks 2–17 (per-task TDD), 19 (e2e fixture)
   - §11 files touched → all tasks; docs in Task 20
