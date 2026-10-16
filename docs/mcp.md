---
title: MCP server
description: "Expose typed, allowlisted gog tools to MCP clients without a generic command runner."
---

# MCP server

`gog mcp` runs a Model Context Protocol server over stdio. It is for agent
clients that need Google Workspace tools but should not receive a generic shell
or arbitrary `gog` command bridge.

The server registers a small set of typed tools such as `gmail_search`,
`docs_get`, and `sheets_read_range`. Each tool has a fixed schema, maps to one
specific `gog` operation, and returns a structured result containing the tool
name, service, risk level, exit code, parsed stdout, and stderr.

## Quick start

Start a read-only server for one account:

```bash
gog --account you@example.com mcp
```

List the tools this server would expose and exit:

```bash
gog --account you@example.com mcp --list-tools
```

Limit the server to Gmail search and Docs reads:

```bash
gog --account you@example.com mcp \
  --allow-tool gmail_search,docs_get
```

Expose Docs read/write tools:

```bash
gog --account you@example.com mcp \
  --allow-write \
  --allow-tool 'docs.*'
```

`--allow-write` is always required for write tools. A write tool that matches
`--allow-tool` is still hidden until `--allow-write` is present.

The exception is an explicit persistent MCP policy. It can authorize a narrow
write surface without repeating `--allow-write` in every client definition;
runtime flags can only reduce that configured surface.

Gmail sending additionally requires `--allow-gmail-send`. Permanent message
and draft deletion additionally require `--allow-gmail-delete`; real deletion
also needs the operator's root `--force` flag. Existing `gmail`, `gmail.*`,
`write`, and wildcard grants never authorize either capability on their own.

## Why this is not `gog_exec`

MCP clients are often LLM-driven. A generic "run this command" tool would expose
every current and future CLI behavior through one broad capability, including
commands that were not reviewed for MCP use.

`gog mcp` uses a narrower contract:

- no generic command execution tool
- no model-supplied argv passthrough
- fixed tool schemas validated before command execution, including required
  fields, types, and rejection of unknown fields
- read-only tools by default
- write tools require explicit server startup flags
- existing `gog` account, auth, dry-run, no-input, and command safety flags are
  preserved

This keeps MCP useful for agents while making the permission surface visible at
server startup.

## Tool selection

By default, all read tools are registered and write tools are hidden.

Use `--allow-tool` to narrow the registered set. Values can be comma-separated
or repeated. An explicitly empty, whitespace-only, or comma-only list is rejected;
omit the flag to use the defaults or configured policy:

```bash
gog mcp --allow-tool gmail_search --allow-tool docs_get
gog mcp --allow-tool gmail_search,docs_get
```

Accepted selectors:

| Selector | Meaning |
| --- | --- |
| `gmail_search` | One exact tool |
| `gmail` | All Gmail tools allowed by risk mode |
| `gmail.*` | All Gmail tools allowed by risk mode |
| `read` | All read tools |
| `write` | All write tools, only when `--allow-write` is also set |
| `*` or `all` | All tools allowed by risk mode |

Examples:

```bash
# Read-only Gmail tools.
gog mcp --allow-tool gmail

# Only Docs tools, including writes.
gog mcp --allow-write --allow-tool 'docs.*'

# Read-only server, but only Calendar and Sheets reads.
gog mcp --allow-tool calendar,sheets

# Ordinary write tools. Read tools and gated Gmail send/delete are not included.
gog mcp --allow-write --allow-tool write

# Compose drafts and organize the mailbox, with sending blocked.
gog --gmail-no-send mcp --allow-write --allow-tool gmail

# Send only through the two explicitly selected typed tools.
gog mcp --allow-write --allow-gmail-send \
  --allow-tool gmail_send_message,gmail_send_draft
```

## Persistent capability policy

For several MCP clients or accounts, put the maximum registered tool surface in
`config.json` instead of duplicating capability arguments. Without an `mcp`
block, behavior is unchanged: all read tools are available, writes require
`--allow-write`, and `--allow-tool` filters the result.

```json5
{
  "mcp": {
    "allow_tools": ["read"],
    "allow_write": false,
    "accounts": {
      "personal@example.com": {
        "allow_tools": ["read", "docs.*", "calendar.*"],
        "allow_write": true
      },
      "work@example.com": {
        "allow_tools": ["read"],
        "allow_write": false
      }
    }
  }
}
```

An account entry is a complete replacement for the global policy, not a partial
merge. Account keys are matched case-insensitively after aliases and automatic
account selection are resolved, then that resolved account is pinned for every
MCP child command. Per-account policies require stored account credentials;
direct access tokens and ADC can use only the global policy because an account
label does not prove the authenticated principal. An omitted `allow_tools` value defaults to
`["read"]`; an explicitly empty list is rejected. `allow_write: true` requires
an explicit tool list so a typo cannot accidentally expose every write tool.

The optional policy keys `allow_gmail_send` and `allow_gmail_delete` default to
false and each requires `allow_write: true`. They apply to the selected account
policy, or to the global policy when no account override matches. For example:

```json5
{
  "mcp": {
    "allow_tools": ["read"],
    "accounts": {
      "assistant@example.com": {
        "allow_tools": ["gmail_create_draft", "gmail_send_draft"],
        "allow_write": true,
        "allow_gmail_send": true
      }
    }
  }
}
```

This account can create and send drafts, but cannot delete drafts or messages.
An account override does not inherit global send/delete permissions. Runtime
`--allow-gmail-send` and `--allow-gmail-delete` cannot widen a configured policy.
An exact send/delete tool selector still needs its corresponding capability.

On upgrade, existing saved policies with `allow_write: true` and `gmail`,
`gmail.*`, `write`, `*`, or `all` intentionally gain the ordinary Gmail draft
and mailbox tools under the existing read/write selector contract. Sending and
permanent deletion remain unavailable without their separate grants. To retain
a narrower reviewed set across upgrades, pin exact tool names in `allow_tools`;
for the former Gmail read surface, use `gmail_search`, `gmail_get_message`, and
`gmail_get_thread` alongside any explicitly authorized tools from other services.

The configured policy is a ceiling. `--allow-tool` can intersect it with a
smaller runtime set, `--readonly` removes all writes, and `--allow-write` cannot
widen a read-only policy. Baked safety profiles remain the outer immutable
ceiling. Unknown configured selectors and attempted write widening fail before
the MCP server starts. Use `gog mcp --list-tools` with the same account and flags
to inspect the final registered surface.

## Typed tools

Read tools:

| Tool | Purpose |
| --- | --- |
| `gmail_search` | Search Gmail messages with Gmail query syntax. |
| `gmail_get_message` | Read one Gmail message by ID. Sanitized content is on by default. |
| `gmail_get_thread` | Read one Gmail thread by ID. Sanitized content is on by default. |
| `gmail_list_drafts` | List draft, message, and thread IDs with bounded pagination. |
| `gmail_get_draft` | Read one draft's MIME payload without downloading attachments. |
| `gmail_list_labels` | List label names and case-sensitive IDs. |
| `drive_search` | Search Drive files by text or Drive query language. |
| `drive_get` | Read Drive file metadata by ID. |
| `docs_get` | Read a Google Doc as wrapped text, optionally one tab or all tabs. |
| `sheets_read_range` | Read values from a Sheets range. |
| `calendar_events` | List Calendar events. |

Write tools, hidden unless `--allow-write`:

| Tool | Purpose |
| --- | --- |
| `docs_write` | Append or replace Google Docs text, optionally as Markdown. |
| `sheets_update_range` | Update values in a Sheets range from a literal JSON 2D array. |
| `gmail_create_draft` | Compose a draft from literal plain text and/or HTML. |
| `gmail_update_draft` | Replace a draft's subject and body. |
| `gmail_create_label` | Create a label. |
| `gmail_modify_messages` | Add/remove labels on 1–1000 explicit message IDs. |
| `gmail_modify_thread` | Add/remove labels on every message in an explicit thread. |
| `gmail_mark_read`, `gmail_mark_unread` | Change read state for explicit messages. |
| `gmail_archive_messages` | Remove `INBOX` from explicit messages. |
| `gmail_trash_messages` | Add `TRASH` and remove `INBOX` from explicit messages. |
| `gmail_restore_messages` | Remove `TRASH`; this does not add `INBOX`. |

Additional gated write tools:

| Tool | Additional authorization |
| --- | --- |
| `gmail_send_message`, `gmail_send_draft` | `--allow-gmail-send` or configured `allow_gmail_send`. |
| `gmail_delete_draft`, `gmail_delete_messages` | `--allow-gmail-delete` or configured `allow_gmail_delete`, plus root `--force` for real deletion. |

All message-list mutations take explicit IDs, with at most 1000 per call;
they do not accept a search query. Search first, review the IDs, then mutate.
Label arrays accept literal names or case-sensitive IDs. Each entry is one label,
including any commas or backslashes, and each add/remove array is limited to 100
entries. The corresponding `gmail batch modify` and `gmail thread modify` CLI
commands accept repeatable `--add-label` and `--remove-label` literal flags;
their existing `--add` and `--remove` comma-separated syntax is unchanged.

Compose tools accept literal `body` and/or `body_html`, recipients, subject,
verified send-as aliases, and reply context. They cannot read attachment paths,
raw message files, body files, or signature files. Draft creation allows omitted
recipients. Sending requires recipients or a reply-all target.

`gmail_update_draft` replaces the subject and body, rather than patching them.
Omit `to` to keep existing To recipients, or pass `"to": ""` to clear them.
Omitted Cc/Bcc are cleared. Existing attachments and reply headers are preserved
unless `clear_attachments` or `clear_reply_context` is true. Supply `body_html`
when retaining HTML: a plain-only update replaces the draft's HTML body and
reports the existing CLI warning.

Draft deletion is permanent, with no Trash or restore path. Message deletion
also requires the broader `https://mail.google.com/` OAuth scope, which is not
part of the default Gmail grant. Tool authorization never changes OAuth scopes.
For example, this explicitly authorized server exposes only draft deletion:

```bash
gog --account you@example.com --force mcp \
  --allow-write --allow-gmail-delete --allow-tool gmail_delete_draft
```

There is no model-supplied `force` or confirmation argument. Without root
`--force`, destructive calls fail non-interactively; `--dry-run` still previews
them. Send tools retain all global, account-specific, and runtime no-send
restrictions. `--readonly` hides all writes, including send and delete.

The generated command reference for the server itself is
[`gog mcp`](commands/gog-mcp.md).

MCP clients discover the registered surface through the protocol's standard
`tools/list` request. For shell-side inspection before starting the server, use
`gog mcp --list-tools`; no model-callable discovery tool is added.

## Exact exports and compact Gmail reads

| Tool | Contract |
| --- | --- |
| `gmail_get_raw` | Exact decoded RFC822 bytes for `message_id`, in base64 chunks. |
| `gmail_get_attachment` | Exact attachment bytes for `message_id` and `attachment_id`. |
| `gmail_thread_message_ids` | Ordered metadata-only message IDs, up to 128 whole rows per page. |
| `gmail_search_threads` | One compact Gmail search page, default 20 and maximum 100 threads. |

Export arguments are `offset` (default 0), `length` (default 32768, maximum
262144), and optional `snapshot_id`. The first response includes object IDs,
`snapshot_id`, decoded `size`, SHA-256, `expires_at`, actual `offset` and `length`,
`complete`, and standard `data_base64`. Raw exports include the provider's
thread ID when available. This is decoded RFC822 data, not a Unicode conversion
or reserialization of Gmail's full payload.

Pin the snapshot and all integrity metadata. Repeat the same object IDs and
`snapshot_id` on every later request, advancing by the returned length, which
may be smaller than requested to fit the output budget. Nonzero offsets require
a snapshot. An offset exactly at EOF returns an empty completed chunk. Handles
expire 900 seconds after publication and are invalid after restart. A failed or
expired transfer requires an explicit fresh download; never append a different
snapshot to an existing partial file. Verify final byte count and SHA-256 before
publishing a destination.

The transport-neutral Python example at
[`scripts/examples/mcp-gmail-export.py`](https://github.com/openclaw/gogcli/blob/main/scripts/examples/mcp-gmail-export.py)
accepts an authenticated `call_tool(name, args)` callable, checks every native
MCP envelope and chunk, and atomically replaces the destination only after
integrity verification. It creates a private sibling temporary file, cleans it
on errors or cancellation, and works with structured content or the native text
envelope. Transport, account routing and authentication belong to the caller.

`gmail_thread_message_ids` takes `thread_id`, `max`, and `cursor`. Its signed
`next_cursor` pins the same immutable metadata snapshot; all message IDs remain
whole and ordered. Follow pages until `complete`. Headers and snippets are
explicitly wrapped as untrusted content, with `truncated_fields` for text limits.
It fetches no message bodies or attachments. At most 10000 messages and 8 MiB of
metadata are retained per snapshot.

`gmail_search_threads` takes literal `query`, `max`, and opaque provider `page`.
Follow `nextPageToken`; `count` counts this page and `complete` means no next
page. Search pages are not a mailbox snapshot. Detail reads use at most two
workers; text and labels carry explicit truncation metadata. An oversized page
is refetched with fewer rows instead of clipping identifiers or dropping its
continuation token. Neither search tool reads local paths or accepts `--all`.

## Calendar and Gmail settings tools

| Tool | Purpose / additional gate |
| --- | --- |
| `calendar_list_calendars` | One provider page, with `max` and `page`. |
| `calendar_get_event` | Read `calendar_id` (default `primary`) and `event_id`. |
| `calendar_search_events` | One query page with date window, `max`, and `page`; native -30/+90 day default. |
| `calendar_freebusy` | Busy intervals and per-calendar errors for at most 50 literal calendar IDs. |
| `gmail_list_filters` | List native Gmail settings filters. |
| `calendar_create_event`, `calendar_update_event` | Typed native event creation and PATCH updates; ordinary write grant. |
| `calendar_move_event` | Move to a literal destination calendar; ordinary write grant. |
| `calendar_respond` | RSVP; always requires `calendar_notify`. |
| `gmail_rename_label`, `gmail_create_filter` | Rename labels or create literal criteria/actions; ordinary write grant. |
| `calendar_cancel_event` | Requires `calendar_delete` and operator startup `--force`. |
| `gmail_delete_label`, `gmail_delete_filter` | Require `gmail_settings_delete` and operator startup `--force`. |

The three additional capability flags are `--allow-calendar-notify`,
`--allow-calendar-delete` and `--allow-gmail-settings-delete`. Matching persistent
policy keys use underscores, for example `allow_calendar_notify`. They default
to false, require write authorization, and never follow from `calendar.*`,
`gmail.*`, `write`, or `*` alone. Account overrides replace these grants too;
runtime flags cannot widen a configured policy. `--readonly` hides every write.
No tool accepts model-supplied `force`.

Calendar writes default `send_updates` to `none`. `all` and `externalOnly`
require the notification grant in addition to the write/delete grant. RSVP can
notify the organizer even with `none`, so it always requires notification
permission. `--gmail-no-send` governs Gmail and does not block Calendar
invitations or reminders. Reminder emails to the acting user are possible;
select exact tools and review event reminder settings where that matters.

Event schemas include literal attendee, recurrence, reminder and attachment-URL
arrays, endpoint timezones, source URL/title, guest permissions, visibility,
color, transparency, Meet controls and recurrence scope. They never accept local
files. Commas remain inside array entries. For updates, omission preserves a
field, while supported empty strings/arrays explicitly clear it and false guest
permissions remain false. A shared update `timezone` requires both `start` and
`end`; `single`/`future` scope requires RFC3339 `original_start`, or a
`YYYY-MM-DD` instance date for all-day events. Filter creation
accepts literal criteria and label arrays, without forwarding or file input.

New ordinary tools are included by existing broad write selectors on upgrade.
Pin exact tool names to preserve a previously reviewed surface. Existing Gmail
send/delete gates and native draft semantics remain unchanged.

### Bounded mutation receipts

The new Calendar and Gmail settings writes return a bounded receipt. Successful
writes have `outcome`, `known_steps`, `attempted_steps`, bounded confirmed `ids`,
`metadata_omitted`, and `retry_safe: false`. Outcomes are `committed`, `partial`,
`failed`, `outcome_unknown`, or `not_attempted` for a dry run or successful no-op.
A failure has `stdout.error.code` and `stdout.receipt`, with MCP `isError` and a
nonzero native `exit_code`. Provider error bodies and tokens are not included.

A multi-step operation can partially commit. Missing or malformed child output,
timeout, response overflow or uncertain transport failure returns
`outcome_unknown`; it never replays the write to reconstruct output. Metadata
can be omitted to preserve a complete receipt within the budget. These new
mutations disable automatic write retries. Inspect confirmed IDs and provider
state before deciding any subsequent action. Existing tools retain their
established output contracts.

### Snapshot and output limits

New tools require `--max-output-bytes` of at least 4096 and cap the complete
serialized MCP result at the smaller of that setting and 1 MiB, including text
and structured content. Below the minimum, these tools are omitted from the
catalog. `--results-only` and `--select` are rejected for them before provider
access, including cached ranges. They return complete JSON or a bounded error.
Legacy tools keep their existing capture behavior.

Decoded exports are limited to 50 MiB each; provider bodies are bounded before
JSON/base64 allocation (72 MiB for exports, 12 MiB for metadata/action reads).
Private snapshot storage allows 256 MiB including pending reservations,
128 entries and two concurrent fetch jobs. Unexpired handles are retained;
capacity exhaustion fails without evicting them. Eight new tool calls can be
active at once, with immediate `resource_exhausted` errors beyond that bound. Storage is
removed at shutdown; UNIX files are mode 0600 in mode 0700 directories, and
Windows files/directories use protected owner-user DACLs.

Account/client selection and the capability policy are resolved at startup;
handles are private to that process/account partition. Command deny/exact
allowlists, readonly and output rules are checked on every call, including
snapshot hits. OAuth permissions remain independent of tool grants.

### Migrating feature sidecars

Choose one owner for each tool name. Suppress sidecar copies of native
`gmail_list_labels` and `gmail_list_drafts`, and update callers to native schemas
rather than advertising duplicates. Native draft recipient strings and clear
semantics differ from some sidecar arrays. Use `gmail_trash_messages` for native
Trash operations. Native permanent draft deletion requires its existing Gmail
delete grant and startup force; ordinary write permission is insufficient.

Adapt clients to the native structured/text envelope and chunk integrity
contract before removing export hooks. Keep deployment-specific account routing,
HTTP proxies, keyring packaging and update policies until they have their own
verified replacement. Validate catalogs, schemas, policy negatives, paging,
expiry, concurrent transfers and independent hashes in fixtures. Deployment
rollout is separate from tool availability; keep the previous image and client
configuration available for rollback without recreating credentials.

## Client configuration

MCP clients usually need a command and an argument list. Put account selection
and safety policy on the server command, not inside tool calls.

Minimal stdio configuration:

```json
{
  "command": "gog",
  "args": ["--account", "you@example.com", "mcp"]
}
```

Read-only Docs and Sheets configuration:

```json
{
  "command": "gog",
  "args": [
    "--account", "you@example.com",
    "--enable-commands-exact", "mcp,docs.cat,sheets.get",
    "mcp",
    "--allow-tool", "docs_get,sheets_read_range"
  ]
}
```

Docs read/write configuration:

```json
{
  "command": "gog",
  "args": [
    "--account", "you@example.com",
    "--enable-commands-exact", "mcp,docs.cat,docs.write",
    "--no-input",
    "mcp",
    "--allow-write",
    "--allow-tool", "docs.*"
  ]
}
```

For headless services, set `GOG_KEYRING_BACKEND=file` and
`GOG_KEYRING_PASSWORD` on the MCP client process or service unit. A successful
interactive shell check does not prove the MCP client inherited those
variables; verify through the same process manager that launches the server.

## mcporter examples

List registered tools and their schemas:

```bash
mcporter list \
  --stdio gog \
  --stdio-arg --account \
  --stdio-arg you@example.com \
  --stdio-arg mcp \
  --stdio-arg --allow-tool \
  --stdio-arg 'docs.*' \
  --schema \
  --json
```

Dry-run a Docs write through MCP:

```bash
mcporter call \
  --stdio gog \
  --stdio-arg --account \
  --stdio-arg you@example.com \
  --stdio-arg --dry-run \
  --stdio-arg mcp \
  --stdio-arg --allow-write \
  --stdio-arg --allow-tool \
  --stdio-arg docs_write \
  docs_write \
  '{"document_id":"DOCUMENT_ID","text":"MCP smoke test\n","append":true}'
```

Read a Sheet range:

```bash
mcporter call \
  --stdio gog \
  --stdio-arg --account \
  --stdio-arg you@example.com \
  --stdio-arg mcp \
  --stdio-arg --allow-tool \
  --stdio-arg sheets_read_range \
  sheets_read_range \
  '{"spreadsheet_id":"SPREADSHEET_ID","range":"Sheet1!A1:C10"}'
```

Update a Sheet range:

```bash
mcporter call \
  --stdio gog \
  --stdio-arg --account \
  --stdio-arg you@example.com \
  --stdio-arg mcp \
  --stdio-arg --allow-write \
  --stdio-arg --allow-tool \
  --stdio-arg sheets_update_range \
  sheets_update_range \
  '{"spreadsheet_id":"SPREADSHEET_ID","range":"Sheet1!A1:B1","values_json":"[[\"status\",\"ok\"]]","input":"RAW"}'
```

`sheets_update_range.values_json` must be literal JSON. MCP rejects `@file`,
`@-`, and `-` expansion forms so a model cannot cause the server process to
read arbitrary local files or stdin.

## Safety model

Most tool calls run as subprocesses of the same `gog` executable. Exact exports
are read from private snapshots; thread-ID enumeration uses the native Gmail
client with the same pinned account and safety context. The server adds a
non-interactive, agent-oriented root context to every child command:

- `--json`
- `--wrap-untrusted`
- `--no-input`
- `--color=never`

The server also preserves selected parent root flags:

- `--account`
- `--client`
- `--home`
- `--dry-run`
- `--force` for explicitly authorized permanent-deletion tools only
- `--results-only`
- `--select`
- direct access tokens

And it preserves command safety flags:

- `--gmail-no-send`
- `--enable-commands`
- `--enable-commands-exact`
- `--disable-commands`

Use both MCP tool allowlists and command allowlists when the server is exposed
to an untrusted or semi-trusted agent:

```bash
gog --account you@example.com \
  --enable-commands-exact mcp,docs.cat,docs.write \
  --disable-commands gmail.send,gmail.drafts.send \
  --gmail-no-send \
  mcp \
  --allow-write \
  --allow-tool 'docs.*'
```

If a tool maps to a disabled command, the tool call returns a non-zero exit code
and the child command error in `stderr`.

## Output shape

Successful calls return structured MCP content shaped like:

```json
{
  "tool": "docs_get",
  "service": "docs",
  "risk": "read",
  "exit_code": 0,
  "stdout": {
    "documentId": "..."
  },
  "stderr": ""
}
```

If a child command prints valid JSON, `stdout` is parsed as JSON with numeric
literals preserved. Otherwise `stdout` is returned as a string. Empty stdout is
omitted.

If the child command exits non-zero, the MCP result is marked as an error and
includes the same structured fields with `exit_code` and `stderr`.

Send and permanent-deletion tool listings and results also include
`"capability": "gmail_send"` or `"capability": "gmail_delete"`. Their risk remains
`write`, so existing clients can keep using the read/write classification.

## Limits and timeouts

Each tool call has a subprocess timeout and bounded stdout/stderr capture:

```bash
gog mcp --timeout-seconds 30 --max-output-bytes 262144
```

Defaults:

- timeout: 60 seconds
- max captured stdout/stderr: 102400 bytes each

Use command-specific limits too. For example, `docs_get` has a `max_bytes`
argument, and search tools have `max` arguments.

## Authentication

The MCP server uses normal `gog` auth. Before wiring a client, verify the same
account and scopes from a shell:

```bash
gog --account you@example.com auth doctor --check
gog --account you@example.com mcp --list-tools
```

Then verify through the MCP client entrypoint. In services and desktop MCP
clients, most auth failures are environment inheritance problems: missing
`GOG_ACCOUNT`, missing file-keyring password, different `GOG_HOME`, or a
different OAuth client selected by `--client`.

## Troubleshooting

`no MCP tools enabled`

: Your `--allow-tool` filters excluded everything, or you selected only write
  tools without `--allow-write`.

`command "..." is disabled`

: The MCP tool was registered, but the child `gog` command was blocked by
  `--enable-commands`, `--enable-commands-exact`, `--disable-commands`, or a
  baked safety profile.

Tool missing in the client

: Run `gog mcp --list-tools` with the same flags. If the tool is not listed,
  fix `--allow-tool` or add `--allow-write` for write tools. If it is listed,
  refresh or restart the MCP client.

Auth works in Terminal but not in the MCP client

: Compare `--account`, `--client`, `--home`, `GOG_HOME`,
  `GOG_KEYRING_BACKEND`, and `GOG_KEYRING_PASSWORD` in the process that starts
  the MCP server.

Large output is truncated

: Increase `--max-output-bytes`, narrow the request, or use tool arguments such
  as `max`, `max_bytes`, date ranges, or Drive field masks.
