# `gog auth add`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Authorize and store a refresh token

## Usage

```bash
gog auth add <email> [flags]
```

## Parent

- [gog auth](gog-auth.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--auth-url` | `string` |  | Redirect URL from browser (manual flow; required for --remote --step 2) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `--drive-scope` | `string` | full | Drive scope mode: full\|readonly\|file |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--extra-scopes` | `string` |  | Comma-separated list of additional OAuth scope URIs to request (appended after service scopes) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--force-consent` | `bool` |  | Force consent screen to obtain a refresh token |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--gmail-scope` | `string` | full | Gmail scope mode: full\|readonly |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--listen-addr` | `string` |  | Address to listen on for OAuth callback (for example 0.0.0.0 or 0.0.0.0:8080) |
| `--manual` | `bool` |  | Browserless auth flow (paste redirect URL) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--redirect-host` | `string` |  | Hostname for OAuth callback in browser flows; builds https://{host}/oauth2/callback |
| `--redirect-uri` | `string` |  | Override OAuth redirect URI for manual/remote flows (for example https://host.example/oauth2/callback) |
| `--remote` | `bool` |  | Remote/server-friendly manual flow (print URL, then exchange code) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--services` | `string` | user | Services to authorize: user\|all-user or comma-separated gmail,calendar,chat,classroom,drive,driveactivity,drivelabels,docs,slides,contacts,tasks,sheets,people,forms,sites,meet,appscript,analytics,searchconsole,ads,youtube,photos; explicit opt-in: photospicker; all means all default user OAuth services. Workspace service-account-only services: admin, groups, keep |
| `--step` | `int` |  | Remote auth step: 1=print URL, 2=exchange code |
| `--timeout` | `time.Duration` |  | Authorization timeout (manual flows default to 5m) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog auth](gog-auth.md)
- [Command index](README.md)
