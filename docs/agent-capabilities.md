---
title: Agent capabilities
description: "Inspect gog authentication disclosure, runtime safety rules, and MCP tool exposure before calling Google APIs."
---

# Agent capabilities

`gog agent capabilities` emits a local runtime snapshot for agents, CI jobs,
and MCP clients. The default command is offline: it does not open the keyring,
inspect stored credentials, or print an account identity.

```bash
gog agent capabilities --json
```

The response includes:

- supported authentication methods
- active dry-run and Gmail no-send state
- runtime command allow and deny rules
- baked safety-profile name and status
- the command for retrieving the full visible CLI schema

The snapshot reports configured command rules rather than claiming a single
global read/write state. Different commands can have different safety and OAuth
requirements.

## Credential metadata

Credential inspection is explicit because it can access the configured keyring:

```bash
gog --account you@example.com agent capabilities --include-auth --json
```

`--include-auth` may add the active authentication method, OAuth client,
granted services and scopes, and cached access-token expiry. It never returns
access tokens, refresh tokens, client secrets, service-account keys, or
credential paths.

Account identity remains omitted unless separately requested:

```bash
gog --account you@example.com agent capabilities \
  --include-auth \
  --include-account \
  --json
```

Use `--include-account` only when the output destination is allowed to receive
that identity.

## MCP discovery

`gog mcp` registers a read-only `gog_capabilities` tool. Its response includes
the tools actually enabled by the server's `--allow-tool` and `--allow-write`
flags.

The MCP tool accepts two optional booleans:

- `include_auth`: inspect selected credential metadata; may access the keyring
- `include_account`: include the selected account identity

Both default to false. The tool runs in the MCP server process so it can report
the actual filtered tool set without invoking a generic command bridge.

This capability document is gog-specific. It does not claim conformance with
the `auth.md` agent-registration protocol, and it does not add HTTP or
well-known endpoints.
