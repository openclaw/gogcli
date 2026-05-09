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
- [gog slides copy](gog-slides-copy.md) - Copy a Google Slides presentation
- [gog slides create](gog-slides-create.md) - Create a Google Slides presentation
- [gog slides create-from-markdown](gog-slides-create-from-markdown.md) - Create a Google Slides presentation from markdown
- [gog slides create-from-template](gog-slides-create-from-template.md) - Create a presentation from template with text replacements
- [gog slides delete-slide](gog-slides-delete-slide.md) - Delete a slide by object ID
- [gog slides export](gog-slides-export.md) - Export a Google Slides deck (pdf|pptx)
- [gog slides info](gog-slides-info.md) - Get Google Slides presentation metadata
- [gog slides insert-text](gog-slides-insert-text.md) - Insert text into an existing page element (shape or table) by objectId
- [gog slides list-slides](gog-slides-list-slides.md) - List all slides with their object IDs
- [gog slides raw](gog-slides-raw.md) - Dump raw Google Slides API response as JSON (Presentations.Get; lossless; for scripting and LLM consumption)
- [gog slides read-slide](gog-slides-read-slide.md) - Read slide content: speaker notes, text elements, and images
- [gog slides replace-slide](gog-slides-replace-slide.md) - Replace the image on an existing slide in-place
- [gog slides replace-text](gog-slides-replace-text.md) - Find-and-replace text across a presentation
- [gog slides thumbnail](gog-slides-thumbnail.md) - Get or download a rendered thumbnail for a slide
- [gog slides update-notes](gog-slides-update-notes.md) - Update speaker notes on an existing slide

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog](gog.md)
- [Command index](README.md)
