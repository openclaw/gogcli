---
title: Connected Sheets
description: Inspect, create, update, refresh, and safely delete Connected Sheets data sources and extracts.
---

# Connected Sheets

`gog sheets datasource` can inspect external data sources, add and update BigQuery sources, and refresh or delete one explicitly selected source. Its read-only commands list sources, return the complete source specification and execution status, discover anchored data-source tables (called extracts in the Sheets editor), and read a bounded number of extract rows.

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

## Add one BigQuery data source

```bash
gog --account you@example.com sheets datasource add <spreadsheetId> \
  --billing-project <your-billing-enabled-project> \
  --query 'SELECT 1 AS gog_probe' --dry-run --json

gog --account you@example.com sheets datasource add <spreadsheetId> \
  --billing-project <your-billing-enabled-project> \
  --table-project bigquery-public-data --dataset samples --table shakespeare --json
```

`--billing-project` is always required and must identify a BigQuery-enabled Google Cloud project with billing attached. Never substitute the global `--project` option, which selects JSON output fields instead. Choose exactly one source: `--query` for custom SQL, or `--dataset` plus `--table` for a native table. Omit `--table-project` when the table belongs to the billing project.

Adding a source creates its linked `DATA_SOURCE` sheet and immediately starts an asynchronous, potentially billable execution. Dry-run output identifies the billing project and query size but never echoes SQL; successful JSON reports only the new source ID, optional linked-sheet ID, and provider execution status. An immediate `FAILED` status preserves the new source ID in JSON and exits unsuccessfully so it can still be cleaned up.

## Update one BigQuery data source

```bash
gog --account you@example.com sheets datasource update <spreadsheetId> <dataSourceId> \
  --query 'SELECT 1 AS gog_probe' --dry-run --json

gog --account you@example.com sheets datasource update <spreadsheetId> <dataSourceId> \
  --billing-project <your-billing-enabled-project> --dataset replacement --table current --json
```

Only explicitly supplied fields are changed; the generated API field mask never overwrites an omitted billing project, query, dataset, table, or table project. SQL updates are allowed only for existing query sources, while native-table fields are allowed only for existing table sources. Query and table flags cannot be combined, and `--project` is rejected because it selects JSON output fields rather than the billing project.

Google immediately starts an asynchronous, potentially billable execution after updating. Dry-run and successful JSON report the changed field mask and execution status without revealing SQL; an immediate provider failure retains structured JSON and returns a nonzero exit code.

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

## Delete one data source

```bash
gog --account you@example.com sheets datasource delete \
  <spreadsheetId> <dataSourceId> --dry-run --json

gog --account you@example.com sheets datasource delete \
  <spreadsheetId> <dataSourceId> --force --json
```

Deleting a source also deletes its linked `DATA_SOURCE` sheet and unlinks dependent extracts, charts, and pivot tables; it does not delete those dependent objects. Interactive use asks for confirmation, noninteractive use refuses without `--force`, and `--force` skips only that confirmation. It never bypasses readonly restrictions, authorization checks, explicit targeting, or provider errors.

Dry-run output previews the exact one-source API request and its linked-object impact without authentication, prompts, or network access. The destructive request is never automatically retried.

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
