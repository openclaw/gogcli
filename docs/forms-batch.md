---
title: Google Forms request batches
description: "Queue ordered Forms edits locally and submit one revision-locked atomic update."
---

# Google Forms request batches

`gog batch` queues Forms edits across commands, then submits one
`forms.batchUpdate` call. Google applies the requests in order and rejects the
entire update if any request is invalid.

```bash
BATCH_ID="$(gog --account you@example.com batch begin --form <formId> --name survey)"

gog --account you@example.com forms update <formId> --title 'Team survey' --batch "$BATCH_ID"
gog --account you@example.com forms add-question <formId> --title 'Your name' --batch "$BATCH_ID"
gog --account you@example.com forms questions add <formId> --title 'Your feedback' \
  --type paragraph --batch "$BATCH_ID"

gog batch show "$BATCH_ID" --json
gog batch end "$BATCH_ID" --dry-run --json
gog batch end "$BATCH_ID"
```

Choose exactly one target: `--form`, `--doc`, `--presentation`, or `--spreadsheet`. Forms accept
an ID or edit URL; optional `--service forms` must match the selected target.
`begin` records the target, account, and OAuth client without contacting Google.
The first queued mutation reads the form revision and initial item count.
Later commands append locally, and submission uses the recorded account/client.
Direct-token and ADC modes still use their active credential; an account label
does not verify that principal.

## Commands and positions

The `--batch` flag supports `forms add-question`, `delete-question`,
`move-question`, and `update`, including the `forms questions add/delete/move`
aliases. Omit the flag to submit immediately. An explicitly empty batch flag
returns a usage error and never falls back to an immediate write. Queue results
report `batch_id`, `form_id`, `queued`, and total `requests`.

Indices are zero-based and describe the form **after earlier queued requests**.
Default appends account for previous queued insertions and deletions. For
example, two appends to an empty form insert at indices 0 and 1. Moving or
deleting a question can reference an item added earlier in the same batch.
Indices count every form item, including existing non-question items.
Index calculation and queue persistence share one lock, so concurrent appends
receive consecutive positions. Google validates question content and settings
at submission; gog does not construct a virtual copy of the form.

Deletion still requires confirmation or `--force` when queued. Create, publish,
watch, response, and read commands do not accept `--batch`. Publishing uses a
separate API call and is not part of the content transaction. Set and verify
the desired publication state explicitly when that matters for a workflow.

## Submission and conflicts

Forms batches support atomic submission only, with gog's limit of 500 native
requests. `--auto-split` and `--continue-on-error` are rejected because skipping
a positional request can change which item a later operation addresses.
`forms update` can queue two native requests when changing both form information
and quiz settings.

Submission sends the original `writeControl.requiredRevisionId`. If the form
has changed, Google rejects the update and the complete batch remains on disk.
The revision is never silently refreshed. Review the current form, abort the old
batch, and queue a new one against the current revision when needed. Google
revision IDs are user-specific and only guaranteed valid for 24 hours; this
does not guarantee that the form stays unchanged for that duration.

Successful submissions remove the local batch. Failed submissions retain it.
Submission disables automatic retries and redirects: if a connection fails
after Google applies a request, inspect the form before retrying. Atomicity
does not make replaying an already successful creation batch idempotent.

`batch show` and `batch end --dry-run` preview the exact stored wire payload
without reading Google or changing local state. Individual mutation dry-runs
preview intent without queueing or resolving a default append index.

## Restricted command policies

Persisted requests can express any Forms content mutation. Under a restricted
runtime or baked policy, submission requires explicit `forms.batch-submit`
permission in addition to `batch.end`; parent grants alone do not suffice:

```bash
gog --enable-commands batch.end,forms.batch-submit batch end "$BATCH_ID"
```

`forms.batch-submit` is a permission capability, not a separate CLI command.
`--readonly` prevents submission. `gog schema --json` reports this permission
and whether the active policy allows Forms submission.

See [Docs batches](docs-batch.md#local-state) for private state-file permissions,
inspection, aborting, and pruning. Existing Docs and Slides batch files remain
compatible.
