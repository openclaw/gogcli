---
summary: "Unified GitHub Actions release process for GogCLI"
---

# Releasing `gogcli`

Official releases run only through `.github/workflows/release-unified.yml`, which calls the fleet-standard `openclaw/release-workflows` Go CLI pipeline at `@v1`. Binary signing, notarization, and publication stay in Actions. After a successful dry run, create and verify the release tag with the fleet release SSH key before dispatching publication.

Release authorization is repository Actions write access combined with a protected, required-check-green `main` commit and the organization-scoped signing, notarization, and tap credentials. The shared workflow reuses the immutable signed annotated tag, verifies its target, and never moves it. Use only the fleet release SSH key (fingerprint `SHA256:WmI9lVtd7F2c5XyRHbZVO3yYYJzwsSNzcZQMPT147HI`), not the local default signing key. Verify the signature before pushing; if the local allowlist does not trust that key, use a task-local allowed-signers file without changing trust settings.

## Prepare the release

1. Update dependencies and land the complete release queue.
2. Finalize one dated `## X.Y.Z - YYYY-MM-DD` section at the top of `CHANGELOG.md`.
3. Set `internal/cmd/VERSION` to `vX.Y.Z`.
4. Run `make ci`, review the complete diff, and land it on protected `main` with required checks green.

The workflow first runs a read-only `xcrun notarytool history` preflight on the `xcode-27` runner. The Darwin CI and release builds pin `MACOSX_DEPLOYMENT_TARGET=15.0`, include `-mmacosx-version-min=15.0` in the cgo compiler/linker flags so cached objects use the same target, and check every binary slice with `otool` before signing or publication.

The workflow freezes the protected `main` commit, creates or reuses its immutable annotated version tag, builds from that commit, signs and notarizes the native Darwin binaries with organization secrets, verifies the complete asset inventory independently for arm64 and x86_64 (running Intel binaries under Rosetta on Apple Silicon), publishes the GitHub Release, and updates `openclaw/homebrew-tap/Formula/gogcli.rb` from verifier-bound asset hashes.

The caller preserves GogCLI's public artifact contract where applicable:

- native Darwin, Linux, and Windows archives from the GoReleaser matrix;
- `checksums.txt` as the checksum asset;
- the established `com.steipete.gogcli.gog` signing identifier;
- no additional universal Darwin archive;
- non-Darwin reproducible rebuild verification;
- nFPM auto-detection, which remains inactive unless the GoReleaser config adds packages.

## Dispatch

First run the dry run described below on the protected release-prep merge commit. Once it succeeds, create the fleet-signed annotated `vX.Y.Z` tag at that exact commit, verify it, and push it. From current protected `main`, dispatch the workflow with a SemVer version without a leading `v`:

```sh
gh workflow run release-unified.yml \
  --repo openclaw/gogcli \
  --ref main \
  -f version=X.Y.Z
```

`scripts/release.sh X.Y.Z` is the equivalent convenience wrapper. Watch the exact returned workflow run through completion. Retrying the same version reuses the immutable annotated tag; the workflow never moves it.

To validate the pipeline without creating a tag, release, or tap update, dispatch
the same workflow with `-f dry_run=true` and the numeric version recorded in
`internal/cmd/VERSION`, omitting its development suffix (for example, `0.44.0`
for `v0.44.0-dev`). This version must have a dated changelog section. The dry run
builds the current protected `main` head, signs, and verifies its artifacts;
it does not rebuild the older release tag. A real release still requires an
exact version match without the development suffix.

## Docker closeout

After the GitHub Release is public and its tag is verified, publish the Docker image separately unless an exact-tag Docker run has already succeeded. A signed tag pushed by the release operator normally triggers Docker's tag-push workflow; tags created with `GITHUB_TOKEN` do not.

```sh
gh workflow run docker.yml \
  --repo openclaw/gogcli \
  --ref vX.Y.Z \
  -f tag=vX.Y.Z
```

Use the release tag for both `--ref` and `tag`: checkout follows the input, but build commit metadata comes from the dispatch's `GITHUB_SHA`. Dispatching from `main` can label a tagged build with a newer commit. Verify the resulting run's head SHA matches the frozen release commit and wait for success before treating the GHCR version tag (and `latest` for stable releases) as published.

## Verify and close out

Before declaring the release complete, verify:

- the exact workflow run succeeded;
- the annotated tag resolves to the frozen protected-main commit;
- the GitHub Release is published and its body matches the tagged changelog section;
- the published checksum and inventory controls cover every asset;
- both native macOS verifier jobs passed signature, Team ID, stable identifier, hardened-runtime, timestamp, architecture, and online notarization checks;
- both native macOS binaries report `minos 15.0` in `otool -l`, matching the macOS 15 minimum documented in the install guide;
- the Homebrew handoff succeeded and `Formula/gogcli.rb` contains the verified version, archive names, and hashes.

Finally, land the next patch `Unreleased` changelog section and set `internal/cmd/VERSION` to the released version with the `-dev` suffix. The reusable workflow may open a closeout PR containing only the changelog heading; add the development-version change to that exact PR, then review and merge it rather than creating a competing transition. Keep exactly one `Unreleased` section.
