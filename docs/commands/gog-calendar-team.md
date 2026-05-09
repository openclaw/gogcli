# `gog calendar team`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Show events for all members of a Google Group

## Usage

```bash
gog calendar (cal) team <group-email> [flags]
```

## Parent

- [gog calendar](gog-calendar.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--days` | `int` | 0 | Next N days |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--freebusy` | `bool` |  | Show only busy/free blocks (faster, single API call) |
| `--from` | `string` |  | Start time (RFC3339, date, or relative: today, tomorrow, monday) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max`<br>`--limit` | `int64` | 100 | Max events per calendar |
| `--no-dedup` | `bool` |  | Show each person's view without deduplication |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `-q`<br>`--query` | `string` |  | Filter events by title (case-insensitive) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--to` | `string` |  | End time (RFC3339, date, or relative) |
| `--today` | `bool` |  | Today only |
| `--tomorrow` | `bool` |  | Tomorrow only |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--week` | `bool` |  | This week (uses --week-start, default Mon) |
| `--week-start` | `string` |  | Week start day for --week (sun, mon, ...) |

## See Also

- [gog calendar](gog-calendar.md)
- [Command index](README.md)
