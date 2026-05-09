# `gog gmail settings vacation update`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Update vacation responder settings

## Usage

```bash
gog gmail (mail,email) settings vacation update (edit,set) [flags]
```

## Parent

- [gog gmail settings vacation](gog-gmail-settings-vacation.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--body` | `string` |  | HTML body of the auto-reply message |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--contacts-only` | `bool` |  | Only respond to contacts |
| `--disable` | `bool` |  | Disable vacation responder |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `--domain-only` | `bool` |  | Only respond to same domain |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable` | `bool` |  | Enable vacation responder |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `--end` | `string` |  | End time in RFC3339 format (e.g., 2024-12-31T23:59:59Z) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--start` | `string` |  | Start time in RFC3339 format (e.g., 2024-12-20T00:00:00Z) |
| `--subject` | `string` |  | Subject line for auto-reply |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog gmail settings vacation](gog-gmail-settings-vacation.md)
- [Command index](README.md)
