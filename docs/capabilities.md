---
title: Runtime capabilities
description: "Discover gog output, auth, safety, command, exit-code, and MCP contracts."
---

# Runtime capabilities

`gog` exposes automation contracts at the top level; there is no separate
agent mode or agent command namespace.

```bash
gog capabilities --json
gog exit-codes --json
gog schema --json
gog mcp --list-tools
```

## Capability snapshot

`gog capabilities` reports:

- supported machine output modes and automation flags
- supported authentication methods
- active `--dry-run`, `--no-input`, `--wrap-untrusted`, and
  `--gmail-no-send` state
- baked safety-profile state
- runtime command allowlists and denylists
- commands for schema, exit-code, and MCP tool discovery

The default snapshot is local, redacted, and keyring-free. It can indicate that
an account was selected, but does not return the identity, credential type,
scopes, services, or token expiry.

```bash
gog --account you@example.com capabilities --json
```

Credential metadata requires an explicit local CLI request:

```bash
gog --account you@example.com capabilities --include-auth --json
gog --account you@example.com capabilities \
  --include-auth \
  --include-account \
  --json
```

`--include-auth` may open the configured keyring. It returns credential type,
OAuth client name, stored services/scopes, and cached access-token expiry when
available. It never returns access tokens, refresh tokens, client secrets,
service-account key material, or keyring passwords.

`--include-account` is separate because account identity can itself be
sensitive in shared logs.

## MCP discovery

The read-only `gog_capabilities` MCP tool reports the active safety state and
the filtered tool set registered by that server process. It accepts no input
fields and never performs account or credential disclosure.

If `gog mcp` is started with `--allow-tool`, include `gog_capabilities` or the
`gog` service selector to expose it:

```bash
gog mcp --allow-tool gog_capabilities,docs_get
```

MCP write availability is based on the actual registered tool set after
`--allow-write` and `--allow-tool` filtering.
