---
title: Connected Sheets
description: Inspect and refresh BigQuery and Looker data sources, execution status, and anchored Connected Sheets extracts.
---

# Connected Sheets

`gog sheets datasource` can inspect external data sources and refresh one explicitly selected source. Its read-only commands list sources, return the complete source specification and execution status, discover anchored data-source tables (called extracts in the Sheets editor), and read a bounded number of extract rows.

## Authorize BigQuery access explicitly

Google's [Connected Sheets API guide](https://developers.google.com/workspace/sheets/api/guides/connected-sheets) requires `https://www.googleapis.com/auth/bigquery.readonly` in addition to Sheets API authorization whenever a response contains BigQuery Connected Sheets data. Ordinary `sheets` authorization intentionally does not request the BigQuery scope.

If the stored account does not already have Sheets API authorization and the BigQuery read-only scope, re-authorize it with its existing service selection, append the BigQuery scope, and force the consent screen. For a Sheets-only token:

```bash
gog auth add you@example.com \
  --services sheets \
  --extra-scopes https://www.googleapis.com/auth/bigquery.readonly \
  --force-consent
```

If the account token covers more services, keep that existing `--services` selection instead of narrowing it to `sheets`, including any narrower `--drive-scope` or `--gmail-scope` choices. Domain-wide delegated service accounts must also have both the Sheets read-only and BigQuery read-only scopes approved by the Workspace administrator; refresh instead requires writable Sheets authorization and the BigQuery read-only scope.

Looker data sources reuse the account's existing Looker link, but the same commands and output shape apply.

## List and describe data sources

```bash
gog --readonly --account you@example.com \
  sheets datasource list <spreadsheetId>

gog --readonly --account you@example.com \
  sheets datasource describe <spreadsheetId> <dataSourceId> --json
```

`list` returns a compact source summary joined with its `DATA_SOURCE` sheet and current `DataExecutionStatus`. It deliberately does not print custom SQL. `describe` returns the complete API `DataSource`, associated sheet properties, execution status, and refresh schedules, so its JSON can include a BigQuery raw query and error messages.

## Refresh one data source

```bash
gog --account you@example.com \
  sheets datasource refresh <spreadsheetId> <dataSourceId> --dry-run --json

gog --account you@example.com \
  sheets datasource refresh <spreadsheetId> <dataSourceId> --json

gog --account you@example.com \
  sheets datasource refresh <spreadsheetId> <dataSourceId> --force-refresh --json
```

Refresh requires writable Sheets authorization plus the explicitly granted `bigquery.readonly` scope. Only the requested source is refreshed; the command never refreshes an entire spreadsheet and never automatically retries a potentially billable execution. Use `--force-refresh` when retrying a source already in an error state. `--readonly` blocks the operation, while `--dry-run` previews it without authentication or network access.

Google starts refreshes asynchronously. JSON preserves the provider's native object references and execution statuses; an immediately `FAILED` status returns a nonzero exit code while retaining its structured JSON output. Poll `list` or `describe` until `state` is `SUCCEEDED` or `FAILED`.

## Discover and read extracts

A data-source table has no standalone ID in the Sheets API. Its definition lives only on the table's top-left anchor cell, so the CLI identifies extracts with an A1 anchor that includes the sheet name.

```bash
gog --readonly --account you@example.com \
  sheets datasource table list <spreadsheetId>

gog --readonly --account you@example.com \
  sheets datasource table describe <spreadsheetId> 'Extracts!B3' --json

gog --readonly --account you@example.com \
  sheets datasource table read <spreadsheetId> 'Extracts!B3' \
  --max-rows 250 --json
```

Table discovery asks `spreadsheets.get` only for anchor definitions and related sheet metadata. `read` then uses the selected table's configured columns and row limit to construct a bounded `spreadsheets.values.get` request. The default is at most 1,000 data rows plus the header; JSON output reports `truncated: true` when the configured extract can contain more rows. Use `--render FORMULA` or `--render UNFORMATTED_VALUE` when formatted display values are not suitable.

An extract that syncs every column keeps its column list on the linked `DATA_SOURCE` sheet rather than on the anchor, and the anchor lookup is range-scoped, so `read` issues one additional `spreadsheets.get` for those column definitions. Add pacing when reading many extracts in a loop; back-to-back reads can reach the Sheets per-minute quota.

These commands do not create, update, or delete data sources.
