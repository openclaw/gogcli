# Architecture — gogcli

Operational architecture of this repo, written for an LLM doing work inside it. Distilled from the ADR that drove the generator (see `setup-downstream-fork` skill, ADR 0001).

## Shape

This repo is a `git clone` of [steipete/gogcli](https://github.com/steipete/gogcli.git). Upstream's files stay at the repo root, at the paths upstream uses. Fork-authored scaffolding lives under a single namespace, `.fork/`. Upstream's full git history is present in the object store; you can `git log upstream` for unsquashed upstream commits.

The only root-level files the fork owns:

- `AGENTS.md` — short pointer to `.fork/AGENTS.md`.
- `CLAUDE.md` — symlink to `AGENTS.md`.
- `.mergify.yml` — Mergify requires it at root.
- `.claude/skills` and `.agents/skills` — symlinks to `.fork/skills/` so agent harnesses auto-discover repo-local skills.
- `.github/workflows/fork-*.yml` — three workflows. The `fork-` prefix avoids colliding with upstream's own workflow names.

Everything else fork-authored is under `.fork/`.

## Two branches, many tags

- **`main`** — default branch. Upstream code plus fork patches applied as commits. Never force-pushed. Only ever moves by squash-merge from a `sync/*` PR.
- **`upstream`** — pristine mirror of `steipete/gogcli@main`. Force-updated every sync. Fork commits never land here.

Tags:

| Tag | What | Lifecycle |
|---|---|---|
| `upstream-sync/<date>-<sha>` | every upstream SHA ever imported | permanent; durable rollback anchor |
| `sync/<date>-<sha>-merged` | patch-level SHAs preserved after squash-merge | permanent |
| `release/<upstream-tag>` | `main` snapshot at each upstream version | permanent |

The `upstream-sync/*` tags are the reason upstream force-pushes can't destroy our history — every imported SHA stays reachable.

## Patch model

Fork state is conceptually `upstream_at_SHA + [patch_0, patch_1, ...]`. Patches are git commits carrying two trailers:

```
Fork-Patch: <slug>
Reason: <why this patch exists>
```

The `Fork-Patch:` trailer is what the sync pipeline greps for when regenerating `.fork/patches/`. The `Reason:` trailer is what the LLM resolver reads when it needs to decide how to merge a drifted patch.

Two equivalent views of the patch set:

```
# commit-level
git log upstream..main --grep='Fork-Patch:' --oneline

# flat-file
ls .fork/patches/
```

Both always agree. `.fork/patches/` is regenerated on every sync via `git format-patch upstream..main --grep='Fork-Patch:'`. Do not edit files in `.fork/patches/` directly — edit the git commit and re-export.

## The sync mechanism

Sync runs on a short-lived `sync/<date>-<sha>` branch, never on `main` directly. That's how `main` stays append-only.

1. `fork-upstream-sync.yml` fires on cron (`0 6 * * *`) or `workflow_dispatch`.
2. Fetch upstream. If HEAD equals `.fork/revision.txt`'s `upstream`, exit clean.
3. Tag `upstream-sync/<date>-<sha>` at upstream HEAD. Durable.
4. Reset the `upstream` branch to upstream HEAD (force-update; this branch is a mirror).
5. Create `sync/<date>-<sha>` from `main`. Rebase onto the new `upstream`.
6. On conflict, `.fork/tools/llm_resolve.py` handles each conflicted file. See `resolver-prompt.md` for what it loads and `patch-vocabulary.md` for the state vocabulary it uses.
7. If the resolver bails, it writes an inline `DESIGN_CONFLICT: <reason>` marker.
8. Regenerate `.fork/patches/`. Update `.fork/revision.txt`. Write `.fork/snapshots/<date>-<sha>.json`.
9. Open PR against `main`. If any `DESIGN_CONFLICT:` markers exist, copy them into the PR body under `### DESIGN_CONFLICT:`, label `needs-human`, file a tracking issue. Otherwise label `automerge`.
10. `fork-build-release.yml` runs `go build ./...` and `go test ./...`, plus a `drift-recheck` that re-fetches upstream and fails if HEAD advanced past the snapshot SHA.
11. Mergify squash-merges when `build`, `smoke-test`, and `drift-recheck` are green and the PR body carries no `DESIGN_CONFLICT:` line.
12. After merge, tag `sync/<date>-<sha>-merged`. If upstream cut a tag, tag `release/<upstream-tag>`.

## Conflict resolution

Three outcomes per conflicted file:

1. **Resolved.** Resolver produced a patched file that compiles and passes the smoke test. Commit continues, rebase continues.
2. **`DESIGN_CONFLICT: <reason>`.** Resolver couldn't decide. Marker stays in the file. The sync PR is labeled `needs-human`, not `automerge`. A human edits, removes the marker, pushes.
3. **Cache hit.** If `(file_path, pre_context_hash, post_context_hash)` matches something in `.fork/.llm-cache/`, the cached resolution is replayed without hitting the provider.

The resolver's prompt template, input schema, and output rules are in `resolver-prompt.md`. The structured vocabulary it uses (PATCH_CHANGED, SRC_CHANGED, etc., borrowed from Brave's `gitPatcher.js`) is in `patch-vocabulary.md`.

## Audit trail

Every sync writes `.fork/snapshots/<date>-<sha>.json`:

```json
{
  "upstream_sha": "...",
  "pre_sync_main_sha": "...",
  "merged_commit_sha": "...",
  "ci_result": "pass|fail|pending",
  "llm_resolutions": [
    {"file": "src/foo.c", "patch": "quiet-flag", "outcome": "resolved|design_conflict|cache_hit"}
  ]
}
```

Combined with `upstream-sync/*` and `sync/*-merged` tags, any past sync is fully reconstructable.

## Rollback

```
# list sync points
git tag -l 'upstream-sync/*'
git tag -l 'sync/*-merged'

# restore the tree as of a specific sync
git checkout sync/<date>-<sha>-merged
```

Works offline — every imported upstream SHA is in the local object store.

## What lives where

- `.fork/AGENTS.md` — contract. Short enough to skim.
- `.fork/README.md` — human-facing readme.
- `.fork/revision.txt` — current upstream pin. One file, four lines.
- `.fork/patches/` — derived flat-file patch inventory. Do not edit.
- `.fork/snapshots/` — per-sync audit JSON.
- `.fork/.llm-cache/` — resolver cache. Gitignored.
- `.fork/references/` — LLM-consumable docs (this file, resolver prompt, doctor checklist, patch vocabulary).
- `.fork/tools/` — shell + python helpers called by workflows and by humans.
- `.fork/skills/` — repo-local agent skills.

## Constraints to respect

- Never force-push `main`. Ever.
- `.github/workflows/` uses the `fork-*.yml` prefix. Don't drop new workflow files at root without the prefix.
- Fork edits at root get `Fork-Patch:` + `Reason:` trailers. Commits without trailers won't be picked up as patches.
- `.fork/patches/` is derived. If you hand-edit a file there, the next sync overwrites you. Edit the commit instead, then run `.fork/tools/export-patches.sh`.
- `upstream` branch is a mirror. Don't commit to it.
