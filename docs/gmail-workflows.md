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

Use `--from-contact 'Ada Lovelace'` with `gmail search` to resolve a contact
into a sender query. If contact search misses, the fallback scans connections
page by page and retains only exact name or email matches. Multiple matching
contacts require a more specific selector; a repeated page token stops with a
pagination error instead of leaving the command stuck.

For agents, logs, or issue reports, prefer sanitized content:

```bash
gog gmail get <messageId> --sanitize-content --json
gog gmail thread get <threadId> --sanitize-content --json
```

`--sanitize-content` strips unsafe/raw payload details while keeping useful
message text for automation. Message JSON remains under the `message` key;
add `--results-only` to emit that sanitized message directly. Both shapes emit
the message headers and body once.

Thread and draft attachment downloads honor `--dry-run` before opening account
credentials, fetching messages, or writing files. Thread downloads keep their
current-directory default or explicit `--out-dir`; draft downloads retain the
existing configured attachment directory.

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

## Drafts Sent from Gmail Web

gog preserves long logical lines when building a plain-text draft from `--body`
or `--body-file`. Gmail web has been reported to insert hard line breaks when
it opens and sends such a draft without edits; see the
[before/after MIME comparison in #1058](https://github.com/openclaw/gogcli/issues/1058#issuecomment-5501187051).

For drafts you intend to review and send in Gmail web, the reported workaround
is to supply HTML paragraphs through `--body-html` or `--body-html-file`:

```bash
gog --account you@example.com gmail drafts create \
  --to recipient@example.com --subject "Draft for review" \
  --body-html '<p>One continuous paragraph that can reflow.</p><p>Another paragraph.</p>'
```

This creates an HTML draft; it does not change plain-text MIME behavior. Check
the draft and recipient-client rendering before relying on the workaround for
a particular workflow.

## Send Guardrails

Block send operations globally for one run:

```bash
gog --gmail-no-send gmail send --to you@example.com --subject test --body body
```

Or use the environment variable in agent shells:

```bash
export GOG_GMAIL_NO_SEND=1
```

For account-specific send blocking, use the no-send config commands:

- [`gog config no-send set`](commands/gog-config-no-send-set.md)
- [`gog config no-send list`](commands/gog-config-no-send-list.md)
- [`gog config no-send remove`](commands/gog-config-no-send-remove.md)

### Send an exact RFC822 message

Use raw mode when a trusted caller has already constructed and approved the
complete message, including its headers and MIME body:

```bash
gog --account you@example.com gmail send --raw-file approved.eml
cat approved.eml | gog --account you@example.com gmail send --raw-file - --thread-id <threadId>
```

Raw mode sends the input bytes unchanged. It cannot be combined with compose,
reply, attachment, signature, or tracking flags; `--thread-id` is the only
optional message setting, and Gmail thread URLs are accepted. The single `From`
address must match the authenticated account or a verified send-as alias;
checking aliases requires Gmail settings access, so a `--gmail-scope send` token
can send only from its own account. Direct access tokens and ADC require an
explicit `--account`. Read-only and global/per-account no-send policies still
apply. A dry-run validates the RFC822 structure without authentication and
reports only the source, byte count, SHA-256 digest, and optional thread ID.

Command page: [`gog gmail send`](commands/gog-gmail-send.md).

## Import an RFC822 Message

Import one existing RFC822/EML message from a file or stdin:

```bash
gog gmail import message.eml --label Imported --never-mark-spam
cat message.eml | gog gmail import - --internal-date-source receivedTime
```

Use `--dry-run` to parse the message and report its source, byte count, key
headers, labels, and import controls without authenticating or changing the
mailbox. `--internal-date-source dateHeader` (the default) asks Gmail to use a
valid `Date` header; `receivedTime` uses the import time. The optional
`--process-for-calendar` flag lets Gmail process calendar invitations.

This command imports one message through Gmail's normal delivery scanning and
classification. It does not fetch from IMAP, synchronize mailboxes, or
orchestrate bulk migrations.

Command page: [`gog gmail import`](commands/gog-gmail-import.md).

## Reply and Reply All

The Gmail API has no reply method. Clients fetch the original message, build a
complete RFC MIME message, and call `messages.send`. Use the first-class reply
commands so gog owns that composition work:

```bash
gog gmail reply <messageId> --body-file reply.txt
gog gmail reply-all <messageId> --body-file reply.txt \
  --bcc '"Introducer" <introducer@example.com>'
```

Reply defaults match normal Gmail composition:

- The original subject is inherited with one `Re:` prefix.
- The original message is quoted; use `--no-quote` to omit it.
- `reply` targets `Reply-To` when present, otherwise `From`.
- `reply-all` also carries forward original To/Cc recipients while excluding
  the active account and its send-as aliases.
- Display names are preserved.
- CID-backed inline images referenced by quoted HTML are fetched and rebuilt
  as `multipart/related`. If a referenced MIME part is missing, the command
  fails instead of sending broken images.

Recipient flags modify the derived recipient set. `--to`, `--cc`, and `--bcc`
are additive; naming an inherited recipient in a different field moves it
there. Repeat `--remove` to subtract recipients from every field:

```bash
gog gmail reply-all <messageId> --body "Thanks all" \
  --bcc introducer@example.com \
  --remove former-participant@example.com
```

An explicit `--subject` override is supported. A changed subject cannot meet
Gmail's thread-matching requirement, so gog keeps the RFC reply headers but
does not force the original `threadId`; Gmail creates a new conversation.

To stage a reply for review instead of sending it, use the draft-side
counterparts. They accept the same flags and build the same message; only the
finalize step differs (the draft is saved, not sent), so they work under
no-send guardrails:

```bash
gog gmail drafts reply <messageId> --body-file reply.txt
gog gmail drafts reply-all <messageId> --body "Thanks all"
```

## Forward

`gog gmail forward` sends a message on with a `Fwd:` subject, a Gmail-style
forwarded-message block, and the original attachments (skip them with
`--skip-attachments`). `gog gmail drafts forward` saves the same composition
as a draft instead; unlike the send side it does not require `--to`, matching
Gmail's UI, which allows an addressless forward draft:

```bash
gog gmail forward <messageId> --to colleague@example.com --note "FYI"
gog gmail drafts forward <messageId> --note "FYI"
```

Remote HTTP images remain remote references. Only MIME parts referenced with
`cid:` are copied into the outgoing message.

`gmail send --reply-to-message-id` remains available as lower-level
composition. It now inherits an omitted subject, but its explicit `--to` and
`--cc` values retain replacement semantics and quoting remains opt-in. Prefer
`gmail reply` or `gmail reply-all` for ordinary replies.

Official behavior references:

- [Gmail API: Manage threads](https://developers.google.com/workspace/gmail/api/guides/threads)
- [Gmail API: Create and send messages](https://developers.google.com/workspace/gmail/api/guides/sending)
- [RFC 2387: multipart/related](https://www.rfc-editor.org/rfc/rfc2387.html)
- [RFC 2392: Content-ID URLs](https://www.rfc-editor.org/rfc/rfc2392)

## Attachment Confirmation

`gmail send --json`, `gmail drafts create|update --json`, and
`gmail drafts reply|reply-all --json` include an `attachments` array when the
resulting message contains attachments:

```json
{"attachments":[{"filename":"report.pdf","size":2411233}]}
```

Sizes are reported in bytes. Draft updates report preserved attachments when
`--attach` is omitted; `--clear-attachments` removes them and omits the field.

## Watches and Pub/Sub

Gmail watch/PubSub workflows are documented in [Gmail watch](watch.md).

Key command pages:

- [`gog gmail watch start`](commands/gog-gmail-settings-watch-start.md)
- [`gog gmail watch serve`](commands/gog-gmail-settings-watch-serve.md)
- [`gog gmail watch pull`](commands/gog-gmail-settings-watch-pull.md)
- [`gog gmail watch renew`](commands/gog-gmail-settings-watch-renew.md)
- [`gog gmail history`](commands/gog-gmail-history.md)

## Email Tracking

Open tracking is documented in [Email Tracking](email-tracking.md) and
[Email Tracking Worker](email-tracking-worker.md).

## Raw Gmail

Use [`gog gmail raw`](commands/gog-gmail-raw.md) when you need the underlying
Gmail API `Message` object. See [Raw API Dumps](raw-api.md) for safety notes.
