# `gog slides`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Slides

## Usage

```bash
gog slides (slide) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog slides add-slide](gog-slides-add-slide.md) - Add a slide with a full-bleed image and optional speaker notes
- [gog slides bullets](gog-slides-bullets.md) - Turn paragraph bullets on or off in one page element
- [gog slides copy](gog-slides-copy.md) - Copy a Google Slides presentation
- [gog slides create](gog-slides-create.md) - Create a Google Slides presentation
- [gog slides create-from-markdown](gog-slides-create-from-markdown.md) - Create a Google Slides presentation from markdown
- [gog slides create-from-template](gog-slides-create-from-template.md) - Create a presentation from template with text replacements
- [gog slides delete-slide](gog-slides-delete-slide.md) - Delete a slide by object ID
- [gog slides duplicate-slide](gog-slides-duplicate-slide.md) - Duplicate a slide by object ID
- [gog slides element](gog-slides-element.md) - Create and manipulate native page elements
- [gog slides export](gog-slides-export.md) - Export a Google Slides deck (pdf|pptx)
- [gog slides info](gog-slides-info.md) - Get Google Slides presentation metadata
- [gog slides insert-image](gog-slides-insert-image.md) - Insert a local or public image at a position and size
- [gog slides insert-text](gog-slides-insert-text.md) - Insert text into an existing page element (shape or table) by objectId
- [gog slides link](gog-slides-link.md) - Apply a hyperlink to a text range in one page element
- [gog slides list-slides](gog-slides-list-slides.md) - List all slides with their object IDs
- [gog slides locate](gog-slides-locate.md) - Locate text in shapes and table cells with object IDs and UTF-16 ranges
- [gog slides move-slide](gog-slides-move-slide.md) - Move a slide to a zero-based insertion index
- [gog slides new-slide](gog-slides-new-slide.md) - Create a native themed slide
- [gog slides raw](gog-slides-raw.md) - Dump raw Google Slides API response as JSON (Presentations.Get; lossless; for scripting and LLM consumption)
- [gog slides read-slide](gog-slides-read-slide.md) - Read slide content: speaker notes, text elements, and images
- [gog slides replace-slide](gog-slides-replace-slide.md) - Replace an existing slide image from a local file or public URL
- [gog slides replace-text](gog-slides-replace-text.md) - Find-and-replace text in an explicit object, slide, or presentation scope
- [gog slides style-text](gog-slides-style-text.md) - Apply range-scoped text styling to one page element
- [gog slides table](gog-slides-table.md) - Create and update native tables
- [gog slides thumbnail](gog-slides-thumbnail.md) - Get or download a rendered thumbnail for a slide
- [gog slides update-notes](gog-slides-update-notes.md) - Update speaker notes on an existing slide

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog](gog.md)
- [Command index](README.md)
