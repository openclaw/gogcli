# `gog docs write`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Write content to a Google Doc

## Usage

```bash
gog docs (doc) write <docId> [flags]
```

## Parent

- [gog docs](gog-docs.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--alignment` | `string` |  | Paragraph alignment: left, center, right, justify, start, end, justified |
| `--append` | `bool` |  | Append instead of replacing the document body |
| `--batch` | `string` |  | Append requests to a persisted Docs batch instead of submitting |
| `--bg-color` | `string` |  | Text background color as #RRGGBB or #RGB |
| `--bold` | `bool` |  | Set bold |
| `--check-orphans` | `bool` |  | Block markdown replacement when open comment quotes would disappear |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--code` | `bool` |  | Apply code style (Courier New + grey background) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--file` | `string` |  | Text file path ('-' for stdin) |
| `--font-family` | `string` |  | Font family, for example Arial or Georgia |
| `--font-size` | `float64` |  | Font size in points |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--heading-level` | `*int` |  | Set paragraph named style to HEADING_1..HEADING_6 (shortcut for --named-style=HEADING_N) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `--italic` | `bool` |  | Set italic |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--line-spacing` | `float64` |  | Paragraph line spacing percentage, for example 100 or 150 |
| `--margin-bottom` | `string` |  | Set bottom page margin (points by default; supports pt, in, cm, mm) |
| `--margin-left` | `string` |  | Set left page margin (points by default; supports pt, in, cm, mm) |
| `--margin-right` | `string` |  | Set right page margin (points by default; supports pt, in, cm, mm) |
| `--margin-top` | `string` |  | Set top page margin (points by default; supports pt, in, cm, mm) |
| `--markdown` | `bool` |  | Convert markdown to Google Docs formatting (requires --replace or --append) |
| `--named-style` | `string` |  | Set paragraph named style: NORMAL_TEXT, TITLE, SUBTITLE, HEADING_1..HEADING_6 |
| `--no-bold` | `bool` |  | Clear bold |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--no-italic` | `bool` |  | Clear italic |
| `--no-strikethrough`<br>`--no-strike` | `bool` |  | Clear strikethrough |
| `--no-underline` | `bool` |  | Clear underline |
| `--page-height` | `string` |  | Set page height (points by default; supports pt, in, cm, mm) |
| `--page-size` | `string` |  | Named page size: A4, A5, Letter, Legal, Tabloid |
| `--page-width` | `string` |  | Set page width (points by default; supports pt, in, cm, mm) |
| `--pageless` | `bool` |  | Set document to pageless mode |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--replace` | `bool` |  | Replace all content explicitly (required with --markdown unless --append is set) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--strikethrough`<br>`--strike` | `bool` |  | Set strikethrough |
| `--tab` | `string` |  | Target a specific tab by title or ID (see docs list-tabs) |
| `--text` | `string` |  | Text to write |
| `--text-color` | `string` |  | Text color as #RRGGBB or #RGB |
| `--underline` | `bool` |  | Set underline |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog docs](gog-docs.md)
- [Command index](README.md)
