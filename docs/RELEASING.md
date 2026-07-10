---
summary: "Serialized local release contract for signed and notarized GogCLI artifacts"
---

# Releasing `gogcli`

Official releases are serialized maintainer operations. Apple signing and
notarization happen only on the authorized local Mac through the managed
Developer ID release keychain and the `release-mac-app` `codesign-run`
boundary. GitHub Actions never receives Apple credentials or certificate
material.

## Identity and migration invariants

- The Mach-O identifier remains `com.steipete.gogcli.gog`.
- New official Darwin binaries use `Developer ID Application: OpenClaw Foundation (FWJYW4S8P8)` and Team ID `FWJYW4S8P8`.
- The canonical designated requirement keeps the identifier and Apple Developer ID anchors, with `certificate leaf[subject.OU] = FWJYW4S8P8`.
- v0.33.0 used Team ID `Y5PE65HELJ`. Because the Team ID is part of the designated requirement, moving to the Foundation necessarily changes that requirement once even though the identifier is preserved. Existing Keychain ACL entries created for the old requirement may therefore require one authorization or ACL refresh. Releases after the migration retain Foundation-Team continuity.
- Do not change the identifier to avoid or disguise the Team migration. Gog's Keychain continuity depends on keeping it stable.

## Credential boundary

- `.mac-release.env` is local-only and ignored. Never commit it or copy its keychain/profile locators into scripts, workflows, examples, logs, or review comments.
- `.release-state/` is ignored and may contain only the non-secret, hash-bound Homebrew install recovery record. It must never contain keychain, notary, identity, token, or 1Password locators.
- `NOTARYTOOL_KEYCHAIN_PROFILE` is injected at runtime by the configured 1Password package-secret boundary. Never store its value in GitHub, Git, or shell history.
- The runtime manifest's package-field allowlist must contain only `NOTARYTOOL_KEYCHAIN_PROFILE`. A pinned system-Python bridge first starts the canonical helper with a clean fixed environment, then copies only that profile, the managed codesign routing, and—only for draft creation—`GITHUB_TOKEN` into a second explicit environment before executing GoReleaser. Ambient shell functions and unrelated variables cannot cross either boundary.
- `MAC_RELEASE_HELPER` names the absolute, regular canonical `release-mac-app` helper at runtime; its locator stays local. `scripts/release-local pilot` and `scripts/release-local draft` invoke that helper's `codesign-run --with-package-secrets` boundary. Ordinary GoReleaser builds leave Darwin binaries untouched.
- Official producer tools are not taken from the ambient `PATH`. Before the credential boundary, the local lane downloads the native Go 1.26.5 and GoReleaser 2.17.0 release archives with an empty environment, verifies repository-pinned SHA-256 values for both host architectures, extracts them into the ephemeral release directory, and rechecks the pinned archives and binaries immediately before and after signing.
- `GOG_OFFICIAL_RELEASE=1` is set only inside that bounded helper. When set, missing or wrong identity, managed-keychain routing, notary routing, notarization status, Team ID, identifier, timestamp, hardened runtime, designated requirement, or online notarization proof fails closed without replacing the unsigned GoReleaser candidate.

## Artifact contract

The release contains exactly seven assets:

- `checksums.txt`
- `gogcli_VERSION_darwin_amd64.tar.gz`
- `gogcli_VERSION_darwin_arm64.tar.gz`
- `gogcli_VERSION_linux_amd64.tar.gz`
- `gogcli_VERSION_linux_arm64.tar.gz`
- `gogcli_VERSION_windows_amd64.zip`
- `gogcli_VERSION_windows_arm64.zip`

Every archive contains only `gog` or `gog.exe`. Both Darwin binaries are thin,
signed, hardened-runtime, timestamped, notarized Foundation binaries. Every
binary must report Go 1.26.5, the exact signed-tag `vcs.revision`,
`vcs.modified=false`, and its matching OS/architecture. Build and archive
member mtimes are derived from the tagged commit. Linux and Windows payloads
are independently rebuilt byte-for-byte from the signed tag.

The repository-owned `.github/release-allowed-signers` pins the annotated tag
signer. Proof binds the tag object and peeled commit separately. Mutable release
inventory is frozen before verification and checked again before the native
candidate runs.

`.github/workflows/release.yml` is a read-only contract/snapshot check. A tag
cannot publish through it. `.github/workflows/release-assets.yml` runs the
protected-default verifier on native Apple Silicon and Intel runners. Its token
is scoped to the exact asset-download step; static verification, independent
rebuilds, and final isolated execution are token-free. CI holds no Apple
signing or notary credentials.

## Preparation

For v0.33.1, finalize the top changelog heading with the release date and set
`internal/cmd/VERSION` to `v0.33.1`. Run the repository gates, review the diff,
commit, push, and require exact-head CI before creating the signed annotated
tag. Release notes are extracted from the tagged changelog; mutable worktree
notes are not accepted. The verifier also models GoReleaser v2.17's single
release-body renderer newline and compares the API body byte-for-byte.

The local no-public-mutation precheck is:

```sh
scripts/release-local --check
```

## Serialized public gates

Run only one gate at a time and only after explicit maintainer authorization:

```sh
# Submits ephemeral binaries to Apple, but creates no tag or GitHub release.
scripts/release-local pilot v0.33.1

# After the reviewed release commit is on protected main:
git tag -s v0.33.1 -m "Release 0.33.1"
git push origin v0.33.1

# Wait for successful exact-tag `ci` and read-only `release-check` runs.
# Every later local gate verifies both run identities, tag commit, and success.

# Builds from a fresh clone of the exact signed tag and creates a GitHub draft.
scripts/release-local draft

# Dispatches and accepts only exact two-architecture protected-default proof.
scripts/release-local verify-draft v0.33.1

# Revalidates tag, notes, assets, and proof before publishing the frozen draft.
scripts/release-local publish v0.33.1

# Re-verifies public assets, dispatches the hash-bound tap update, and proves a
# clean Homebrew install from Formula/gogcli.rb.
scripts/release-local homebrew v0.33.1
```

The publish and Homebrew gates are safe to rerun after an interrupted network
response or later proof failure. `publish` accepts an already-published release
only after the signed tag, exact-tag checks, notes, release fields, and full
asset snapshot validate twice; it never sends another PATCH in that path.
`homebrew` resumes only when the protected tap head is the exact formula-only
updater commit, its provenance and successful workflow run match the accepted
tag, and its parent still contains the frozen updater contract. If this lane
previously installed Gog before a later check failed, its hash-bound local
recovery record permits verification of that exact install. An unrelated
pre-existing install still fails closed. Do not delete or edit recovery state
to bypass a mismatch; investigate the changed release or tap state instead.

The Homebrew path is `openclaw/homebrew-tap/Formula/gogcli.rb`, installed as
`openclaw/tap/gogcli`; the formula must install the verified `gog` archive
member without rebuilding it.

## Post-public proof and closeout

On a clean macOS VM, download each Darwin asset through a normal browser so
quarantine is applied naturally. Extract normally and run `gog --version`.
No Gatekeeper override is allowed.

For Keychain continuity, use a controlled non-production account and the same
installed path. Confirm the v0.33.0 binary can read its existing test credential,
replace it in place with v0.33.1, record any one-time authorization caused by
the known Team-bound requirement migration, and confirm subsequent v0.33.1
reads do not prompt again. Do not claim continuity from source inspection alone.

Only after GitHub, both native verifier jobs, public downloads, release notes,
Homebrew install/test, Gatekeeper, and the migration observation are complete:

```sh
scripts/start-next-release.sh v0.33.1
git diff -- CHANGELOG.md internal/cmd/VERSION
committer "chore(release): start v0.33.2" CHANGELOG.md internal/cmd/VERSION
git push origin main
git pull --ff-only
git status -sb
```

The closeout script opens `0.33.2 - Unreleased` and sets
`internal/cmd/VERSION` to `v0.33.1-dev`. There is no automatic post-tag writer;
the explicit closeout commit is the sole next-release transition.
