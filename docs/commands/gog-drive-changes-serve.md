# `gog drive changes serve`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Receive Drive change notifications and run a local hook

## Usage

```bash
gog drive (drv) changes serve --state-file=STRING [flags]
```

## Parent

- [gog drive changes](gog-drive-changes.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--auto-renew` | `bool` |  | Create and renew the Drive notification channel |
| `--cert` | `string` |  | TLS certificate path; pair with --key (omit behind an HTTPS reverse proxy) |
| `--channel-token` | `string` |  | Expected X-Goog-Channel-Token value |
| `--channel-token-file` | `string` |  | Read the expected channel token from a file |
| `--channel-ttl` | `time.Duration` | 24h | Requested channel lifetime |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `--drive`<br>`--drive-id` | `string` |  | Shared drive ID for a shared-drive change log |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--filter-file` | `string` |  | Only invoke the hook for changes to this file ID |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `--include-removed` | `bool` | true | Include removed changes |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--key` | `string` |  | TLS private key path; pair with --cert |
| `--listen` | `string` | 127.0.0.1:8443 | Listen address |
| `--max`<br>`--limit` | `int64` | 100 | Max changes per API page |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--notification-timeout` | `time.Duration` | 5m | Maximum time for one callback, including Drive reads and the hook |
| `--on-change` | `string` |  | Trusted local shell command run for each non-empty change batch; event JSON is provided on stdin |
| `--path` | `string` | /drive-changes | Notification handler path |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--renew-before` | `time.Duration` | 10m | Renew this long before channel expiration |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--state-file` | `string` |  | JSON file that stores the current Drive page token and channel state |
| `--token` | `string` |  | Initial Drive page token when creating a new state file |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--webhook-url` | `string` |  | Public HTTPS callback URL used by --auto-renew |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog drive changes](gog-drive-changes.md)
- [Command index](README.md)
