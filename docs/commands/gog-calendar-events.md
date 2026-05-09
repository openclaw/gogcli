# `gog calendar events`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

List events from a calendar or all calendars

## Usage

```bash
gog calendar (cal) events (list,ls) [<calendarId> ...] [flags]
```

## Parent

- [gog calendar](gog-calendar.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--all` | `bool` |  | Fetch events from all calendars |
| `--all-pages`<br>`--allpages` | `bool` |  | Fetch all pages |
| `--cal` | `[]string` |  | Calendar ID or name (can be repeated) |
| `--calendars` | `string` |  | Comma-separated calendar IDs, names, or indices from 'calendar calendars' |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--days` | `int` | 0 | Next N days (timezone-aware) |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `--fail-empty`<br>`--non-empty`<br>`--require-results` | `bool` |  | Exit with code 3 if no results |
| `--fields` | `string` |  | Comma-separated fields to return |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from` | `string` |  | Start time (RFC3339 with timezone, date, or relative: today, tomorrow, monday) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max`<br>`--limit` | `int64` | 10 | Max results |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--page`<br>`--cursor` | `string` |  | Page token |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--private-prop-filter` | `string` |  | Filter by private extended property (key=value) |
| `--query` | `string` |  | Free text search |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--shared-prop-filter` | `string` |  | Filter by shared extended property (key=value) |
| `--to` | `string` |  | End time (RFC3339 with timezone, date, or relative) |
| `--today` | `bool` |  | Today only (timezone-aware) |
| `--tomorrow` | `bool` |  | Tomorrow only (timezone-aware) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--week` | `bool` |  | This week (uses --week-start, default Mon) |
| `--week-start` | `string` |  | Week start day for --week (sun, mon, ...) |
| `--weekday` | `bool` | false | Include start/end day-of-week columns |

## See Also

- [gog calendar](gog-calendar.md)
- [Command index](README.md)
