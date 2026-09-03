# `gog adsense reports query`

> Generated from `gog schema --json`. Do not edit this page by hand; run `make docs-commands`.

Run an AdSense report

## Usage

```bash
gog adsense reports (report) query (run,generate) <account> [flags]
```

## Parent

- [gog adsense reports](gog-adsense-reports.md)

## Flags

| Flag | Type | Default | Help |
| --- | --- | --- | --- |
| `--access-token` | `string` |  | Use provided access token directly (bypasses stored refresh tokens; token expires in ~1h) |
| `-a`<br>`--account`<br>`--acct` | `string` |  | Account email, alias, or auto for authenticated Google API commands |
| `--client` | `string` |  | OAuth client name (selects stored credentials + token bucket) |
| `--color` | `string` | auto | Color output: auto\|always\|never |
| `--currency` | `string` |  | Currency code override (e.g. USD) |
| `--date-range` | `string` | LAST_7_DAYS | Named date range (TODAY,YESTERDAY,MONTH_TO_DATE,YEAR_TO_DATE,LAST_7_DAYS,LAST_30_DAYS); ignored if --from/--to set |
| `--dimensions` | `string` | DATE | Comma-separated report dimensions (e.g. DATE,COUNTRY_NAME) |
| `--disable-commands` | `string` |  | Comma-separated list of disabled commands; dot paths allowed |
| `-n`<br>`--dry-run`<br>`--dryrun`<br>`--noop`<br>`--preview` | `bool` |  | Do not make changes; print intended actions and exit successfully |
| `--enable-commands` | `string` |  | Comma-separated list of enabled command prefixes; dot paths allowed (restricts CLI) |
| `--enable-commands-exact` | `string` |  | Comma-separated list of exact enabled commands; dot paths allowed and parent commands do not enable children |
| `--fail-empty`<br>`--non-empty`<br>`--require-results` | `bool` |  | Exit with code 3 if no rows |
| `--filter` | `[]string` |  | Report filter, repeatable, passed through to the API (e.g. COUNTRY_NAME==United States) |
| `-y`<br>`--force`<br>`--assume-yes`<br>`--yes` | `bool` |  | Skip confirmations for destructive commands |
| `--from`<br>`--start` | `string` |  | Start date (YYYY-MM-DD); combine with --to for a custom range |
| `--gmail-no-send` | `bool` | false | Block Gmail send operations (agent safety) |
| `-h`<br>`--help` | `kong.helpFlag` |  | Show context-sensitive help. |
| `--home` | `string` |  | Override gogcli config/data/state/cache root (equivalent to GOG_HOME) |
| `-j`<br>`--json`<br>`--machine` | `bool` | false | Output JSON to stdout (best for scripting) |
| `--language` | `string` |  | Language code for headers (e.g. en-US) |
| `--max`<br>`--limit` | `int64` |  | Max rows to return |
| `--metrics` | `string` | ESTIMATED_EARNINGS,CLICKS,IMPRESSIONS | Comma-separated report metrics (e.g. ESTIMATED_EARNINGS,CLICKS,IMPRESSIONS) |
| `--no-input`<br>`--non-interactive`<br>`--noninteractive` | `bool` |  | Never prompt; fail instead (useful for CI) |
| `--order-by` | `[]string` |  | Sort order, repeatable (e.g. -DATE for descending) |
| `-p`<br>`--plain`<br>`--tsv` | `bool` | false | Output stable, parseable text to stdout (TSV; no colors) |
| `--quota-project` | `string` |  | Google Cloud project to bill for API usage (sent as X-Goog-User-Project; some APIs require it with --access-token or ADC) |
| `--readonly` | `bool` | false | Block mutating API requests at runtime; auth add also requests read-only OAuth scopes |
| `--results-only` | `bool` |  | In JSON mode, emit only the primary result (drops envelope fields like nextPageToken) |
| `--select`<br>`--pick`<br>`--project` | `string` |  | In JSON mode, select comma-separated fields (best-effort; supports dot paths). Desire path: use --fields for most commands. |
| `--timezone` | `string` |  | Reporting timezone: ACCOUNT_TIME_ZONE (default) or GOOGLE_TIME_ZONE (America/Los_Angeles) |
| `--to`<br>`--end` | `string` |  | End date (YYYY-MM-DD); combine with --from for a custom range |
| `-v`<br>`--verbose` | `bool` |  | Enable verbose logging |
| `--version` | `kong.VersionFlag` |  | Print version and exit |
| `--wrap-untrusted` | `bool` | false | In JSON/raw output, wrap fetched text fields in external untrusted-content markers |

## See Also

- [gog adsense reports](gog-adsense-reports.md)
- [Command index](README.md)
