---
title: Google Docs request batches
description: "Build revision-locked Google Docs edits locally, inspect the exact API payload, and submit them atomically."
---

# Google Docs request batches

`gog batch` persists Google Docs API requests locally, then submits them with one revision-locked `documents.batchUpdate` call. The default `batch end` path is atomic: either every request is accepted or none is applied.

## Basic flow

```bash
BATCH_ID="$(gog --account you@example.com batch begin --service docs --doc <docId> --name "weekly update")"

gog --account you@example.com docs insert <docId> "Status: ready" --index 1 --batch "$BATCH_ID"
gog --account you@example.com docs format <docId> --match "Status: ready" --bold --batch "$BATCH_ID"

gog batch show "$BATCH_ID" --json
gog --dry-run batch end "$BATCH_ID" --json
gog batch end "$BATCH_ID"
```

`batch begin` prints only the UUID in text and plain modes, making command substitution stable. It records the selected account, OAuth client, and target document without reading the document or pinning a revision. The first queued mutation records the document revision because that is when request positions are first resolved. Later appends fail if the identity or revision differs.

## Supported mutations

The `--batch` flag is available on directly composable Docs mutations:

- `docs write` for plain text; replacement must be the first queued operation
- `docs update` for plain text
- `docs insert` and `docs delete`
- `docs format`
- `docs cell-style` and `docs table-column-width`
- `docs insert-person`, `docs insert-file-chip`, and `docs insert-date-chip`
- `docs insert-page-break`

Markdown writes and updates, page-layout changes, image insertion, table construction, and other multi-phase operations are intentionally excluded. They perform reads or side effects between writes and cannot honestly share one atomic Docs API request.

Ranges, anchors, tabs, and end-of-document positions are resolved against the live document when each command is queued. Earlier queued requests are not replayed locally during later position resolution. Prefer explicit stable indices, or queue position-sensitive operations from the end of the document toward the beginning.

## Submit and recovery modes

```bash
# Default: one atomic call, maximum 500 requests.
gog batch end "$BATCH_ID"

# Non-atomic: ordered chunks of at most 500 requests.
gog batch end "$BATCH_ID" --auto-split

# Recovery after an atomic HTTP 400 validation failure.
# Successful requests are removed; failed requests remain in the batch.
gog batch end "$BATCH_ID" --continue-on-error
```

`--auto-split` and `--continue-on-error` are explicit non-atomic modes and cannot be combined. Failed default submissions leave the complete batch intact. Split submissions persist remaining requests after every successful chunk.

Use `batch abort <batchId>` to discard a batch and `batch prune --older-than 72h` to remove stale batches.

## Local state

Batch files live under the state directory described in [Paths and State](paths.md), in the `batches/` subdirectory. Directories use mode `0700`; batch files and the lock file use mode `0600`. `batch list`, `batch show`, and `batch end --dry-run` read atomic state without creating the directory or lock file. Commands that create, queue, submit, abort, or prune batches retain the cross-process mutation lock.

Requests can contain document text, links, email addresses, and other content. Treat the state directory as sensitive. `batch show` exposes both request metadata and the exact wire payload.
