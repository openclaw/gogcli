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
