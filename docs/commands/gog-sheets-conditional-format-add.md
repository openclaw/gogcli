# `gog sheets conditional-format add`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Add a conditional formatting rule

## Usage

```bash
gog sheets (sheet) conditional-format (cf,conditional-formats) add (create,new) --type=STRING --format-json=STRING <spreadsheetId> <range> [flags]
```

## Parent

- [gog sheets conditional-format](gog-sheets-conditional-format.md)

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
| `--expr` | `string` |  | Expression value or custom formula (omit for blank/not-blank) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--format-fields` | `string` |  | Format field mask for force-sending zero/false fields (e.g. backgroundColor,textFormat.bold) |
| `--format-json` | `string` |  | CellFormat JSON (inline or @file) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--index` | `int64` | 0 | Insert rule at this priority index |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--type` | `string` |  | Rule type: text-eq\|text-contains\|text-starts-with\|text-ends-with\|number-eq\|number-gt\|number-gte\|number-lt\|number-lte\|blank\|not-blank\|custom-formula |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog sheets conditional-format](gog-sheets-conditional-format.md)
- [Command index](README.md)
