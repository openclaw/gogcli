# External Plugin Discovery - Phase 1 MVP Design

Issue: https://github.com/steipete/gogcli/issues/188

## Overview

Implement cargo/git-style external command discovery allowing `gog foo bar` to execute `gog-foo-bar` binary from PATH.

## Design Decisions

### Decision 1: Post-parse fallback (Option B) vs Pre-parse interception (Option A)

**Chosen: Post-parse fallback (Option B)**

| Aspect | Option A (Pre-parse) | Option B (Post-parse) |
|--------|---------------------|----------------------|
| Performance | Slower on normal commands (PATH check first) | Faster for built-in commands |
| Simplicity | Cleaner - no error parsing needed | Need to detect Kong error types |
| Plugin priority | Plugins can shadow built-ins | Built-ins always take precedence |
| Safety | Less safe - accidental shadowing | Safer |

**Why Option B:**

1. **Safety**: Built-in commands always take precedence - an external binary cannot accidentally or maliciously shadow core functionality
2. **Convention**: Follows git/cargo pattern where built-ins win
3. **Performance**: No PATH scanning for normal command usage (majority of invocations)

### Decision 2: Longest-first (greedy) matching

**Chosen: Longest-first**

When user types `gog docs headings list`, search order:
1. `gog-docs-headings-list` (most specific)
2. `gog-docs-headings`
3. `gog-docs` (least specific)

**Why longest-first:**

1. **Specificity wins**: More specific plugin takes precedence over generic one
2. **Convention**: Matches cargo/git behavior
3. **Composability**: `gog-docs-headings` handles headings, while `gog-docs` could handle generic docs operations - no conflict

### Decision 3: Binary prefix `gog-*`

**Chosen: `gog-` prefix**

Example: `gog docs headings` → `gog-docs-headings`

**Why `gog-` not `gogcli-`:**

1. **Consistency**: Matches the CLI binary name users invoke
2. **Brevity**: Shorter for plugin developers
3. **Convention**: git uses `git-*`, cargo uses `cargo-*` (matches binary name)

## Phase 1 MVP Scope

**Included:**

* PATH discovery + exec
* Longest-first matching
* Pass remaining args to plugin
* Unit tests documenting behavior

**Excluded (Phase 2+):**

* `--help-oneliner` protocol
* Help integration (plugins in `gog --help`)
* Environment variable passing (GOG_AUTH_TOKEN_PATH, etc.)
* Discovery caching

## Implementation

### Files

| File | Purpose |
|------|---------|
| `internal/cmd/external.go` | Discovery + exec logic |
| `internal/cmd/external_test.go` | Unit tests |
| `internal/cmd/root.go` | Integrate into Execute() |

### Core Algorithm

```go
func tryExternalCommand(args []string) error {
    // Longest-first: try most specific binary first
    // Why: More specific plugins should take precedence (cargo/git pattern)
    for i := len(args); i > 0; i-- {
        binaryName := "gog-" + strings.Join(args[:i], "-")
        if path, err := exec.LookPath(binaryName); err == nil {
            return execExternal(path, args[i:])
        }
    }
    return nil // not found
}
```

### Integration Point

In `root.go` `Execute()`, after `parser.Parse(args)` fails:

```go
kctx, err := parser.Parse(args)
if err != nil {
    // Try external command before returning parse error
    // Why post-parse: Built-in commands always take precedence (safer)
    if extErr := tryExternalCommand(args); extErr != nil {
        return extErr
    }
    // Fall through to original error if no external command found
    ...
}
```

## Commits

1. `feat(plugin): add external command discovery and execution`
2. `test(plugin): add unit tests for external command discovery`
3. `feat(plugin): integrate external commands into Execute()`

## Future Phases

**Phase 2 (separate PRs):**

* `--help-oneliner` protocol for help integration
* Environment variable passing to plugins
* Plugin listing in `gog --help`

**Phase 3:**

* Discovery caching for performance
* Version compatibility checks
