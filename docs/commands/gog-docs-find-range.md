# `gog docs find-range`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Find text and print Docs API UTF-16 index ranges

## Usage

```bash
gog docs (doc) find-range <docId> <text> [flags]
```

## Parent

- [gog docs](gog-docs.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--all` | `bool` |  | Return all matches |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--fail-empty`<br>`--non-empty`<br>`--require-results` | `bool` |  | Exit with code 3 if no matches |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--match-case` | `bool` |  | Use case-sensitive matching |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--normalize-whitespace` | `bool` | true | Collapse whitespace while matching |
| `--occurrence` | `*int` |  | Return the Nth occurrence (1-based; default first) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--segment` | `string` |  | Target an exact header, footer, or footnote segment ID |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--tab` | `string` |  | Target a specific tab by title or ID (see docs list-tabs) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog docs](gog-docs.md)
- [Command index](README.md)
