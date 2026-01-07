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
- If we squash, add `Co-authored-by:` for the PR author when appropriate; leave a PR comment with what landed + SHAs.
- New contributor: thank in `CHANGELOG.md` (and update README contributors list if present).

## Security & Configuration Tips

- Never commit OAuth client credential JSON files or tokens.
- Prefer OS keychain backends; use `GOG_KEYRING_BACKEND=file` + `GOG_KEYRING_PASSWORD` only for headless environments.
