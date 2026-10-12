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

## Changing the Keyring Password

`gog auth rekey <outdir>` reads the file keyring with the current password and
writes a verified copy encrypted with the new password to the separate output
directory given as its argument:

```bash
GOG_KEYRING_PASSWORD='current password' \
GOG_KEYRING_NEW_PASSWORD='new password' \
gog auth rekey /path/to/new-keyring
```

The live keyring is opened read-only. Its files, permissions and password remain
untouched on success, failure or interruption. Rekey does only the re-encryption;
it never installs the output. There are no swap markers, backups, rollback steps
or rekey write guards. Other commands can continue using the live keyring.

The output directory receives the source directory's permissions. It must be
outside the live directory and cannot be that directory or one of its ancestors,
including aliases through symlinks. Existing empty output is accepted. Nonempty
output is refused unless you explicitly pass `--force`, which replaces the whole
output directory and reports the names replaced; it never merges files. After an
interruption, run again with `--force` to rewrite the output from the live store.
If replacement fails, completed removals are reported and the error names any
entry whose removal failed; that entry may have been partially removed.

Before reporting success, rekey independently opens the output with the new
password and reads every item back, comparing its data and metadata. Output
contains only this run's entries: no password verifier or internal lock file is
created. An empty keyring produces empty output and explicitly reports that no
password was changed or established.

Inspect the output and any reported unreadable keys before installing it. The
command prints the exact move command for your paths (a shell command on Linux
and macOS, a PowerShell command on Windows). Stop running `gog` processes before
moving directories so their subsequent writes are not lost. For example:

```bash
test ! -e '/path/to/keyring.old' && test ! -L '/path/to/keyring.old' && mv -- '/path/to/keyring' '/path/to/keyring.old' && mv -- '/path/to/new-keyring' '/path/to/keyring'
```

Keep the old directory until the new keyring is proven. After installing the
output, update `GOG_KEYRING_PASSWORD` in the shell, services and CI that use it,
and test your normal commands. The command refuses to overwrite an existing
`.old` directory. These installation steps are the user's action, separate from
rekey.

Passwords come from the environment or a no-echo TTY prompt, never from argv.
The interactive new-password prompt asks twice and preserves whitespace.
Paths, counts and key names can appear in output; payloads and passwords cannot.

- Undecryptable entries abort before output is replaced. With
  `--skip-unreadable`, they are omitted from output and their keys are reported;
  their live copies remain untouched. A password that opens no entry is always
  an error, even with this flag. Inspect omissions before installing the output.
- Only the file backend qualifies. `auto` qualifies on headless Linux without a
  D-Bus session, where `gog` forces the file backend. Other `auto` configurations
  are refused without opening an OS keychain or creating a live directory; use
  `gog auth keyring file` if the file backend is your intended store.
- Legacy filenames are read without modification and written encoded in the
  output. Conflicting legacy and encoded copies of a key abort rather than
  silently discarding one.
- Rekey does not lock the live keyring. Concurrent changes are not an atomic
  snapshot; inspect the output and rerun after credential changes as needed.

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
