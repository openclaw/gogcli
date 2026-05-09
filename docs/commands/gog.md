# `gog`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google CLI for Gmail/Calendar/Chat/Classroom/Drive/Contacts/Tasks/Sheets/Docs/Slides/People/Forms/App Script/Ads/Groups/Admin/Keep

Config:
  file: /Users/steipete/Library/Application Support/gogcli/config.json
  keyring backend: file (source: config)

## Usage

```bash
gog <command> [flags]
```

## Subcommands

- [gog admin](gog-admin.md) - Google Workspace Admin (Directory API) - requires domain-wide delegation
- [gog agent](gog-agent.md) - Agent-friendly helpers
- [gog appscript](gog-appscript.md) - Google Apps Script
- [gog auth](gog-auth.md) - Auth and credentials
- [gog backup](gog-backup.md) - Encrypted Google account backups
- [gog calendar](gog-calendar.md) - Google Calendar
- [gog chat](gog-chat.md) - Google Chat
- [gog classroom](gog-classroom.md) - Google Classroom
- [gog completion](gog-completion.md) - Generate shell completion scripts
- [gog config](gog-config.md) - Manage configuration
- [gog contacts](gog-contacts.md) - Google Contacts
- [gog docs](gog-docs.md) - Google Docs (export via Drive)
- [gog download](gog-download.md) - Download a Drive file (alias for 'drive download')
- [gog drive](gog-drive.md) - Google Drive
- [gog exit-codes](gog-exit-codes.md) - Print stable exit codes (alias for 'agent exit-codes')
- [gog forms](gog-forms.md) - Google Forms
- [gog gmail](gog-gmail.md) - Gmail
- [gog groups](gog-groups.md) - Google Groups
- [gog keep](gog-keep.md) - Google Keep (Workspace only)
- [gog login](gog-login.md) - Authorize and store a refresh token (alias for 'auth add')
- [gog logout](gog-logout.md) - Remove a stored refresh token (alias for 'auth remove')
- [gog ls](gog-ls.md) - List Drive files (alias for 'drive ls')
- [gog me](gog-me.md) - Show your profile (alias for 'people me')
- [gog open](gog-open.md) - Print a best-effort web URL for a Google URL/ID (offline)
- [gog people](gog-people.md) - Google People
- [gog schema](gog-schema.md) - Machine-readable command/flag schema
- [gog search](gog-search.md) - Search Drive files (alias for 'drive search')
- [gog send](gog-send.md) - Send an email (alias for 'gmail send')
- [gog sheets](gog-sheets.md) - Google Sheets
- [gog slides](gog-slides.md) - Google Slides
- [gog status](gog-status.md) - Show auth/config status (alias for 'auth status')
- [gog tasks](gog-tasks.md) - Google Tasks
- [gog time](gog-time.md) - Local time utilities
- [gog upload](gog-upload.md) - Upload a file to Drive (alias for 'drive upload')
- [gog version](gog-version.md) - Print version
- [gog whoami](gog-whoami.md) - Show your profile (alias for 'people me')

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

- [Command index](README.md)
