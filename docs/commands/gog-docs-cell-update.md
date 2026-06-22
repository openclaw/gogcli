# `gog docs cell-update`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Replace or append content inside a specific table cell

## Usage

```bash
gog docs (doc) cell-update (update-cell) --row=INT --col=INT <docId> [flags]
```

## Parent

- [gog docs](gog-docs.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--append` | `bool` |  | Append inside the cell instead of replacing existing cell content |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--col` | `int` |  | 1-based column number |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--content` | `string` |  | Replacement content (omit when using --content-file) |
| `--content-file` | `string` |  | Read replacement content from a file |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--format` | `string` | markdown | Content format: markdown\|plain |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--row` | `int` |  | 1-based row number |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--tab` | `string` |  | Target a specific tab by title or ID (see docs list-tabs) |
| `--table-index` | `int` | 1 | 1-based table index in document order; negative indexes count from the end |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog docs](gog-docs.md)
- [Command index](README.md)
