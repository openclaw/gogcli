# `gog gmail settings`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Settings and admin

## Usage

```bash
gog gmail (mail,email) settings <command>
```

## Parent

- [gog gmail](gog-gmail.md)

## Subcommands

- [gog gmail settings autoforward](gog-gmail-settings-autoforward.md) - Auto-forwarding settings
- [gog gmail settings delegates](gog-gmail-settings-delegates.md) - Delegate operations
- [gog gmail settings filters](gog-gmail-settings-filters.md) - Filter operations
- [gog gmail settings forwarding](gog-gmail-settings-forwarding.md) - Forwarding addresses
- [gog gmail settings sendas](gog-gmail-settings-sendas.md) - Send-as settings
- [gog gmail settings vacation](gog-gmail-settings-vacation.md) - Vacation responder
- [gog gmail settings watch](gog-gmail-settings-watch.md) - Manage Gmail watch

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog gmail](gog-gmail.md)
- [Command index](README.md)
