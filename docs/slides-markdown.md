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

A bare `---` line is a slide separator unless it opens a contiguous YAML
header: key-value lines followed by a closing `---` before any blank/body line.

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
`cdn.jsdelivr.net`, rasterized locally with `rsvg-convert` or ImageMagick, and
inserted as PNG images. If no SVG rasterizer is available, icons are skipped
with a warning; `--strict` makes that fatal. Style derivation:

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
In `layout: two-cols` and `layout: three-cols` slides, `::col2::`,
`::col3::`, and `::right::` may also be used without an opening `::cols::`;
the content after the slide title becomes the first column.

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
