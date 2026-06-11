# `gog docs comments`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Manage comments on files

## Usage

```bash
gog docs (doc) comments <command>
```

## Parent

- [gog docs](gog-docs.md)

## Subcommands

- [gog docs comments add](gog-docs-comments-add.md) - Add a comment to a Google Doc
- [gog docs comments delete](gog-docs-comments-delete.md) - Delete a comment
- [gog docs comments get](gog-docs-comments-get.md) - Get a comment by ID
- [gog docs comments list](gog-docs-comments-list.md) - List comments on a Google Doc
- [gog docs comments locate](gog-docs-comments-locate.md) - Resolve a comment quote to Docs API index ranges
- [gog docs comments poll](gog-docs-comments-poll.md) - Poll new and modified comments with persisted state
- [gog docs comments reopen](gog-docs-comments-reopen.md) - Reopen a previously resolved comment
- [gog docs comments reply](gog-docs-comments-reply.md) - Reply to a comment
- [gog docs comments resolve](gog-docs-comments-resolve.md) - Resolve a comment (mark as done)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/drivelabels/docs/slides/contacts/tasks/people/sheets/forms/sites/appscript/analytics/searchconsole/youtube/photos) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog docs](gog-docs.md)
- [Command index](README.md)
