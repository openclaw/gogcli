# .fork/patches/ — derived inventory

This directory is regenerated on every sync by `.fork/tools/export-patches.sh`, which runs `git format-patch upstream..main --grep='Fork-Patch:'`. The files here are a flat-file mirror of the `Fork-Patch:` commits on `main`.

**Do not edit files in this directory.** They will be overwritten.

To change a patch:

1. Edit the corresponding git commit on `main` (interactive rebase, amend, or revert + new commit).
2. Run `.fork/tools/export-patches.sh` to regenerate this directory.
3. Commit the regenerated patches as part of your change.

See `.fork/skills/refresh-patches/SKILL.md` for the full refresh flow and `.fork/AGENTS.md` for the patch model.

Upstream tracked: [steipete/gogcli](https://github.com/steipete/gogcli.git), branch `main`.
