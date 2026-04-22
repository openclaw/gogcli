# gogcli

Downstream fork of [steipete/gogcli](https://github.com/steipete/gogcli.git), tracking the `main` branch.

## Why this fork exists

<!-- TODO: fill in. One or two sentences on what this fork does that upstream doesn't. -->

## What it tracks

- Upstream: https://github.com/steipete/gogcli.git
- Upstream branch: `main`
- Upstream SHA at init: `$upstream_default_sha`
- Initialized: $setup_iso_date
- Sync schedule: `0 6 * * *` (cron)

Current upstream pin lives in [`.fork/revision.txt`](./.fork/revision.txt). Patch inventory lives in [`.fork/patches/`](./.fork/patches/).

## Clone and build

```
git clone <this-repo-url> gogcli
cd gogcli
go build ./...
```

Smoke test:

```
go test ./...
```

## Layout

Upstream's files are at the repo root where upstream put them. This fork's scaffolding lives under `.fork/`. `main` is the default branch and contains upstream plus fork patches applied as commits on top. See [`.fork/AGENTS.md`](./.fork/AGENTS.md) for the full contract.

If upstream shipped its own README, it is preserved at [`.fork/upstream-README.md`](./.fork/upstream-README.md).

## Day-to-day

- Add a feature: see `.fork/skills/add-feature/SKILL.md`.
- Sync with upstream manually: `.fork/tools/sync.sh`.
- Send a patch upstream: `.fork/tools/upstream-patch.sh <slug>`.
- Audit the fork: see `.fork/skills/doctor/SKILL.md`.

## Cruise control

A cron job (`0 6 * * *`) in `.github/workflows/fork-upstream-sync.yml` fetches upstream, rebases patches, asks the configured LLM (claude / ) to resolve boring conflicts, and opens a PR. Mergify squash-merges on green CI. Human review is required only when the resolver bails with `DESIGN_CONFLICT:`.
