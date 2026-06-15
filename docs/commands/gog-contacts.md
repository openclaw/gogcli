# `gog contacts`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Contacts

## Usage

```bash
gog contacts (contact) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog contacts create](gog-contacts-create.md) - Create a contact
- [gog contacts dedupe](gog-contacts-dedupe.md) - Find likely duplicate contacts and optionally merge them
- [gog contacts delete](gog-contacts-delete.md) - Delete a contact
- [gog contacts directory](gog-contacts-directory.md) - Directory contacts
- [gog contacts export](gog-contacts-export.md) - Export contacts as vCard (.vcf)
- [gog contacts get](gog-contacts-get.md) - Get a contact
- [gog contacts list](gog-contacts-list.md) - List contacts
- [gog contacts other](gog-contacts-other.md) - Other contacts
- [gog contacts raw](gog-contacts-raw.md) - Dump raw People API response as JSON (People.Get; lossless; for scripting and LLM consumption)
- [gog contacts search](gog-contacts-search.md) - Search contacts by name/email/phone
- [gog contacts update](gog-contacts-update.md) - Update a contact

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
