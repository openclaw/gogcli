---
title: Live testing
description: "Run broad Google API smoke tests against a dedicated account, including recent feature workflows and cleanup."
---

# Live Testing

The repository's live suite exercises real Google APIs and cleans up disposable
resources. Use a dedicated test account:

```bash
make build
scripts/live-test.sh --account clawdbot@gmail.com
```

The account name must look like a test, bot, sandbox, QA, staging, or development
account unless `--allow-nontest` is supplied. `GOG_KEYRING_PASSWORD` must already
be available when the file keyring is active.

## Coverage

The default suite covers core output and schema behavior plus live Gmail,
Calendar, Drive, Docs, Sheets, Slides, Tasks, Contacts, People, and other
available services. Recent-feature coverage includes:

- Docs UTF-16 ranges, links, named ranges, comments locate/poll, orphan guards,
  literal anchors, tab-scoped Markdown/export, one-column and nested-list
  tables, persisted batches, direct table operations, URL images, and content
  enumerators.
- Sheets rich-text links, validation, structured tables, and table-aware
  dimension deletion.
- Slides local-image insertion with API read-back.
- Calendar attachment replacement and clearing.
- Drive shortcuts, revisions, and persisted changes polling.
- Gmail thread-aware drafts, attachment metadata preservation/clearing, and
  explicit thread archive semantics.
- CLI schema exit codes, Git-style help, output-mode precedence, and early
  validation errors.

Commands that return stable retryable exit code `8` are retried up to three
times by default. Set `GOG_LIVE_RETRIES` to change the attempt count. Created
Drive files, Docs batches, Calendar events, Gmail drafts/threads, and Photos
Picker sessions are also registered for best-effort cleanup if a later check
fails.

## Optional Coverage

Some APIs need account- or project-specific setup:

```bash
GOG_LIVE_CHAT_SPACE=spaces/... scripts/live-test.sh --account workspace@example.com
GOG_LIVE_GMAIL_WATCH_TOPIC=projects/.../topics/... scripts/live-test.sh --account bot@example.com
GOG_LIVE_CLASSROOM_COURSE=<courseId> scripts/live-test.sh --account bot@example.com
```

Photos Picker is tested when the account has the explicit `photospicker`
service:

```bash
gog auth add you@gmail.com --services photospicker
```

Google Chat attachments and Keep require Workspace accounts. Gmail watch pull
requires a configured Pub/Sub subscription. The suite reports these as skipped
instead of treating unavailable infrastructure as a product failure.

Use `--fast` to skip Docs, Sheets, and Slides, `--skip` for selected modules,
and `--strict` when optional service failures should fail the run.
