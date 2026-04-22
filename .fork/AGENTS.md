# Fork contract — gogcli

Full operating contract for any agent working in this repo. The root `AGENTS.md` routes here.

- Upstream: [steipete/gogcli](https://github.com/steipete/gogcli.git)
- Tracks branch: `main`
- Contract version: $fork_contract_version
- Initialized: $setup_iso_date
- LLM provider for auto-resolve: `claude` (``)

## Repo shape

```
<repo root>/                  upstream files at their original paths (may carry fork patches)
├── AGENTS.md                 root pointer → this file
├── CLAUDE.md                 symlink → AGENTS.md
├── .claude/skills → .fork/skills
├── .agents/skills → .fork/skills
├── .mergify.yml              auto-merge rules (lives at root; Mergify requires it)
├── .github/workflows/fork-*.yml
└── .fork/
    ├── AGENTS.md             this file
    ├── README.md             human-facing fork readme
    ├── revision.txt          current upstream SHA + sync tag
    ├── upstream-AGENTS.md    preserved if upstream shipped one
    ├── upstream-README.md    preserved if upstream shipped one
    ├── patches/              derived (git format-patch); do not edit files here
    ├── snapshots/            per-sync audit JSON (<date>-<sha>.json)
    ├── .llm-cache/           gitignored resolver cache
    ├── references/           LLM-consumable docs (prompts, architecture, vocab)
    ├── tools/                sync.sh, export-patches.sh, upstream-patch.sh, llm_resolve.py
    └── skills/               repo-local skills
```

## Branches and tags

- `main` — default branch. Upstream code plus fork patches applied as commits on top. **Never force-pushed.** Moves append-only via squash-merges of `sync/*` PRs.
- `upstream` — pristine mirror of `steipete/gogcli@main`. Force-updated every sync. No fork commits ever land here.
- `sync/<date>-<sha>` — ephemeral branch per sync. Created from `main`, rebased onto new `upstream`, resolved, PR'd into `main`, deleted after merge.
- `upstream-sync/<date>-<sha>` — tag at every imported upstream SHA (durable rollback anchor).
- `sync/<date>-<sha>-merged` — tag preserving per-patch SHAs after the squash-merge.
- `release/<upstream-tag>` — tag when upstream cuts a release.

## Patches

A "fork patch" is a git commit carrying these trailers:

```
Fork-Patch: <kebab-case-slug>
Reason: <one line on why this patch exists>
```

The `Fork-Patch:` trailer is what `git format-patch --grep='Fork-Patch:'` selects when regenerating `.fork/patches/`. The `Reason:` trailer is fed to the LLM resolver so it knows intent when patches drift.

## Add a feature

See `.fork/skills/add-feature/` for the full playbook. Summary:

1. Edit upstream files at the repo root. TDD — write a failing test first.
2. Commit with both trailers.
3. Run `.fork/tools/export-patches.sh` to refresh `.fork/patches/`.
4. Amend or follow-up commit to include the `.fork/patches/` delta.
5. Push, open PR against `main`.

## Sync with upstream

Cron `0 6 * * *` runs `.github/workflows/fork-upstream-sync.yml`. Manual runs via `.fork/tools/sync.sh`. Both do the same thing:

1. Fetch upstream. Exit clean if HEAD matches `.fork/revision.txt`.
2. Tag `upstream-sync/<date>-<sha>` at upstream HEAD (before touching `upstream` branch).
3. Reset `upstream` branch to upstream HEAD.
4. Create `sync/<date>-<sha>` from `main`, rebase onto `upstream`.
5. On conflict: `.fork/tools/llm_resolve.py` handles each conflicted file.
6. Regenerate `.fork/patches/`, write `.fork/snapshots/<date>-<sha>.json`.
7. Open PR against `main`. Mergify squash-merges on green CI.

See `.fork/skills/sync-upstream/` for the manual variant.

## Upstream a patch back

```
.fork/tools/upstream-patch.sh <slug>
```

Cherry-picks the `Fork-Patch: <slug>` commit onto a fresh branch from upstream, pushes to your upstream-facing fork, opens a PR against `steipete/gogcli`. Or grab `.fork/patches/NNNN-<slug>.patch` and send it manually.

## Conflict resolution

When a rebase hits a conflict, `.fork/tools/llm_resolve.py` loads the prompt at `.fork/references/resolver-prompt.md`, this contract, and the conflicted commit's trailers. It writes back either the resolved file or an inline `DESIGN_CONFLICT: <reason>` marker.

`DESIGN_CONFLICT:` means the resolver bailed. Procedure:

1. The sync PR is labeled `needs-human` (not `automerge`). Mergify will not merge it.
2. A tracking issue is filed automatically.
3. Open the file; find the `DESIGN_CONFLICT:` line. The text after the colon explains what the resolver couldn't decide.
4. Make the call yourself. Either fix the conflict and remove the marker, or revise the patch's `Reason:` trailer to reflect a new decision, then rebase.
5. Push the fix to the sync branch. CI reruns; Mergify re-evaluates.

See `.fork/skills/resolve-conflict/` for the full loop.

## Smoke test

```
go test ./...
```

Run this after any manual resolution, before pushing. CI also runs it on every sync PR via `fork-build-release.yml`.

## Doctor

`.fork/skills/doctor/` has the audit checklist. Run it when the repo feels off — tags missing, patches not regenerated, `.fork/` drifted from `main`.

## Pointers

- `.fork/references/architecture.md` — operational architecture of this repo.
- `.fork/references/resolver-prompt.md` — LLM resolver prompt template.
- `.fork/references/patch-vocabulary.md` — structured vocabulary for patch/source state.
- `.fork/references/doctor-checklist.md` — full audit list.
