# Slidey-flavored markdown import for `gog slides create-from-markdown`

**Status:** Draft for review
**Date:** 2026-05-13
**Author:** Nicholas Reid (with Claude)
**Related code:** `internal/cmd/slides_markdown.go`, `internal/cmd/slides_formatter.go`, `internal/cmd/slides.go`
**Reference inputs:** `../univrs/slidey/DESIGN.md`, `../univrs/slidey/slides/index.md`

## 1. Goal

Extend `gog slides create-from-markdown` so it can faithfully import decks authored for the slidey Rust slide engine. The current parser handles only flat `## title / bullets / paragraphs / code` content; slidey decks use per-slide frontmatter, layout names, column markers, Font Awesome icon shortcodes, mermaid diagrams, and a trailing `## Notes` section for speaker notes.

The user's `slides/index.md` is the canonical example to satisfy.

## 2. Scope

### In scope

1. Per-slide YAML frontmatter (`layout:`, `content:`).
2. Trailing `## Notes` section per slide → Google Slides speaker notes.
3. Font Awesome shortcodes (`:fa-*:`, `:fas-*:`, `:far-*:`, `:fab-*:`) → SVG fetched from jsDelivr CDN, uploaded to Drive, inserted as image (SVG-direct, no local raster).
4. Mermaid fenced code blocks → rendered to PNG via local `mmdc` CLI, uploaded to Drive, inserted as image.
5. Column markers `::cols::`, `::col2::`, `::col3::`, `::right::`, `::/cols::`.
6. `::boxes::` and `::arrows::` blocks flattened to bulleted lists (one row per item; icon-prefix preserved).
7. Layouts: `title`, `hero`, `statement` → centered section-header; `center`, `default` → title + body; `two-cols`, `three-cols` → custom-positioned boxes on `BLANK`.

### Out of scope (deferred)

- `content: wide | narrow` — parsed and stored, ignored by the renderer this PR.
- KDL syntax highlighting — code blocks render as plain monospace.
- Mermaid rendering when `mmdc` is missing — skipped with warning (or fatal under `--strict`).
- D2 diagrams (slidey is migrating; not in `index.md` yet).
- Inline HTML tags (`<u-brand>`).
- PNG rasterization fallback for FA icons.

## 3. CLI surface

`gog slides create-from-markdown` keeps its existing flags (`--content`, `--content-file`, `--parent`, `--debug`, `--dry-run`). New flags:

| Flag | Default | Purpose |
|------|---------|---------|
| `--fa-style` | `solid` | Default style when shortcode is `:fa-x:` (no style prefix). `:fas-`/`:far-`/`:fab-` always win. |
| `--mmdc` | `mmdc` | Path/name of the mermaid CLI. Missing binary → mermaid blocks skipped with warning. |
| `--strict` | `false` | Treat any skipped asset (FA fetch fail, mermaid render fail) as fatal. |
| `--keep-temp-images` | `false` | Don't delete the Drive uploads for icons/diagrams after the presentation is built. |
| `--no-notes` | `false` | Discard `## Notes` sections instead of inserting them as speaker notes. |

Existing `--debug` gains additional output: parsed AST as JSON and per-stage asset-pipeline progress. Existing `--dry-run` runs parse + a stub asset pipeline (records what *would* be fetched) + render with placeholder URLs (`gogcli://pending/fa-truck-fast`); no network for fetch/render in dry-run.

## 4. Markdown grammar additions

### 4.1 Per-slide frontmatter

A `---` line followed immediately by `key: value` lines and a closing `---` is treated as that slide's frontmatter. Parsed with `gopkg.in/yaml.v3` (already in `go.mod`). Recognized keys: `layout`, `content`. Unknown keys retained on `Slide.Frontmatter.Raw` and ignored.

A bare `---` line that does *not* open a frontmatter block remains the slide separator (current behavior).

Disambiguation rule (deterministic, no lookahead-of-arbitrary-length):

1. A `---` at file start, or immediately following another `---` separator (with only blank lines between), opens a *frontmatter candidate*.
2. The next non-blank line must match `^[A-Za-z_][A-Za-z0-9_-]*:\s` (a YAML key). If not, the original `---` is treated as a slide separator and the candidate is abandoned.
3. From the candidate's opening `---`, scan forward; the first line that is exactly `---` (after trim) closes the frontmatter. If no closing `---` is found before EOF, the parser emits a fatal error naming the offending line.

### 4.2 Title hoisting

- Layouts `title`, `hero`, `statement`: the first `# h1` (or `## h2` if no h1) stays in body — no title hoisting. Body box renders the heading at large size.
- All other layouts: the first `# h1` is the slide title. If no h1 exists, fall back to the first `## h2`. (Back-compat with existing decks that use only h2.)

### 4.3 `## Notes` section

Hard-matched: a heading line whose trimmed text is exactly `Notes` (case-sensitive, level 2 or 3). Everything from that heading until the next slide separator becomes `Slide.Notes` as raw text. FA shortcodes inside notes are stripped to plain words (e.g. `:fa-truck-fast: Orders` → `Orders`). Diagrams inside notes are dropped.

### 4.4 Columns

```
::cols::

content of column 1

::col2::

content of column 2

::col3::

content of column 3

::/cols::
```

`::right::` is accepted as a synonym for `::col2::` (slidey allows both for the 2-col case). Three columns require either `two-cols`/`three-cols` layout or render on `default` as side-by-side text boxes (renderer infers column count from how many `::colN::` markers appear).

### 4.5 `::boxes::` and `::arrows::`

```
::boxes::
:fa-rectangle-ad: Campaigns
:fa-headset: Support Tickets
::/boxes::
```

Each line becomes an `IconRow{icon: optional, text: string}`. Rendered as a bulleted list — bullets use the icon image when available, plain bullet otherwise. `::arrows::` rows are rendered the same shape, just with an arrow glyph (`→`) prefix instead of a bullet.

### 4.6 Font Awesome shortcodes

Regex: `:fa[srlbd]?-[a-z0-9-]+:` matched anywhere in text.

Style derivation from prefix:

| Prefix | Style |
|--------|-------|
| `fa-` | `--fa-style` default (`solid`) |
| `fas-` | `solid` |
| `far-` | `regular` |
| `fab-` | `brands` |
| `fal-`, `fad-` | `solid` (FA Free has no light/duotone — substitute and warn once per icon) |

URL: `https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/<style>/<name>.svg`.

### 4.7 Mermaid fenced blocks

```` ```mermaid ... ``` ```` captured as `DiagramBlock{Kind: "mermaid", Source: ...}`. Other code-fence languages keep current code-block handling.

## 5. Internal AST

```go
type Slide struct {
    Frontmatter SlideFrontmatter
    Title       string
    Body        []Block
    Notes       string
}

type SlideFrontmatter struct {
    Layout  string            // "title"|"hero"|"center"|"default"|"two-cols"|"three-cols"|"statement"|""
    Content string            // "wide"|"narrow"|"" — parsed but not rendered this PR
    Raw     map[string]string // forward-compat for unknown keys
}

type Block interface { isBlock() }

type ParagraphBlock struct{ Text string; Inlines []Inline }
type BulletsBlock   struct{ Items []BulletItem; Ordered bool }
type CodeBlock      struct{ Lang, Source string }
type HeadingBlock   struct{ Level int; Text string; Inlines []Inline }
type ColumnsBlock   struct{ Columns [][]Block }   // 2 or 3 columns
type IconRowsBlock  struct{ Kind string; Rows []IconRow } // boxes|arrows
type DiagramBlock   struct{ Kind, Source string } // mermaid

type Inline interface { isInline() }
type TextRun struct{ Text string; Bold, Italic, Code bool }
type IconRef struct{ Style, Name string } // resolved at asset stage to ImageRef

type IconRow struct {
    Icon *IconRef // nil if line had no shortcode
    Text string
}

type BulletItem struct {
    Inlines []Inline
    Indent  int
}
```

After the asset pipeline runs, every `IconRef` and `DiagramBlock` is paired with an `ImageRef{ DriveFileID, PublicURL }` in a side map keyed by a stable block/inline ID. The parser stays pure; the renderer reads both.

## 6. Layout mapping (slidey → Google Slides)

| slidey layout | Google Slides treatment |
|---------------|-------------------------|
| `title` | `BLANK` + single centered text box; h1 at 44pt, subtitles below at 24pt. No title hoist. |
| `hero` | Same as `title`. |
| `statement` | Same shape as `title`/`hero` — large-text section break. |
| `center` | `TITLE_AND_BODY`, title + body centered. |
| `default` (or unset) | `TITLE_AND_BODY`, body left-aligned, body font 18pt regular (matches the existing renderer's defaults). |
| `two-cols` | `BLANK` + one title text box (top) + two body text boxes (50/50 split below). |
| `three-cols` | `BLANK` + one title text box + three body text boxes (33/33/33). |
| anything else | falls back to `default`. |

Column-box geometry uses the presentation's `PageSize` (read once after `Presentations.Create`, like `slides_add_slide.go` does). Geometry: 36pt outer margin, 24pt gutter; column widths = `(pageWidth − 2·margin − (n−1)·gutter) / n`. Title box height ≈ 100pt; body boxes start at `1.5 × 72pt` from top (matching current renderer).

## 7. Asset pipeline

Runs after parsing, before rendering. Returns `map[blockID]ImageRef` consumed by the renderer.

### 7.1 Font Awesome icons

1. Walk AST, collect unique `IconRef{Style, Name}` set.
2. For each, GET `https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6/svgs/<style>/<name>.svg`. 30s timeout, single retry on 5xx. Empty body or 404 → mark as failed.
3. **SVG-direct to Slides**: upload the SVG bytes to Drive (`Files.Create` with `MimeType: "image/svg+xml"` and `Media`), `Permissions.Create{Type: "anyone", Role: "reader"}`, read `WebContentLink`. Store as `ImageRef`.
4. After `Presentations.BatchUpdate` succeeds, defer-delete the Drive files (unless `--keep-temp-images`).

If any icon fetch fails: log a warning naming the slide and shortcode; the renderer drops the icon (keeps surrounding text). With `--strict`, the failure is fatal.

### 7.2 Mermaid diagrams

1. Walk AST, collect `DiagramBlock{Kind: "mermaid"}` set.
2. For each: write source to a temp file in `os.TempDir()`, run `<mmdc> -i <in.mmd> -o <out.png> -b transparent --scale 2`. 60s timeout.
3. If `mmdc` is missing on PATH or the command returns non-zero: log warning, mark block as `skipped`. With `--strict`, fail.
4. Upload PNG to Drive (same pattern as 7.1, `MimeType: "image/png"`), record `ImageRef`.

### 7.3 Drive cleanup

The pipeline tracks every Drive file ID it created. The orchestrator `defer`s a cleanup pass that calls `Files.Delete` for each. `--keep-temp-images` skips the deletion. Cleanup errors are logged but do not fail the command.

## 8. Renderer & batch update

The current `SlidesToAPIRequests` is replaced. Per-slide shape:

1. `CreateSlide{ObjectId: slide_<i>, PredefinedLayout: "BLANK"}`.
2. **Title box** (when hoisted): one `CreateShape{TEXT_BOX}` + `InsertText` + `UpdateTextStyle{Bold: true, FontSize: 28pt}`. Skipped for `title`/`hero`/`statement` (the h1 lives in the body box at 44pt).
3. **Body box(es)**: for single-column layouts, one full-width body box. For columns, N boxes side-by-side using the geometry from §6. Each body box's text is built by walking that column's `Block` list and emitting `InsertText` requests with running offsets, then `UpdateParagraphStyle` / `UpdateTextStyle` / `CreateParagraphBullets` as needed for bullets and emphasis.
4. **Inline icons**: when an icon appears at the start of a line that becomes a bullet item, render it as a small inline `CreateImage` (≈18pt square) positioned just left of the bullet text. Mid-paragraph icons are dropped (their surrounding text is preserved).
5. **Diagrams**: full-width `CreateImage` centered below the title, sized to the remaining slide height with aspect-ratio preserved.
6. **Speaker notes**: after the slide is created, re-fetch the presentation to get the notes object ID (existing pattern in `findSpeakerNotesObjectID` from `slides_shared.go`), then `InsertText` into it (existing pattern in `SlidesUpdateNotesCmd`).

### Batching

Two `Presentations.BatchUpdate` calls per import:

1. All `CreateSlide` / `CreateShape` / `CreateImage` / `InsertText` / `UpdateTextStyle` / `CreateParagraphBullets` requests for every slide.
2. Speaker-notes `InsertText` requests, after re-fetching the presentation to discover notes object IDs.

## 9. Dry-run, debug, error handling

- `--dry-run`: parse + asset-pipeline-stub (record what *would* be fetched) + render with placeholder URLs (e.g. `gogcli://pending/fa-truck-fast`). Print full `BatchUpdatePresentationRequest` JSON via existing `writeSlidesBatchUpdateDryRun`. No network.
- `--debug`: print parsed AST as JSON and per-stage asset-pipeline progress before the API call.
- Per-icon and per-diagram fetch/render failures are non-fatal (warn-and-skip) unless `--strict`.
- Frontmatter parse errors are fatal (give the user a clear pointer: file:line and the offending block).

## 10. Test strategy

- **Parser tests** (`slides_markdown_test.go`, new): table-driven, covering frontmatter (well-formed, missing close, unknown keys, separator-vs-frontmatter disambiguation), `## Notes` split, columns (2 and 3), `::right::` synonym, boxes/arrows, FA shortcode style derivation, mermaid fence.
- **Renderer tests** (`slides_formatter_test.go`, new): given a parsed `Slide` plus a fake `ImageRef` map, assert the emitted `[]*slides.Request` shape. No network.
- **Asset pipeline unit tests**: FA URL builder, mmdc command builder.
- **Asset pipeline integration tests**: behind `//go:build slidey_integration` build tag; talk to live jsDelivr.
- **End-to-end fixture**: `testdata/slidey/index.md` (copy of `../univrs/slidey/slides/index.md`) parsed → rendered → golden-compared `BatchUpdatePresentationRequest` JSON. Run with `go test ./internal/cmd/...`.

## 11. Files touched

### Modified

- `internal/cmd/slides_markdown.go` — replaced parser entrypoint, kept exported `ParseMarkdownToSlides` signature where possible.
- `internal/cmd/slides_formatter.go` — replaced renderer.
- `internal/cmd/slides.go` — new flags on `SlidesCreateFromMarkdownCmd`.
- `docs/slides-markdown.md` — document new grammar.
- `docs/commands/gog-slides-create-from-markdown.md` — flag table.
- `CHANGELOG.md` — entry under Unreleased / 0.17.0.

### New

- `internal/cmd/slides_markdown_ast.go` — AST types (§5).
- `internal/cmd/slides_markdown_frontmatter.go` — per-slide frontmatter parser.
- `internal/cmd/slides_assets.go` — FA fetch + mmdc + Drive upload + cleanup.
- `internal/cmd/slides_layout.go` — geometry / layout-mapping helpers.
- `internal/cmd/slides_markdown_test.go`
- `internal/cmd/slides_formatter_test.go`
- `internal/cmd/slides_assets_test.go`
- `internal/cmd/slides_layout_test.go`
- `testdata/slidey/index.md` — fixture copy of `../univrs/slidey/slides/index.md`.

## 12. Open risks

- **SVG-direct to Slides** is the user's choice. Documented caveat: if Slides rejects an edge-case SVG, the icon falls back to a warning + skip. We can add PNG rasterization later if it bites.
- **Inline icon positioning** is approximate (we don't measure text widths). Worst case: icon overlaps text by a few points. Acceptable for v1.
- **Two-pass `BatchUpdate`** doubles API round-trips. Mitigation: still cheaper than the per-slide round-trip pattern of `slides_add_slide.go`.
- **Live CDN dependency for FA**: tests behind a build tag avoid CI flake. End users get a clear warning if jsDelivr is unreachable.
