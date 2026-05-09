# `gog forms`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Google Forms

## Usage

```bash
gog forms (form) <command> [flags]
```

## Parent

- [gog](gog.md)

## Subcommands

- [gog forms add-question](gog-forms-add-question.md) - Add a question to a form
- [gog forms create](gog-forms-create.md) - Create a form
- [gog forms delete-question](gog-forms-delete-question.md) - Delete a question by index
- [gog forms get](gog-forms-get.md) - Get a form
- [gog forms move-question](gog-forms-move-question.md) - Move a question to a new position
- [gog forms publish](gog-forms-publish.md) - Publish or unpublish a form
- [gog forms raw](gog-forms-raw.md) - Dump raw Google Forms API response as JSON (Forms.Get; lossless; for scripting and LLM consumption)
- [gog forms responses](gog-forms-responses.md) - Form responses
- [gog forms update](gog-forms-update.md) - Update form title, description, or settings
- [gog forms watch](gog-forms-watch.md) - Response watches (push notifications)

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
