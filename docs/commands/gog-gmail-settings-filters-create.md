# `gog gmail settings filters create`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Create a new email filter

## Usage

```bash
gog gmail (mail,email) settings filters create (add,new) [flags]
```

## Parent

- [gog gmail settings filters](gog-gmail-settings-filters.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--add-label` | `string` |  | Label(s) to add to matching messages (comma-separated, name or ID) |
| `--archive` | `bool` |  | Archive matching messages (skip inbox) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--forward` | `string` |  | Forward to this email address |
| `--from` | `string` |  | Match messages from this sender |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--has-attachment` | `bool` |  | Match messages with attachments |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--important` | `bool` |  | Mark as important |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--mark-read` | `bool` |  | Mark matching messages as read |
| `--never-spam` | `bool` |  | Never mark as spam |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--query` | `string` |  | Advanced Gmail search query for matching |
| `--remove-label` | `string` |  | Label(s) to remove from matching messages (comma-separated, name or ID) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--star` | `bool` |  | Star matching messages |
| `--subject` | `string` |  | Match messages with this subject |
| `--to` | `string` |  | Match messages to this recipient |
| `--trash` | `bool` |  | Move matching messages to trash |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog gmail settings filters](gog-gmail-settings-filters.md)
- [Command index](README.md)
