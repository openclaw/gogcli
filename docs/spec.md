# Architecture and contracts

`gog` is a Go CLI for Google Workspace and related APIs. The command tree is
built with Kong in `internal/cmd`; `cmd/gog` is the process entrypoint. The Go
minimum and preferred toolchain are declared in `go.mod`.

The running command tree is the source of truth for command names, arguments,
flags, and aliases:

```bash
gog schema --json
gog schema gmail search --json
gog help docs write
```

Generated [command references](commands/README.md) and
[agent skills](agent-skills.md) come from that schema. Update command structs
first, then regenerate with `make docs-commands` and `make agent-skills`.

## Command boundaries

Commands validate local inputs before creating API clients or mutating state.
Shared service helpers select the account and construct authenticated clients;
request planners in packages such as `docsedit`, `docssed`, and `sheetsdimension`
keep API request construction separate from command orchestration.

Docs editing commands are grouped by operation in `internal/cmd/docs_write.go`,
`docs_write_markdown.go`, `docs_update.go`, `docs_insert.go`, `docs_delete.go`,
and `docs_find_replace.go`. `docs_edit.go` retains the `docs edit` command
wrapper and shared deprecated-tab flag resolution. See
[Docs editing](docs-editing.md) for the user-facing behavior and
[persisted batches](docs-batch.md) for revision and commit semantics.

Whole-document Docs, Sheets, and Slides exports share
`internal/cmd/export_via_drive.go`. The experimental single-tab Docs export
uses the Docs web endpoint instead; see [export conventions](https://github.com/openclaw/gogcli/blob/main/docs/refactor/exports.md).

The generic Discovery API surface has its own request validation, host guards,
and bounded document cache in `internal/discoveryapi`. See [Raw API](raw-api.md)
and [Automation](automation.md#discovery-document-cache).

## Authentication and storage

`internal/googleauth` owns OAuth flows, service scopes, and account selection;
`internal/googleapi` constructs service clients and owns the shared retry
transport. Browser, manual, remote, and account-manager authorization use S256
PKCE. Stored tokens track Google's OIDC subject when available and preserve
legacy email-keyed lookup compatibility.

`internal/config` resolves configuration, data, state, and cache paths.
`internal/secrets` owns platform keyring and encrypted file storage. Do not
introduce a separate plaintext token store or bypass the existing keyring
locking and timeout behavior.

The detailed contracts live in:

- [OAuth clients](auth-clients.md): account/client routing, scopes, service accounts.
- [Paths and state](paths.md): overrides, XDG layout, and legacy reads.
- [Quickstart](quickstart.md): OAuth setup and authorization.
- The generated [OAuth service table](https://github.com/openclaw/gogcli#supported-oauth-services).

## Output and safety

`internal/outfmt` and the command output helpers keep JSON and TSV on stdout;
`internal/ui` sends progress and diagnostics to stderr. Use the invocation's
context writer so embedded commands and tests preserve output routing.

[Automation](automation.md) defines JSON projection, output precedence, exit
codes, retries, dry runs, and read-only behavior. [Safety profiles](safety-profiles.md)
define baked command policy and locked flags. Existing CLI flags, aliases,
configuration keys, and storage formats are compatibility contracts.

The [MCP server](mcp.md) exposes a typed stdio tool surface with read-only
defaults and explicit write authorization. It does not expose arbitrary shell
or argv execution.

## Development gates

`Makefile` owns development-tool pins and local gates; `.golangci.yml` configures
linting against the minimum Go version. `make fmt` applies goimports and
gofumpt; `make fmt-check` reports formatting differences without editing files.

Run `make ci` for formatting, lint, deadcode, Go and script tests, Docker
version consistency, documentation coverage, and generated agent skills. The
tracking worker has a separate pnpm workspace and `make worker-ci` gate.
GitHub workflows under `.github/workflows` define the platform matrix.

Tests live beside the implementation. Google API behavior is normally covered
with `httptest`; [live testing](live-testing.md) describes the opt-in test-account
suite and resource cleanup. [Releasing](RELEASING.md) documents publication.
