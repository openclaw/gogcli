# `gog gmail settings sendas`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Send-as settings

## Usage

```bash
gog gmail (mail,email) settings sendas <command>
```

## Parent

- [gog gmail settings](gog-gmail-settings.md)

## Subcommands

- [gog gmail settings sendas create](gog-gmail-settings-sendas-create.md) - Create a new send-as alias
- [gog gmail settings sendas delete](gog-gmail-settings-sendas-delete.md) - Delete a send-as alias
- [gog gmail settings sendas get](gog-gmail-settings-sendas-get.md) - Get details of a send-as alias
- [gog gmail settings sendas list](gog-gmail-settings-sendas-list.md) - List send-as aliases
- [gog gmail settings sendas update](gog-gmail-settings-sendas-update.md) - Update a send-as alias
- [gog gmail settings sendas verify](gog-gmail-settings-sendas-verify.md) - Resend verification email for a send-as alias

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

- [gog gmail settings](gog-gmail-settings.md)
- [Command index](README.md)
