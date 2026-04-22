<!-- fork-notice:begin (managed by setup-downstream-fork; do not edit) -->
# Fork notice

This repository is a **downstream fork** of [`steipete/gogcli`](https://github.com/steipete/gogcli.git).
It auto-tracks upstream via a daily GitHub Actions workflow and carries our patches on top of `main`; an LLM resolves drift, humans only get paged for real design conflicts.

- Full fork contract: [`.fork/AGENTS.md`](.fork/AGENTS.md) (read this first before editing)
- Current upstream SHA: see [`.fork/revision.txt`](.fork/revision.txt)
- Inventory of our patches: [`.fork/patches/`](.fork/patches/)
- Sync + build + conflict-resolve workflows: [`.github/workflows/fork-*.yml`](.github/workflows/)
- Repo-local agent skills: [`.fork/skills/`](.fork/skills/) (auto-discovered via `.claude/skills` and `.agents/skills`)

Convention: source edits go at the repo root (same paths upstream uses); commits carry `Fork-Patch: <slug>` + `Reason: <why>` trailers.
<!-- fork-notice:end -->

# Repository Guidelines

## Project Structure

- `cmd/gog/`: CLI entrypoint.
- `internal/`: implementation (`cmd/`, Google API/OAuth, config/secrets, output/UI).
- Tests: `*_test.go` next to code; opt-in integration suite in `internal/integration/` (build-tagged).
- `bin/`: build outputs; `docs/`: specs/releasing; `scripts/`: release helpers + `scripts/gog.mjs`.

## Build, Test, and Development Commands

- `make` / `make build`: build `bin/gog`.
- `make tools`: install pinned dev tools into `.tools/`.
- `make fmt` / `make lint` / `make test` / `make ci`: format, lint, test, full local gate.
- Optional: `pnpm gog …`: build + run in one step.
- Hooks: `lefthook install` enables pre-commit/pre-push checks.

## Coding Style & Naming Conventions

- Formatting: `make fmt` (`goimports` local prefix `github.com/steipete/gogcli` + `gofumpt`).
- Output: keep stdout parseable (`--json` / `--plain`); send human hints/progress to stderr.
- Gmail labels: treat label IDs as case-sensitive opaque tokens; only case-fold label names for name lookup.

## Testing Guidelines

- Unit tests: stdlib `testing` (and `httptest` where needed).
- Integration tests (local only):
  - `GOG_IT_ACCOUNT=you@gmail.com go test -tags=integration ./internal/integration`
  - Requires OAuth client credentials + a stored refresh token in your keyring.

## Commit & Pull Request Guidelines

- Create commits with `committer "<msg>" <file...>`; avoid manual staging.
- Follow Conventional Commits + action-oriented subjects (e.g. `feat(cli): add --verbose to send`).
- Group related changes; avoid bundling unrelated refactors.
- PRs should summarize scope, note testing performed, and mention any user-facing changes or new flags.
- PR review flow: when given a PR link, review via `gh pr view` / `gh pr diff` and do not change branches.

### PR Workflow (Review vs Land)

- **Review mode (PR link only):** read `gh pr view/diff`; do not switch branches; do not change code.
- **Landing mode:** temp branch from `main`; bring in PR (squash default; rebase/merge when needed); fix; update `CHANGELOG.md` (PR #/issue + thanks); run `make ci`; final commit; merge to `main`; delete temp; end on `main`.
- If landing contributor work, always add `Co-authored-by:` trailers for PR authors, even when we partially rewrite, group, or manually apply their changes; leave a PR comment with what landed + SHAs.
- New contributor: thank in `CHANGELOG.md` (and update README contributors list if present).

## Security & Configuration Tips

- Never commit OAuth client credential JSON files or tokens.
- Prefer OS keychain backends; use `GOG_KEYRING_BACKEND=file` + `GOG_KEYRING_PASSWORD` only for headless environments.
