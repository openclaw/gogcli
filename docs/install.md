# Install

`gog` ships as a single binary. The visible version is injected at build time:
release builds use the tag, while local builds use `git describe`.

## Homebrew (macOS, Linux)

```bash
brew install openclaw/tap/gogcli
gog --version
```

The Homebrew formula lives in `openclaw/homebrew-tap` and installs the `gog`
binary. Release verification should run:

```bash
brew test openclaw/tap/gogcli
gog --version
```

## Docker / GHCR

Release tags publish a non-root GitHub Container Registry image:

```bash
docker run --rm ghcr.io/openclaw/gogcli:latest version
docker run --rm ghcr.io/openclaw/gogcli:v0.15.0 version
```

Authenticated container runs should mount a persistent `GOG_HOME` directory and
use the encrypted file keyring:

```bash
docker volume create gogcli-state

docker run --rm -it \
  -e GOG_HOME=/persist/gogcli \
  -e GOG_KEYRING_BACKEND=file \
  -e GOG_KEYRING_PASSWORD \
  -v gogcli-state:/persist/gogcli \
  ghcr.io/openclaw/gogcli:latest \
  auth add you@gmail.com --services gmail,calendar,drive
```

Keep `GOG_KEYRING_PASSWORD` in the shell session or your CI secret store. Do
not bake it into images, scripts, or checked-in profiles.
See [Paths and State](paths.md) for `GOG_HOME`, per-kind `GOG_*_DIR`
overrides, and legacy path compatibility.

## Headless agents and systemd

For headless agents, configure `gog` with the encrypted file keyring or the
1Password backend and pass the same environment to the process that will
actually invoke `gog`. A command working in your login shell only proves that
shell has the required secret; it does not prove a systemd service, gateway, or
agent subprocess inherited it.

Use this as the minimum runtime environment:

```ini
Environment=GOG_KEYRING_BACKEND=file
Environment=GOG_KEYRING_PASSWORD=replace-with-secret-manager-injection
Environment=GOG_HOME=/var/lib/gogcli
Environment=HOME=/home/openclaw
```

For headless 1Password-backed storage, replace the keyring lines with:

```ini
Environment=GOG_KEYRING_BACKEND=1password
Environment=GOG_1PASSWORD_AUTH=service-account
Environment=OP_SERVICE_ACCOUNT_TOKEN=replace-with-service-account-token
Environment=GOG_1PASSWORD_VAULT=replace-with-vault-id
```

Desktop-app auth is for local interactive agents where the 1Password app is
running, unlocked, and configured to integrate with other apps. Use
`GOG_1PASSWORD_ACCOUNT` there instead of `OP_SERVICE_ACCOUNT_TOKEN`.
For local agents, the non-secret 1Password selectors can also live in
`config.json`:

```bash
gog auth keyring 1password
gog config set onepassword_auth desktop
gog config set onepassword_account buildkite.1password.com
gog config set onepassword_vault replace-with-vault-id
```

Items are created as API Credential entries titled `gogcli-keyring` by default;
set `Environment=GOG_1PASSWORD_ITEM_TITLE=...` only if you want a different
title.

Then reload and restart the service before testing from the same entrypoint the
agent uses:

```bash
systemctl --user daemon-reload
systemctl --user restart openclaw-gateway.service

systemctl --user show openclaw-gateway.service \
  --property=Environment

openclaw agent --agent main --message \
  'Run: gog auth doctor --check --no-input && gog gmail search "newer_than:1d" --max 1 --json'
```

If the shell command succeeds but the agent still reports `keyring.password`,
fix the agent or service environment first. Re-authenticating usually does not
help when `gog auth doctor --check` already shows readable tokens in the shell.

## Windows

Download the matching ZIP from the
[latest release](https://github.com/openclaw/gogcli/releases):

- `gogcli_<version>_windows_amd64.zip`
- `gogcli_<version>_windows_arm64.zip`

Extract `gog.exe` and put its directory on `PATH`.

## GitHub releases (raw binaries)

Release assets are uploaded by GoReleaser:

- `gogcli_<version>_darwin_amd64.tar.gz`
- `gogcli_<version>_darwin_arm64.tar.gz`
- `gogcli_<version>_linux_amd64.tar.gz`
- `gogcli_<version>_linux_arm64.tar.gz`
- `gogcli_<version>_windows_amd64.zip`
- `gogcli_<version>_windows_arm64.zip`
- `checksums.txt`

Browse the [releases page](https://github.com/openclaw/gogcli/releases) for
the latest tag and the full asset list.

## Build from source

```bash
git clone https://github.com/openclaw/gogcli.git
cd gogcli
make
./bin/gog --version
```

Source builds require the Go version declared in `go.mod`.

## Safety-profile binaries

When `gog` is going to be invoked by an agent, sandbox, or other caller that
should not be able to broaden its own permissions, build a safety-profile
binary instead of the default one. See [Safety Profiles](safety-profiles.md).

```bash
./build-safe.sh safety-profiles/agent-safe.yaml -o bin/gog-agent-safe
./build-safe.sh safety-profiles/readonly.yaml   -o bin/gog-readonly
```

## Verify the install

```bash
gog --version
gog auth keyring         # report current keyring backend
gog --help               # discover top-level commands
```

After running [`gog auth credentials`](commands/gog-auth-credentials.md) and
[`gog auth add`](commands/gog-auth-add.md), `gog auth doctor --check` reports
keyring health, refresh-token validity, and Workspace-specific failure modes.

## Updating

- **Homebrew:** `brew upgrade openclaw/tap/gogcli`.
- **Docker:** pull a new tag (`ghcr.io/openclaw/gogcli:vX.Y.Z`).
- **GitHub release archives:** download the new tarball/ZIP and replace the
  binary.
- **Source builds:** `git pull && make` — the version string comes from
  `git describe`.

Refresh tokens and OAuth clients are forward-compatible across point releases;
no migration step is required for normal upgrades.

## Related command pages

- [`gog version`](commands/gog-version.md)
- [`gog auth keyring`](commands/gog-auth-keyring.md)
- [`gog auth credentials`](commands/gog-auth-credentials.md)
- [`gog auth add`](commands/gog-auth-add.md)
- [`gog auth doctor`](commands/gog-auth-doctor.md)
