# `gog gmail track setup`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Set up email tracking (deploy Cloudflare Worker)

## Usage

```bash
gog gmail (mail,email) track setup [flags]
```

## Parent

- [gog gmail track](gog-gmail-track.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email for API commands (gmail/calendar/chat/classroom/drive/docs/slides/contacts/tasks/people/sheets/forms/appscript/ads) |
| `--admin-key` | `string` |  | Admin key for /opens (generates one if omitted) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--db-name` | `string` |  | D1 database name (defaults to worker name) |
| `--deploy` | `bool` |  | Provision D1 + deploy the worker (requires wrangler) |
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
| `--tracking-key` | `string` |  | Tracking key (base64; generates one if omitted) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--worker-dir` | `string` |  | Worker directory (default: internal/tracking/worker) |
| `--worker-name` | `string` |  | Cloudflare Worker name (defaults to gog-email-tracker-<account>) |
| `--worker-url`<br>`--domain` | `string` |  | Tracking worker base URL (e.g. https://gog-email-tracker.<acct>.workers.dev) |

## See Also

- [gog gmail track](gog-gmail-track.md)
- [Command index](README.md)
