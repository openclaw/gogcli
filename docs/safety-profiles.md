# Safety Profiles

Safety profiles bake a command policy into a dedicated `gog` binary at build time.
They are for agent or sandbox use where runtime flags and config are too easy to change.

```bash
./build-safe.sh safety-profiles/agent-safe.yaml -o bin/gog-agent-safe
./build-safe.sh safety-profiles/readonly.yaml -o bin/gog-readonly
```

The generated binary still parses the normal CLI, but every command run is checked
against the baked profile before command execution. The profile cannot be changed
with environment variables, config files, or `--enable-commands`.

Profiles are fail-closed:

- commands set to `true` are allowed
- commands set to `false` are blocked and override broader allow rules
- commands not listed are blocked when the profile has any allow entries
- `aliases:` entries apply to root shortcuts like `send`, `ls`, and `upload`

Preset profiles:

- `safety-profiles/full.yaml` allows everything and is mostly a smoke-test profile
- `safety-profiles/readonly.yaml` allows read/list/search/get style commands only
- `safety-profiles/agent-safe.yaml` allows reading, drafting, organizing, and low-risk recoverable work while blocking sends, deletes, sharing, admin, and auth writes

Help and schema output remain available from the stock command tree. The security
boundary is execution: blocked commands fail before any Google API call or command
handler runs.
