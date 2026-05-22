# Paths and State

`gog` separates configuration, durable data, mutable state, and regenerable
cache paths. This keeps OAuth material and service-account keys out of config
directories that are commonly synced as dotfiles, while still preserving legacy
reads for existing installs.

## Resolution Order

For config, data, state, and cache, the first configured path wins:

1. `GOG_CONFIG_DIR`, `GOG_DATA_DIR`, `GOG_STATE_DIR`, or `GOG_CACHE_DIR`
2. `--home <path>` for one invocation
3. `GOG_HOME`
4. The matching XDG variable: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
   `XDG_STATE_HOME`, or `XDG_CACHE_HOME`
5. The platform default

`GOG_*_DIR`, `GOG_HOME`, and `--home` must resolve to absolute paths. Relative
XDG values are ignored, matching the XDG Base Directory specification.

When `GOG_HOME=/persist/gogcli`, `gog` uses a flat kind layout:

```text
/persist/gogcli/config
/persist/gogcli/data
/persist/gogcli/state
/persist/gogcli/cache
```

Per-kind overrides are literal directories. For example,
`GOG_DATA_DIR=/secure/gog-data` stores data files directly below
`/secure/gog-data`, without appending another `gogcli` component.

## What Goes Where

- Config: `config.json`, config locks, and backup configuration.
- Data: OAuth client metadata, file-keyring entries, and service-account keys.
- State: Gmail watch cursors and email tracking state.
- Cache: Gmail backup intermediate cache.
- Downloads: unchanged by the XDG/GOG split. Drive downloads and Gmail
  attachments keep their existing default directory unless the command's
  `--out` flag points elsewhere.

## Compatibility

Existing installs are read-compatible:

- OAuth credentials are read from the old config directory when the new data
  path does not contain them.
- File-keyring users keep using the old keyring directory when it exists and no
  explicit data override is set.
- Service-account keys are looked up in both the new data path and the old
  config path.
- Tracking and Gmail watch state read legacy locations when new state files are
  absent.

New writes use the new path for the selected kind. `auth credentials remove`
and service-account removal clean up both new and legacy locations where safe.

## Containers and Agents

For a single persistent root:

```bash
docker run --rm -it \
  -e GOG_HOME=/persist/gogcli \
  -e GOG_KEYRING_BACKEND=file \
  -e GOG_KEYRING_PASSWORD \
  -v gogcli-state:/persist/gogcli \
  ghcr.io/openclaw/gogcli:latest \
  auth add you@gmail.com --services gmail,calendar,drive
```

For split lifetimes:

```bash
export GOG_CONFIG_DIR=/etc/gogcli
export GOG_DATA_DIR=/var/lib/gogcli
export GOG_STATE_DIR=/var/lib/gogcli-state
export GOG_CACHE_DIR=/var/cache/gogcli
```
