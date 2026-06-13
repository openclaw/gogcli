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
- [gog drive audit](gog-drive-audit.md) - Audit Drive sharing without mutation
- [gog drive bulk](gog-drive-bulk.md) - Bulk Drive permission operations
- [gog drive changes](gog-drive-changes.md) - Track Drive changes for sync and automation
- [gog drive comments](gog-drive-comments.md) - Manage comments on files
- [gog drive copy](gog-drive-copy.md) - Copy a file
- [gog drive delete](gog-drive-delete.md) - Move a file to trash (use --permanent to delete forever)
- [gog drive download](gog-drive-download.md) - Download a file (exports Google Docs formats)
- [gog drive drives](gog-drive-drives.md) - List shared drives (Team Drives)
- [gog drive du](gog-drive-du.md) - Summarize Drive folder sizes
- [gog drive get](gog-drive-get.md) - Get file metadata
- [gog drive inventory](gog-drive-inventory.md) - Export a read-only Drive inventory
- [gog drive labels](gog-drive-labels.md) - Read and modify Drive labels
- [gog drive ls](gog-drive-ls.md) - List files in a folder (default: root)
- [gog drive mkdir](gog-drive-mkdir.md) - Create a folder
- [gog drive move](gog-drive-move.md) - Move a file to a different folder
- [gog drive permissions](gog-drive-permissions.md) - List permissions on a file
- [gog drive raw](gog-drive-raw.md) - Dump raw Google Drive API response as JSON (Files.Get; lossless; for scripting and LLM consumption)
- [gog drive rename](gog-drive-rename.md) - Rename a file or folder
- [gog drive revisions](gog-drive-revisions.md) - List and inspect file revisions
- [gog drive search](gog-drive-search.md) - Full-text search across Drive
- [gog drive share](gog-drive-share.md) - Share a file or folder
- [gog drive shortcut](gog-drive-shortcut.md) - Manage shortcuts to Drive files and folders
- [gog drive tree](gog-drive-tree.md) - Print a read-only folder tree
- [gog drive unshare](gog-drive-unshare.md) - Remove a permission from a file
- [gog drive upload](gog-drive-upload.md) - Upload a file
- [gog drive url](gog-drive-url.md) - Print web URLs for files

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
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

- [gog](gog.md)
- [Command index](README.md)
