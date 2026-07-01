# `gog slides table cell style`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Style one zero-based table cell

## Usage

```bash
gog slides (slide) table cell style --row=INT-64 --col=INT-64 <presentationId> <tableObjectId> [flags]
```

## Parent

- [gog slides table cell](gog-slides-table-cell.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--bold` | `bool` |  | Set cell text bold |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--col` | `int64` |  | Zero-based column |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--content-align` | `*string` |  | Vertical content alignment |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--fill-color` | `string` |  | Cell fill as #RGB or #RRGGBB |
| `--fill-transparent` | `bool` |  | Remove the cell fill |
| `--font` | `string` |  | Cell text font family |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `--italic` | `bool` |  | Set cell text italic |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-bold` | `bool` |  | Clear cell text bold |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--no-italic` | `bool` |  | Clear cell text italic |
| `--no-underline` | `bool` |  | Clear cell text underline |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--range` | `string` |  | Optional UTF-16 text range as start:end; defaults to all cell text |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--row` | `int64` |  | Zero-based row |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--size` | `float64` |  | Cell text size in points |
| `--text-color` | `string` |  | Cell text color as #RGB or #RRGGBB |
| `--underline` | `bool` |  | Set cell text underline |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog slides table cell](gog-slides-table-cell.md)
- [Command index](README.md)
