# `gog classroom coursework create`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Create coursework

## Usage

```bash
gog classroom (class) coursework (work) create (add,new) --title=STRING <courseId> [flags]
```

## Parent

- [gog classroom coursework](gog-classroom-coursework.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--description` | `string` |  | Description |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--due` | `string` |  | Due date/time (RFC3339 or YYYY-MM-DD [HH:MM]) |
| `--due-date` | `string` |  | Due date (YYYY-MM-DD) |
| `--due-time` | `string` |  | Due time (HH:MM or HH:MM:SS) |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max-points` | `float64` |  | Max points |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--scheduled` | `string` |  | Scheduled publish time (RFC3339) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--state` | `string` |  | State: PUBLISHED, DRAFT |
| `--title` | `string` |  | Title |
| `--topic` | `string` |  | Topic ID |
| `--type` | `string` | ASSIGNMENT | Work type: ASSIGNMENT, SHORT_ANSWER_QUESTION, MULTIPLE_CHOICE_QUESTION |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog classroom coursework](gog-classroom-coursework.md)
- [Command index](README.md)
