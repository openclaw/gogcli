# `gog backup push`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Export services into encrypted backup shards

## Usage

```bash
gog backup push [flags]
```

## Parent

- [gog backup](gog-backup.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--best-effort` | `bool` | true | Record optional service errors as backup rows and continue |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--config` | `string` |  | Backup config path |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `--drive-binary-contents` | `bool` |  | Include non-Google Drive binary file bytes in encrypted shards |
| `--drive-collaboration` | `bool` | true | Back up Drive permissions, comments, and revision metadata |
| `--drive-content-max-bytes` | `int64` | 0 | Skip individual Drive content exports larger than this many bytes; 0 means unlimited |
| `--drive-content-timeout` | `time.Duration` | 2m | Per-file Drive content export/download timeout |
| `--drive-contents` | `bool` | true | Download/export Drive file contents into encrypted shards |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-cache` | `bool` | true | Cache fetched Gmail raw messages locally so interrupted full backups can resume |
| `--gmail-checkpoint-interval` | `time.Duration` | 30m | Max time between Gmail checkpoint pushes during fetch; 0 disables time-triggered checkpoints |
| `--gmail-checkpoint-rows` | `int` | 10000 | Gmail messages per encrypted checkpoint chunk; 0 disables row-triggered checkpoints |
| `--gmail-checkpoints` | `bool` | true | Commit and push incomplete encrypted Gmail checkpoints during long cached fetches |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--gmail-refresh-cache` | `bool` |  | Refetch Gmail messages even when a local backup cache entry exists |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--identity` | `string` |  | Local age identity path |
| `--include-spam-trash` | `bool` | true | Include Gmail spam and trash |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max`<br>`--limit` | `int64` | 0 | Max Gmail messages to export; 0 means all |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--no-push` | `bool` |  | Commit locally but do not push to the remote |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--query` | `string` |  | Gmail query for bounded/test backups |
| `--recipient` | `[]string` |  | Public age recipient (repeatable) |
| `--remote` | `string` |  | Backup Git remote URL |
| `--repo` | `string` |  | Local backup repository path |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--services` | `string` | gmail | Comma-separated services to back up |
| `--shard-max-rows` | `int` | 1000 | Max rows per encrypted shard |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--workspace-max-files` | `int` | 0 | Max Docs/Sheets/Slides files per type for native Workspace metadata; 0 means all |
| `--workspace-native` | `bool` |  | Fetch full native Docs/Sheets/Slides API JSON in addition to Drive exports |

## See Also

- [gog backup](gog-backup.md)
- [Command index](README.md)
