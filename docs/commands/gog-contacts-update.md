# `gog contacts update`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Update a contact

## Usage

```bash
gog contacts (contact) update (edit,set) <resourceName> [flags]
```

## Parent

- [gog contacts](gog-contacts.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--address` | `[]string` |  | Postal address (can be repeated; empty clears all) |
| `--birthday` | `string` |  | Birthday in YYYY-MM-DD (empty clears) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--custom` | `[]string` |  | Custom field as key=value (can be repeated; empty clears all) |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--email` | `string` |  | Email address (empty clears) |
| `--enable-commands` | `string` |  | Comma-separated list of enabled commands; dot paths allowed (restricts CLI) |
| `--family` | `string` |  | Family name |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from-file` | `string` |  | Update from contact JSON file (use - for stdin) |
| `--gender` | `string` |  | Gender value (empty clears) |
| `--given` | `string` |  | Given name |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--ignore-etag` | `bool` |  | Allow updating even if the JSON etag is stale (may overwrite concurrent changes) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--note` | `string` |  | Note/biography (empty clears) |
| `--notes` | `string` |  | Notes (stored as People API biography; empty clears) |
| `--org` | `string` |  | Organization/company name (empty clears) |
| `--phone` | `string` |  | Phone number (empty clears) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--relation` | `[]string` |  | Relation as type=person (can be repeated; empty clears all) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--title` | `string` |  | Job title (empty clears) |
| `--url` | `[]string` |  | URL (can be repeated; empty clears all) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |

## See Also

- [gog contacts](gog-contacts.md)
- [Command index](README.md)
