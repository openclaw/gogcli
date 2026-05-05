# `gog sheets`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Sheets

## Usage

```bash
gog sheets (sheet) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog sheets add-tab](gog-sheets-add-tab.md) - Add a new tab/sheet to a spreadsheet
- [gog sheets append](gog-sheets-append.md) - Append values to a range
- [gog sheets banding](gog-sheets-banding.md) - Manage alternating color banding
- [gog sheets chart](gog-sheets-chart.md) - Manage spreadsheet charts
- [gog sheets clear](gog-sheets-clear.md) - Clear values in a range
- [gog sheets conditional-format](gog-sheets-conditional-format.md) - Manage conditional formatting rules
- [gog sheets copy](gog-sheets-copy.md) - Copy a Google Sheet
- [gog sheets create](gog-sheets-create.md) - Create a new spreadsheet
- [gog sheets delete-tab](gog-sheets-delete-tab.md) - Delete a tab/sheet from a spreadsheet (use --force to skip confirmation)
- [gog sheets export](gog-sheets-export.md) - Export a Google Sheet (pdf|xlsx|csv) via Drive
- [gog sheets find-replace](gog-sheets-find-replace.md) - Find and replace text across a spreadsheet
- [gog sheets format](gog-sheets-format.md) - Apply cell formatting to a range
- [gog sheets freeze](gog-sheets-freeze.md) - Freeze rows and columns on a sheet
- [gog sheets get](gog-sheets-get.md) - Get values from a range
- [gog sheets insert](gog-sheets-insert.md) - Insert empty rows or columns into a sheet
- [gog sheets links](gog-sheets-links.md) - Get cell hyperlinks from a range
- [gog sheets merge](gog-sheets-merge.md) - Merge cells in a range
- [gog sheets metadata](gog-sheets-metadata.md) - Get spreadsheet metadata
- [gog sheets named-ranges](gog-sheets-named-ranges.md) - Manage named ranges
- [gog sheets notes](gog-sheets-notes.md) - Get cell notes from a range
- [gog sheets number-format](gog-sheets-number-format.md) - Apply number format to a range
- [gog sheets raw](gog-sheets-raw.md) - Dump raw Google Sheets API response as JSON (Spreadsheets.Get; lossless; for scripting and LLM consumption)
- [gog sheets read-format](gog-sheets-read-format.md) - Read cell formatting from a range
- [gog sheets rename-tab](gog-sheets-rename-tab.md) - Rename a tab/sheet in a spreadsheet
- [gog sheets resize-columns](gog-sheets-resize-columns.md) - Resize sheet columns
- [gog sheets resize-rows](gog-sheets-resize-rows.md) - Resize sheet rows
- [gog sheets table](gog-sheets-table.md) - Manage Google Sheets tables
- [gog sheets unmerge](gog-sheets-unmerge.md) - Unmerge cells in a range
- [gog sheets update](gog-sheets-update.md) - Update values in a range
- [gog sheets update-note](gog-sheets-update-note.md) - Set or clear a cell note

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
