# `gog docs`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Docs (export via Drive)

## Usage

```bash
gog docs (doc) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog docs add-tab](gog-docs-add-tab.md) - Add a tab to a Google Doc
- [gog docs cat](gog-docs-cat.md) - Print a Google Doc as plain text
- [gog docs clear](gog-docs-clear.md) - Clear all content from a Google Doc
- [gog docs comments](gog-docs-comments.md) - Manage comments on files
- [gog docs copy](gog-docs-copy.md) - Copy a Google Doc
- [gog docs create](gog-docs-create.md) - Create a Google Doc
- [gog docs delete](gog-docs-delete.md) - Delete text range from document
- [gog docs delete-tab](gog-docs-delete-tab.md) - Delete a tab from a Google Doc
- [gog docs edit](gog-docs-edit.md) - Find and replace text in a Google Doc
- [gog docs export](gog-docs-export.md) - Export a Google Doc (pdf|docx|txt|md|html)
- [gog docs find-replace](gog-docs-find-replace.md) - Find and replace text. Supports plain text or markdown with images; use --first for a single occurrence.
- [gog docs format](gog-docs-format.md) - Apply text or paragraph formatting to a Google Doc
- [gog docs info](gog-docs-info.md) - Get Google Doc metadata
- [gog docs insert](gog-docs-insert.md) - Insert text at a specific position
- [gog docs list-tabs](gog-docs-list-tabs.md) - List all tabs in a Google Doc
- [gog docs raw](gog-docs-raw.md) - Dump raw Google Docs API response as JSON (Documents.Get; lossless; for scripting and LLM consumption)
- [gog docs rename-tab](gog-docs-rename-tab.md) - Rename a tab in a Google Doc
- [gog docs sed](gog-docs-sed.md) - Regex find/replace (sed-style: s/pattern/replacement/g)
- [gog docs structure](gog-docs-structure.md) - Show document structure with numbered paragraphs
- [gog docs update](gog-docs-update.md) - Insert text at a specific index in a Google Doc
- [gog docs write](gog-docs-write.md) - Write content to a Google Doc

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
