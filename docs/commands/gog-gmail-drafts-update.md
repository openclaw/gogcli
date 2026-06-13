# `gog gmail drafts update`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Update a draft

## Usage

```bash
gog gmail (mail,email) drafts (draft) update (edit,set) <draftId> [flags]
```

## Parent

- [gog gmail drafts](gog-gmail-drafts.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--attach` | `[]string` |  | Attachment file path (repeatable). Replaces existing attachments; omit to preserve them, or use --clear-attachments to remove all. |
| `--bcc` | `string` |  | BCC recipients (comma-separated) |
| `--body` | `string` |  | Body (plain text; required unless --body-html is set) |
| `--body-file` | `string` |  | Body file path (plain text; '-' for stdin) |
| `--body-html` | `string` |  | Body (HTML; optional) |
| `--body-html-file` | `string` |  | HTML body file path ('-' for stdin) |
| `--cc` | `string` |  | CC recipients (comma-separated) |
| `--clear-attachments` | `bool` |  | Remove all attachments from the draft. By default, omitting --attach preserves the draft's existing attachments. |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from` | `string` |  | Send from this email address (must be a verified send-as alias) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--quote` | `bool` |  | Include quoted original message in reply |
| `--reply-to` | `string` |  | Reply-To header address |
| `--reply-to-message-id` | `string` |  | Reply to Gmail message ID (sets In-Reply-To/References and thread) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--subject` | `string` |  | Subject (required) |
| `--thread-id` | `string` |  | Reply within a Gmail thread (uses latest message for headers); overrides the draft's existing thread |
| `--to` | `*string` |  | Recipients (comma-separated; omit to keep existing) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog gmail drafts](gog-gmail-drafts.md)
- [Command index](README.md)
