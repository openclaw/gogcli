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

## Initial tools

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

Tool calls run as subprocesses of the same `gog` executable. The server adds a
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
