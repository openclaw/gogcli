# `gog docs format`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Apply text or paragraph formatting to a Google Doc

## Usage

```bash
gog docs (doc) format <docId> [flags]
```

## Parent

- [gog docs](gog-docs.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--alignment` | `string` |  | Paragraph alignment: left, center, right, justify, start, end, justified |
| `--batch` | `string` |  | Append requests to a persisted Docs batch instead of submitting |
| `--bg-color` | `string` |  | Text background color as #RRGGBB or #RGB |
| `--bold` | `bool` |  | Set bold |
| `--bullet-preset` | `string` |  | Create a list with a Google Docs bullet glyph preset |
| `--bullets` | `bool` |  | Create a bulleted list with the default disc preset |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--code` | `bool` |  | Apply code style (Courier New + grey background) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--font-family` | `string` |  | Font family, for example Arial or Georgia |
| `--font-size` | `float64` |  | Font size in points |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--heading-level` | `*int` |  | Set paragraph named style to HEADING_1..HEADING_6 (shortcut for --named-style=HEADING_N) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `--indent-end` | `*float64` |  | Paragraph end indentation in points |
| `--indent-first-line` | `*float64` |  | Paragraph first-line indentation in points |
| `--indent-start` | `*float64` |  | Paragraph start indentation in points |
| `--italic` | `bool` |  | Set italic |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--keep-lines-together` | `*bool` |  | Keep all paragraph lines on one page or column when possible; use --no-keep-lines-together to clear |
| `--keep-with-next` | `*bool` |  | Keep the paragraph with the next paragraph when possible; use --no-keep-with-next to clear |
| `--line-spacing` | `float64` |  | Paragraph line spacing percentage, for example 100 or 150 |
| `--link` | `string` |  | Set hyperlink target (http://, https://, mailto:, #bookmarkId, or #heading-slug) |
| `--match` | `string` |  | Only format the first text match |
| `--match-all` | `bool` |  | Format all matches instead of only the first |
| `--match-case` | `bool` |  | Use case-sensitive matching with --match |
| `--named-style` | `string` |  | Set paragraph named style: NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6 |
| `--no-bold` | `bool` |  | Clear bold |
| `--no-bullets` | `bool` |  | Remove bullets or numbering |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--no-italic` | `bool` |  | Clear italic |
| `--no-link` | `bool` |  | Clear hyperlink |
| `--no-strikethrough`<br>`--no-strike` | `bool` |  | Clear strikethrough |
| `--no-underline` | `bool` |  | Clear underline |
| `--ordered` | `bool` |  | Create a numbered list with the default decimal preset |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--space-above` | `*float64` |  | Space above the paragraph in points |
| `--space-below` | `*float64` |  | Space below the paragraph in points |
| `--strikethrough`<br>`--strike` | `bool` |  | Set strikethrough |
| `--tab` | `string` |  | Target a specific tab by title or ID (see docs list-tabs) |
| `--text-color` | `string` |  | Text color as #RRGGBB or #RGB |
| `--underline` | `bool` |  | Set underline |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog docs](gog-docs.md)
- [Command index](README.md)
