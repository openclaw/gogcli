# `gog drive activity query`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Query Drive Activity API v2

## Usage

```bash
gog drive (drv) activity query (list,ls) [flags]
```

## Parent

- [gog drive activity](gog-drive-activity.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/sites/appscript/ads) |
| `--actions` | `string` |  | Comma-separated action filters (edit,create,delete,move,rename,restore,comment,share,label,dlp,reference,settings) |
| `--all`<br>`--all-pages`<br>`--allpages` | `bool` |  | Fetch all pages |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--consolidate` | `bool` |  | Use Drive Activity legacy consolidation strategy |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `--fail-empty`<br>`--non-empty`<br>`--require-results` | `bool` |  | Exit with code 3 if no activities |
| `--file`<br>`--file-id` | `string` |  | Drive file ID to query |
| `--filter` | `string` |  | Raw Drive Activity filter expression appended with AND |
| `--folder`<br>`--folder-id` | `string` |  | Drive folder ID; includes descendants |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from` | `string` |  | Lower activity time bound (RFC3339) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max`<br>`--limit` | `int64` | 10 | Page size |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--page`<br>`--cursor` | `string` |  | Page token |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--to` | `string` |  | Upper activity time bound (RFC3339) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog drive activity](gog-drive-activity.md)
- [Command index](README.md)
