---
name: add-feature
description: Add a new feature or customization to this downstream fork of steipete/gogcli. Use when the user wants to patch upstream behavior, add a flag, disable a subsystem, or otherwise modify root-level source files. Commits the change with the required Fork-Patch and Reason trailers, regenerates .fork/patches/, and opens a PR against main.
---

# add-feature

Use when adding or modifying a fork patch. This is the default "I want to change something" flow in this repo.

## Prerequisites

- You are on `main` (or a branch off `main`).
- Working tree is clean.
- You know what upstream files the change touches.

## Steps

1. **Write a failing test first.** TDD. The test should exercise the new behavior and fail against current `main`.

2. **Edit upstream files at the repo root.** Use upstream's paths, don't relocate files. If the change spans many files, that's fine — it's still one patch.

3. **Run the smoke test locally.**
   ```
   go test ./...
   ```
   Iterate until green.

4. **Commit with both trailers.** The trailers are what make this a tracked fork patch.
   ```
   git commit -m "feat(<scope>): <short summary>

   Fork-Patch: <kebab-case-slug>
   Reason: <one sentence on why this exists>"
   ```
   Pick a slug that reads well in a filename (`.fork/patches/NNNN-<slug>.patch`).

5. **Regenerate the patch inventory.**
   ```
   .fork/tools/export-patches.sh
   ```
   This rewrites `.fork/patches/` from the `Fork-Patch:` commits on `main`.

6. **Fold the patches/ update into the commit** (or add a follow-up, either works):
   ```
   git add .fork/patches/
   git commit --amend --no-edit
   ```

7. **Push and open the PR.**
   ```
   git push -u origin feat/<slug>
   gh pr create --base main
   ```

8. CI runs build + smoke test. Mergify squash-merges on green.

## Tools involved

- `.fork/tools/export-patches.sh` — regenerates `.fork/patches/` from `git format-patch`.
- `.fork/tools/upstream-patch.sh <slug>` — if you also want to send this patch upstream later.

## References

- `.fork/AGENTS.md` — the fork contract (patch model, trailers, branch rules).
- `.fork/references/architecture.md` — full operational picture.

## What NOT to do

- Don't edit files under `.fork/patches/` directly. They're derived; the next sync overwrites you.
- Don't skip the `Fork-Patch:` trailer. Untrailed commits don't get picked up as patches and will silently disappear on the next rebase.
- Don't commit on the `upstream` branch. That branch is a mirror.
