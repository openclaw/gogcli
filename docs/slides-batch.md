---
title: Google Slides request batches
description: "Queue native Slides edits across commands and submit one revision-locked atomic update."
---

# Google Slides request batches

`gog batch` can queue Slides requests across CLI invocations, then submit them
with one `presentations.batchUpdate` call. Google applies requests in order and
validates the entire batch atomically. A later request can reference an object
created earlier in the batch, reducing both round trips and write-quota usage.

```bash
BATCH_ID="$(gog --account you@example.com batch begin --presentation <presentationId>)"

gog --account you@example.com slides element create-shape <presentationId> <slideId> \
  --object-id panel_1 --width 240 --height 80 --batch "$BATCH_ID"
gog --account you@example.com slides element style <presentationId> panel_1 \
  --fill-color '#3367d6' --batch "$BATCH_ID"
gog --account you@example.com slides insert-text <presentationId> panel_1 \
  'Quarterly revenue' --batch "$BATCH_ID"
gog --account you@example.com slides style-text <presentationId> panel_1 \
  --range 0:17 --bold --text-color '#ffffff' --batch "$BATCH_ID"

gog batch show "$BATCH_ID" --json
gog batch end "$BATCH_ID" --dry-run --json
gog batch end "$BATCH_ID"
```

Use exactly one target: `--presentation` for Slides or `--doc` for Docs.
`--service slides` is optional and must match the target when provided.
`begin` records the target, selected account, and OAuth client without reading
Google. The first queued mutation reads and pins the presentation revision.
Later commands append locally; changes made elsewhere are detected at submission.
Submission uses the account and client recorded in the batch, even if current
defaults differ. Direct-token and ADC authentication still use the active
credential; their account labels do not verify a principal.

## Supported commands

The `--batch` flag is available on:

- `slides new-slide`
- every `slides element` mutation, including creation, transforms, styling,
  grouping, z-order, alt text, and deletion
- `slides insert-text` without `--replace`, `style-text`, `link`, `bullets`,
  and `paragraph-style`
- every `slides table` mutation, including creation, cell text style, row and
  column operations, cell merging, and borders

Created objects receive stable IDs before submission. The queue result includes
those IDs together with `batch_id`, the number `queued`, and total `requests`.
Use the returned ID or an explicit `--object-id` to reference a queued element.
Table creation can therefore be followed by column sizing and cell edits before
the table exists remotely. `new-slide` likewise returns its `slideObjectId`.

Flag syntax is checked while queueing. Object existence, table bounds, and text
ranges are validated by Google against the ordered batch at submission. Direct
commands retain their normal live validation. An explicitly empty `--batch`
value is rejected; omit the flag for immediate execution. Destructive commands
retain their confirmation or `--force` requirement when queued.

`insert-text --replace` is excluded because its style-preserving replacement
depends on current text. Reads, image uploads, notes updates, template/Markdown
creation, replacement, and other slide lifecycle commands do not accept `--batch`.

## Submission and recovery

Default submission is atomic. A rejected update leaves the complete local batch
intact. Slides revisions require editor access, are bound to the authenticated
user, and are guaranteed valid for only 24 hours; stale batches may need to be
rebuilt against the current presentation.

`gog` limits persisted atomic submissions to 500 requests. Use `--auto-split` for
ordered chunks of at most 500, or `--continue-on-error` to try individual requests
after an atomic HTTP 400 failure. These modes are explicitly non-atomic and
cannot be combined. Successful requests are removed from local state; failures
remain, and retained failures produce a nonzero exit status. Response revisions
protect subsequent writes. Dependent requests can fail when an earlier creation
fails during individual recovery; prefer fixing and rebuilding the atomic batch.

Google counts one batch update as one write request. No automatic retry or
redirect replay is performed for Slides submission: after a lost response,
inspect the presentation before retrying the retained batch. A server may have
applied it even when the CLI did not receive its response.

`batch show` and `batch end --dry-run` inspect the exact stored payload without
authentication or mutation. Use `batch abort` to discard a queue or `batch prune`
to remove stale queues. State uses the same private directory and file permissions
as [Docs batches](docs-batch.md#local-state).

## Restricted command policies

Slides submission needs both the normal `batch end` permission and the exact
`slides.batch-submit` capability when command restrictions are configured:

```bash
gog --enable-commands-exact batch.end,slides.batch-submit batch end "$BATCH_ID"
```

The capability authorizes the whole Slides batch endpoint, including deletion;
individual command denials do not filter its payload. Parent `batch` or `slides`
grants, deny-only policies, and allow-all policies with denials do not implicitly
grant it. Unrestricted stock/full policies continue to permit submission, and
explicit service or capability denials win. The same rules apply to baked safety
profiles. `schema` reports this capability and effective submission availability
under `automation.safety.batch_services`.
