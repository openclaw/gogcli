---
name: refresh-patches
description: Regenerate .fork/patches/ from the Fork-Patch commits on main. Use after rebasing or editing a commit interactively, when .fork/patches/ is out of sync with the git log. One command, fast.
---

# refresh-patches

Use when `.fork/patches/` no longer matches the `Fork-Patch:` commits on `main` — usually after an interactive rebase that reworded, reordered, or squashed a patch commit.

## Why this exists

`.fork/patches/` is derived from `git format-patch upstream..main --grep='Fork-Patch:'`. If you rebase or amend a fork-patch commit, the flat-file inventory drifts from the commit history until it's regenerated. The sync workflow will regenerate it eventually, but you want `.fork/patches/` clean before pushing so CI sees consistent state.

## Steps

1. Run the tool:
   ```
   .fork/tools/export-patches.sh
   ```

2. Verify the tree is clean:
   ```
   git diff --quiet -- .fork/patches/
   ```
   If this exits 0, you're done. If it reports changes, stage and commit them:
   ```
   git add .fork/patches/
   git commit -m "fork: refresh patches"
   ```

3. Push.

## When to run it

- After `git rebase -i upstream` on a feature branch.
- After `git commit --amend` on a `Fork-Patch:` commit.
- After reverting a patch (`git revert`) — the patch file should disappear from `.fork/patches/`.
- Any time `.fork/patches/` feels stale.

## Tools involved

- `.fork/tools/export-patches.sh` — the regenerator.

## References

- `.fork/references/architecture.md` — section on the patch model.
