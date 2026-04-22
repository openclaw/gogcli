# Doctor checklist — gogcli

Human-readable version of the audit `.fork/skills/doctor/` runs. Use it to spot-check the repo, or follow along while `scripts/doctor.py` runs from the `setup-downstream-fork` skill. Each item is `ok`, `fixable`, or `broken`.

## Layout

- [ ] `.fork/` exists at repo root.
- [ ] Root `AGENTS.md` is the short pointer (< 30 lines, routes to `.fork/AGENTS.md`).
- [ ] `CLAUDE.md` is a symlink to `AGENTS.md`.
- [ ] `.claude/skills` symlinks to `../.fork/skills`.
- [ ] `.agents/skills` symlinks to `../.fork/skills`.
- [ ] `.fork/AGENTS.md`, `.fork/README.md`, `.fork/revision.txt` all exist.
- [ ] `.fork/references/` has `architecture.md`, `resolver-prompt.md`, `doctor-checklist.md`, `patch-vocabulary.md`.
- [ ] `.fork/tools/` has `sync.sh`, `export-patches.sh`, `upstream-patch.sh`, `llm_resolve.py`, `check-upstream.sh`.
- [ ] `.fork/skills/` has `add-feature/`, `refresh-patches/`, `sync-upstream/`, `resolve-conflict/`, `doctor/`.

## Branches

- [ ] `upstream` branch exists.
- [ ] `upstream` branch's HEAD matches the SHA recorded in `.fork/revision.txt`.
- [ ] `main` exists, is the default branch, and descends from an ancestor present on `upstream`.
- [ ] `main` has never been force-pushed (reflog check).

## Tags

- [ ] At least one `upstream-sync/<date>-<sha>` tag exists.
- [ ] Most recent `upstream-sync/*` tag is within the last 14 days (warn if older).
- [ ] For every merged sync PR, a `sync/<date>-<sha>-merged` tag exists.
- [ ] `release/*` tags exist for every upstream release since the fork was created.

## Patches

- [ ] `.fork/patches/` matches the output of `git format-patch upstream..main --grep='Fork-Patch:'`.
- [ ] Every `Fork-Patch:` commit on `main` also has a `Reason:` trailer.
- [ ] No commit on `upstream` branch carries a `Fork-Patch:` trailer.

## Workflows and Mergify

- [ ] `.github/workflows/fork-upstream-sync.yml` is present and valid YAML.
- [ ] `.github/workflows/fork-build-release.yml` is present and valid YAML.
- [ ] `.github/workflows/fork-conflict-resolve.yml` is present and valid YAML.
- [ ] `.mergify.yml` is present and includes the `fork-sync` queue rule.
- [ ] Mergify app is installed on the repo.
- [ ] Branch protection on `main` requires `build`, `smoke-test`, `drift-recheck` status checks.
- [ ] The provider secret (`ANTHROPIC_API_KEY` or `OPENAI_API_KEY`) is set on the repo.

## Snapshots

- [ ] `.fork/snapshots/` has a JSON file per sync (name: `<date>-<sha>.json`).
- [ ] Every snapshot has `upstream_sha`, `pre_sync_main_sha`, `merged_commit_sha`, `ci_result`, `llm_resolutions`.

## Hygiene

- [ ] `.fork/.llm-cache/` is in `.gitignore`.
- [ ] `.fork/.llm-cache/` is not tracked by git.
- [ ] No `DESIGN_CONFLICT:` markers in any tracked file on `main`.
- [ ] `go test ./...` passes on `main`.
