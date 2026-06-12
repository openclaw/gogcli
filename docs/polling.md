---
summary: "Persisted Drive changes and Docs comment polling"
read_when:
  - Running gog as a local Drive or Docs event poller
  - Wiring Drive changes or Docs comments to shell hooks
---

# Drive and Docs polling

`gog` can poll Drive changes or Google Docs comments while persisting the API
cursor in a local JSON file:

```bash
gog drive changes poll \
  --state-file ~/.local/state/gog/drive-changes.json \
  --interval 30s \
  --json

gog docs comments poll <docId> \
  --state-file ~/.local/state/gog/doc-comments.json \
  --interval 30s \
  --json
```

Both commands poll immediately, then wait for `--interval`. Use
`--max-iterations N` for bounded jobs and tests. `SIGINT` and `SIGTERM` stop the
poller; completed iterations have already persisted their cursor.

## State

State files are written atomically with mode `0600`. Missing and empty state
files initialize without replaying existing history:

- Drive stores a fresh start page token before its first changes request.
- Docs stores the current time as its initial comment watermark.

Drive state is scoped to `--drive`. Docs state is scoped to the document and
the `--include-resolved` setting. Delete the state file or choose a new path to
start a new stream. Run only one poller against a state file; concurrent writers
can overwrite each other's cursor.

The Drive comments API time filter is inclusive. State therefore records both
the latest timestamp and comment IDs already delivered at that timestamp.
Comments that share a modified time are delivered once without moving the
watermark past an unseen peer.

## Output

With `--json`, stdout is newline-delimited JSON: one object per non-empty Drive
batch or Docs comment. Plain and human modes emit one tab-separated line per
change or comment. Empty polls produce no stdout.

`drive changes poll --filter-file <fileId>` filters emitted changes and hook
payloads, while the underlying Drive page token still advances.

## Shell hooks

Hooks are explicit trusted local shell commands:

```bash
gog drive changes poll \
  --state-file drive.json \
  --on-change './handle-drive-batch'

gog docs comments poll <docId> \
  --state-file comments.json \
  --on-new './handle-comment'
```

Payload JSON is passed on stdin. Google-provided text is never interpolated
into the command string. Hook stdout and stderr go to `gog` stderr so event
stdout remains parseable.

Hooks run through the platform shell and are not sandboxed. Use only fixed,
operator-controlled commands; do not build the hook string from Google content.

Drive invokes `--on-change` once per non-empty filtered batch. Docs invokes
`--on-new` once per comment, in modified-time and comment-ID order. Hooks run
sequentially.

State advances only after output and all hooks succeed. Output or hook failure
returns an error and retains the previous cursor, so the event is retried on
the next run. Consumers must tolerate duplicate delivery.

Command pages:

- [`gog drive changes poll`](commands/gog-drive-changes-poll.md)
- [`gog docs comments poll`](commands/gog-docs-comments-poll.md)

## Drive push receiver

`gog drive changes serve` receives Drive push notifications, lists the actual
changes from the persisted page token, and optionally runs the same JSON-stdin
hook shape as polling:

```bash
gog drive changes serve \
  --listen 127.0.0.1:8443 \
  --state-file ~/.local/state/gog/drive-serve.json \
  --channel-token-file ~/.config/gog/drive-channel-token \
  --on-change './handle-drive-batch'
```

The default listener is loopback-only. Expose it through an HTTPS reverse proxy
or tunnel whose public route ends at `/drive-changes`. Google requires a public
HTTPS callback with a valid certificate. To terminate TLS in `gog` instead,
provide both `--cert` and `--key`.

The channel token is required and compared before notification headers are
parsed or any Drive API request or hook runs. Prefer `--channel-token-file` or
`GOG_DRIVE_CHANNEL_TOKEN` so a long-running secret is not exposed in the process
argument list. An explicit token file takes precedence over the ambient
environment variable. Use a random value; do not reuse OAuth credentials or
other sensitive data.

To let `gog` create and renew the channel after the listener is bound:

```bash
gog drive changes serve \
  --listen 127.0.0.1:8443 \
  --state-file ~/.local/state/gog/drive-serve.json \
  --channel-token-file ~/.config/gog/drive-channel-token \
  --auto-renew \
  --webhook-url https://example.com/drive-changes \
  --channel-ttl 24h \
  --renew-before 10m \
  --on-change './handle-drive-batch'
```

Auto-renew creates a unique replacement channel before expiration, persists the
new channel, then stops the previous channel. If cleanup fails, the state keeps
the previous channel metadata and retries before creating another replacement.
The maximum `--channel-ttl` is seven days, matching the Drive Changes API.

Without `--auto-renew`, register the channel separately with `drive changes
watch`. For a new receiver state file, pass the same initial page token to
`serve --token` that was used by `watch --token`.

Receiver behavior:

- only `POST` requests to the configured path are accepted
- mismatched or missing `X-Goog-Channel-Token` returns `401`
- malformed required `X-Goog-*` headers return `400`
- `sync` and duplicate notifications are acknowledged without running the hook
- every authenticated non-`sync` resource state is treated as a signal to read
  the changes feed; notifications are serialized so concurrent deliveries
  cannot race the page token
- queued and in-flight callbacks are bounded by `--notification-timeout`
  (default `5m`)
- request disconnects do not cancel an in-flight Drive read or hook; command
  shutdown still cancels them
- hooks remain sequential, but an in-flight hook does not block channel renewal
- Drive/API, hook, or state-write failure returns `500`; Google retries these
  statuses with backoff
- the page token and message number advance only after the hook succeeds
- `--filter-file` suppresses hooks for unrelated changes while still advancing
  state

The state file stores a SHA-256 digest of the channel token, not the token
itself. Auto-renewed receivers also bind notifications to the persisted current
or previous channel and resource IDs; manual receivers rely only on the channel
token, including when reusing state previously written by auto-renew mode.
`poll` and `serve` require separate state files; each state file records its
own command kind and rejects cross-use. Legacy state files without a kind remain
readable and gain one on the next write. Message-number deduplication is scoped
to both channel and resource ID. Channel IDs must be unique for every
registration as required by the Drive API; reusing an ID for the same resource
can inherit its prior sequence watermark and suppress notifications.

Command page:

- [`gog drive changes serve`](commands/gog-drive-changes-serve.md)
