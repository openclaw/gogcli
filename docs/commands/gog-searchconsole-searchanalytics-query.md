# `gog searchconsole searchanalytics query`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Run a Search Analytics query

## Usage

```bash
gog searchconsole (gsc,search-console,webmasters) searchanalytics (analytics) query (run) <siteUrl> [flags]
```

## Parent

- [gog searchconsole searchanalytics](gog-searchconsole-searchanalytics.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--aggregation` | `string` |  | Aggregation type (AUTO,BY_PROPERTY,BY_PAGE,BY_NEWS_SHOWCASE_PANEL) |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--data-state` | `string` |  | Data state (FINAL,ALL,HOURLY_ALL) |
| `--dimensions` | `string` | QUERY | Comma-separated dimensions (DATE,QUERY,PAGE,COUNTRY,DEVICE,SEARCH_APPEARANCE,HOUR) |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--fail-empty`<br>`--non-empty`<br>`--require-results` | `bool` |  | Exit with code 3 if no rows |
| `--filter` | `[]string` |  | Dimension filter, repeatable: dimension:operator:expression |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from`<br>`--start` | `string` |  | Start date (YYYY-MM-DD) |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--max`<br>`--limit` | `int64` | 1000 | Max rows to return (1-25000) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--offset`<br>`--start-row` | `int64` | 0 | Row offset for pagination |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--request` | `string` |  | SearchAnalyticsQueryRequest JSON spec. Accepts @file, a plain file path, -, or inline JSON. |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--to`<br>`--end` | `string` |  | End date (YYYY-MM-DD) |
| `--type` | `string` | WEB | Search type (WEB,IMAGE,VIDEO,NEWS,DISCOVER,GOOGLE_NEWS) |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog searchconsole searchanalytics](gog-searchconsole-searchanalytics.md)
- [Command index](README.md)
