# `gog drive upload`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Upload a file

## Usage

```bash
gog drive (drv) upload <localPath> [flags]
```

## Parent

- [gog drive](gog-drive.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--convert` | `bool` |  | Auto-convert to native Google format based on file extension (create only) |
| `--convert-to` | `string` |  | Convert to a specific Google format: doc\|sheet\|slides (create only) |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--keep-frontmatter` | `bool` |  | Keep YAML frontmatter (---) in Markdown when converting to a Google Doc (--convert or --convert-to doc; default: strip) |
| `--keep-revision-forever` | `bool` |  | Keep the new head revision forever (binary files only) |
| `--mime-type` | `string` |  | Override MIME type inference |
| `--name` | `string` |  | Override filename (create) or rename target (replace) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--parent` | `string` |  | Destination folder ID (create only) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--replace` | `string` |  | Replace the content of an existing Drive file ID (preserves shared link/permissions) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog drive](gog-drive.md)
- [Command index](README.md)
