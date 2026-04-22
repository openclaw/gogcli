---
name: sync-upstream
description: Manually sync this fork with upstream steipete/gogcli. Use as an alternative to the cron-driven GitHub workflow when you want to pull latest upstream right now instead of waiting for the next tick. Drives the same steps the workflow runs — tag upstream SHA, rebase patches, invoke LLM resolver on conflicts, regenerate .fork/patches/, open PR.
---

# sync-upstream

Use when you want a sync to happen right now instead of waiting for the cron (`0 6 * * *`). Runs the same steps as `.github/workflows/fork-upstream-sync.yml`, locally.

## Prerequisites

- Working tree clean.
- You have the provider env var set for the resolver:
  - `claude` → `ANTHROPIC_API_KEY` or `OPENAI_API_KEY`.
- `gh` CLI authenticated (for PR creation).

## Steps

1. **Run the sync script.**
   ```
   .fork/tools/sync.sh
   ```

   The script:
   - Fetches upstream `main`.
   - Exits clean if upstream HEAD matches `.fork/revision.txt`.
   - Tags `upstream-sync/<date>-<sha>` at upstream HEAD.
   - Fast-forwards the `upstream` branch.
   - Creates `sync/<date>-<sha>` from `main`, rebases onto `upstream`.
   - On each conflict, calls `.fork/tools/llm_resolve.py` with the conflicted file.
   - Regenerates `.fork/patches/` and `.fork/revision.txt`.
   - Writes `.fork/snapshots/<date>-<sha>.json`.
   - Pushes the sync branch and opens a PR.

2. **Check the PR.** If the resolver bailed anywhere, the PR body has a `### DESIGN_CONFLICT:` section and the `needs-human` label. Follow `.fork/skills/resolve-conflict/` to fix.

3. **Otherwise, wait for CI.** Build, smoke test, drift-recheck. Mergify merges on green.

## If it fails

- **"nothing to do" / clean exit** — upstream hasn't moved. Nothing to fix.
- **Rebase hits a conflict the resolver can't fix** — the script leaves the sync branch with `DESIGN_CONFLICT:` markers. Push the branch, open PR, label `needs-human`. Don't try to force a resolution; the marker is load-bearing.
- **Smoke test fails after resolver claims success** — the resolution compiled but regressed behavior. Roll back via `git reset --hard sync/<date>-<sha>-start` (the script saves this ref) and re-run with a different `` if you have access.

## Tools involved

- `.fork/tools/sync.sh` — the driver.
- `.fork/tools/llm_resolve.py` — called per conflicted file.
- `.fork/tools/export-patches.sh` — called at the end to regenerate `.fork/patches/`.

## References

- `.fork/references/architecture.md` — sync mechanism in detail.
- `.fork/references/resolver-prompt.md` — prompt the resolver loads.
