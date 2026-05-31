# MCP server

`gog mcp` runs a typed Model Context Protocol server over stdio.

The MCP surface is deliberately narrower than the CLI. It registers typed tools
such as `gmail_search`, `docs_get`, and `sheets_read_range`; it does not expose a
generic command runner. Agents cannot send arbitrary `gog` argv through MCP.
Raw Google API dumps are also intentionally omitted from MCP tools when they
would bypass untrusted-content wrapping.

## Defaults

By default, only read-only tools are exposed:

```bash
gog --account you@example.com mcp
```

List the tools that would be registered:

```bash
gog --account you@example.com mcp --list-tools
```

Narrow the surface to specific services or tools:

```bash
gog --account you@example.com mcp --allow-tool gmail.*,docs_get,sheets_read_range
```

Expose write tools explicitly:

```bash
gog --account you@example.com mcp --allow-write --allow-tool docs_write,sheets_update_range
```

## Safety model

- No generic `gog_exec` or shell bridge.
- Read-only tools are the default.
- Write tools require `--allow-write`.
- `--allow-tool` can restrict by exact tool name, service name, or service
  wildcard, for example `gmail.*`.
- Parent root context is inherited: `--account`, `--home`, `--client`, JSON
  output, `--wrap-untrusted`, `--no-input`, and command allow/deny safety flags.
- Tool calls run as subprocesses with a timeout and bounded stdout/stderr.

## Initial tool set

Read tools:

- `gmail_search`
- `gmail_get_message`
- `gmail_get_thread`
- `drive_search`
- `drive_get`
- `docs_get`
- `sheets_read_range`
- `calendar_events`

Write tools, hidden unless `--allow-write`:

- `docs_write`
- `sheets_update_range`
