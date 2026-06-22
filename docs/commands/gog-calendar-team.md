# `gog calendar team`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Show events for Workspace group members (service account, direct token, or ADC)

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
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--days` | `int` | 0 | Next N days |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--freebusy` | `bool` |  | Show only busy/free blocks (faster, single API call) |
| `--from` | `string` |  | Start time (RFC3339, date, or relative: now, today, tomorrow, monday) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max`<br>`--limit` | `int64` | 100 | Max events per calendar |
| `--no-dedup` | `bool` |  | Show each person's view without deduplication |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `-q`<br>`--query` | `string` |  | Filter events by title (case-insensitive) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--to` | `string` |  | End time (RFC3339, date, or relative: now, today, tomorrow, monday) |
| `--today` | `bool` |  | Today only |
| `--tomorrow` | `bool` |  | Tomorrow only |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--week` | `bool` |  | This week (uses --week-start, default Mon) |
| `--week-start` | `string` |  | Week start day for --week (sun, mon, ...) |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog calendar](gog-calendar.md)
- [Command index](README.md)
