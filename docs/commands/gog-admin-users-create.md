# `gog admin users create`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Create a new user

## Usage

```bash
gog admin users create (add,new) <email> [flags]
```

## Parent

- [gog admin users](gog-admin-users.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--admin` | `bool` |  | Not supported; assign admin roles separately after user creation |
| `--archived` | `bool` |  | Create user in archived state |
| `--change-password` | `bool` |  | Require password change on first login |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--family`<br>`--family-name`<br>`--last-name`<br>`--ln` | `string` |  | Family (last) name |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--given`<br>`--first-name`<br>`--fn`<br>`--given-name` | `string` |  | Given (first) name |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `--hash-function` | `string` |  | Password hash function when --password is pre-hashed (MD5, SHA-1, crypt) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--org-unit`<br>`--ou` | `string` |  | Organization unit path |
| `--password`<br>`--pass` | `string` |  | Initial password (generated if omitted) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--recovery-email` | `string` |  | Recovery email address |
| `--recovery-phone` | `string` |  | Recovery phone number in E.164 format |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--suspended` | `bool` |  | Create user in suspended state |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog admin users](gog-admin-users.md)
- [Command index](README.md)
