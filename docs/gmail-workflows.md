# Gmail Workflows

read_when:
- Working with Gmail content, filters, watches, labels, or agent-safe reads.
- Reviewing Gmail commands that cross from read-only into send or modify flows.

Gmail is one of gog's broadest surfaces. Use command-specific pages for exact
flags, and use this page to choose the right workflow shape.

## Search and Read

```bash
gog gmail search 'newer_than:7d' --max 10 --json
gog gmail get <messageId> --json
gog gmail thread get <threadId> --json
```

For agents, logs, or issue reports, prefer sanitized content:

```bash
gog gmail get <messageId> --sanitize-content --json
gog gmail thread get <threadId> --sanitize-content --json
```

`--sanitize-content` strips unsafe/raw payload details while keeping useful
message text for automation.

## Filters

Export filters as Gmail WebUI-compatible XML:

```bash
gog gmail settings filters export --out filters.xml
```

Keep API JSON when a script needs the Gmail API shape:

```bash
gog gmail settings filters export --format json --json
```

Command pages:

- [`gog gmail settings filters export`](commands/gog-gmail-settings-filters-export.md)
- [`gog gmail settings filters list`](commands/gog-gmail-settings-filters-list.md)
- [`gog gmail settings filters create`](commands/gog-gmail-settings-filters-create.md)
- [`gog gmail settings filters delete`](commands/gog-gmail-settings-filters-delete.md)

## Send Guardrails

Block send operations globally for one run:

```bash
gog --gmail-no-send gmail send --to you@example.com --subject test --text body
```

Or use the environment variable in agent shells:

```bash
export GOG_GMAIL_NO_SEND=1
```

For account-specific send blocking, use the no-send config commands:

- [`gog config no-send set`](commands/gog-config-no-send-set.md)
- [`gog config no-send list`](commands/gog-config-no-send-list.md)
- [`gog config no-send remove`](commands/gog-config-no-send-remove.md)

## Watches and Push

Gmail watch/PubSub workflows are documented in [Gmail watch](watch.md).

Key command pages:

- [`gog gmail watch start`](commands/gog-gmail-settings-watch-start.md)
- [`gog gmail watch serve`](commands/gog-gmail-settings-watch-serve.md)
- [`gog gmail watch renew`](commands/gog-gmail-settings-watch-renew.md)
- [`gog gmail history`](commands/gog-gmail-history.md)

## Email Tracking

Open tracking is documented in [Email Tracking](email-tracking.md) and
[Email Tracking Worker](email-tracking-worker.md).

## Raw Gmail

Use [`gog gmail raw`](commands/gog-gmail-raw.md) when you need the underlying
Gmail API `Message` object. See [Raw API Dumps](raw-api.md) for safety notes.
