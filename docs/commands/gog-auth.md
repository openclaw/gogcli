# `gog auth`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Auth and credentials

## Usage

```bash
gog auth <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog auth add](gog-auth-add.md) - Authorize and store a refresh token
- [gog auth alias](gog-auth-alias.md) - Manage account aliases
- [gog auth credentials](gog-auth-credentials.md) - Manage OAuth client credentials
- [gog auth doctor](gog-auth-doctor.md) - Diagnose auth, keyring, and refresh-token issues
- [gog auth keep](gog-auth-keep.md) - Configure service account for Google Keep (Workspace only)
- [gog auth keyring](gog-auth-keyring.md) - Configure keyring backend
- [gog auth list](gog-auth-list.md) - List stored accounts
- [gog auth manage](gog-auth-manage.md) - Open accounts manager in browser
- [gog auth remove](gog-auth-remove.md) - Remove a stored refresh token
- [gog auth service-account](gog-auth-service-account.md) - Configure service account (Workspace only; domain-wide delegation)
- [gog auth services](gog-auth-services.md) - List supported auth services and scopes
- [gog auth status](gog-auth-status.md) - Show auth configuration and keyring backend
- [gog auth tokens](gog-auth-tokens.md) - Manage stored refresh tokens

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

- [gog](gog.md)
- [Command index](README.md)
