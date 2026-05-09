# `gog drive`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Drive

## Usage

```bash
gog drive (drv) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog drive activity](gog-drive-activity.md) - Query Drive Activity audit events
- [gog drive changes](gog-drive-changes.md) - Track Drive changes for sync and automation
- [gog drive comments](gog-drive-comments.md) - Manage comments on files
- [gog drive copy](gog-drive-copy.md) - Copy a file
- [gog drive delete](gog-drive-delete.md) - Move a file to trash (use --permanent to delete forever)
- [gog drive download](gog-drive-download.md) - Download a file (exports Google Docs formats)
- [gog drive drives](gog-drive-drives.md) - List shared drives (Team Drives)
- [gog drive du](gog-drive-du.md) - Summarize Drive folder sizes
- [gog drive get](gog-drive-get.md) - Get file metadata
- [gog drive inventory](gog-drive-inventory.md) - Export a read-only Drive inventory
- [gog drive ls](gog-drive-ls.md) - List files in a folder (default: root)
- [gog drive mkdir](gog-drive-mkdir.md) - Create a folder
- [gog drive move](gog-drive-move.md) - Move a file to a different folder
- [gog drive permissions](gog-drive-permissions.md) - List permissions on a file
- [gog drive raw](gog-drive-raw.md) - Dump raw Google Drive API response as JSON (Files.Get; lossless; for scripting and LLM consumption)
- [gog drive rename](gog-drive-rename.md) - Rename a file or folder
- [gog drive search](gog-drive-search.md) - Full-text search across Drive
- [gog drive share](gog-drive-share.md) - Share a file or folder
- [gog drive tree](gog-drive-tree.md) - Print a read-only folder tree
- [gog drive unshare](gog-drive-unshare.md) - Remove a permission from a file
- [gog drive upload](gog-drive-upload.md) - Upload a file
- [gog drive url](gog-drive-url.md) - Print web URLs for files

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/sites/appscript/ads) |
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
