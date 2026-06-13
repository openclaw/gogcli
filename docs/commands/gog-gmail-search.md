# `gog gmail search`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Search threads using Gmail query syntax

## Usage

```bash
gog gmail (mail,email) search (find,query,ls,list) <query> ... [flags]
```

## Parent

- [gog gmail](gog-gmail.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--all`<br>`--all-pages`<br>`--allpages` | `bool` |  | Fetch all pages |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--fail-empty`<br>`--non-empty`<br>`--require-results` | `bool` |  | Exit with code 3 if no results |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from-contact` | `string` |  | Resolve a Google Contact and add from:(email OR email) to the Gmail query |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--local` | `bool` |  | Use local timezone (default behavior, useful to override --timezone) |
| `--max`<br>`--limit` | `int64` | 10 | Max results |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--oldest` | `bool` |  | Show first message date instead of last |
| `--page`<br>`--cursor` | `string` |  | Page token |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `-z`<br>`--timezone` | `string` |  | Output timezone (IANA name, e.g. America/New_York, UTC). Default: GOG_TIMEZONE, config, then local |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog gmail](gog-gmail.md)
- [Command index](README.md)
