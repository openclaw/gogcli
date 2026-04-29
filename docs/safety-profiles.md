# Safety Profiles

Safety profiles build a dedicated `gog` binary with an embedded command policy.
Use them when `gog` is available to an agent, CI job, sandbox, or other caller
that should not be able to change its own command permissions at runtime.

Runtime guards such as `--enable-commands`, `--disable-commands`, and
`--gmail-no-send` are still useful for normal scripting. A baked safety profile is
stronger: the policy is compiled into the binary and cannot be changed with
flags, environment variables, config files, or shell arguments.

## Quick Start

Build an agent-safe binary:

```bash
./build-safe.sh safety-profiles/agent-safe.yaml -o bin/gog-agent-safe
```

Build a read-only binary:

```bash
./build-safe.sh safety-profiles/readonly.yaml -o bin/gog-readonly
```

Use the built binary exactly like `gog`:

```bash
bin/gog-agent-safe gmail search 'from:me newer_than:7d'
bin/gog-agent-safe gmail drafts create --to you@example.com --subject "Review" --body "Draft only"
bin/gog-agent-safe gmail drafts send draft-id
```

The final command fails before the Gmail send handler runs:

```text
command "gmail drafts send" is blocked by baked safety profile "agent-safe"
```

## How It Works

`build-safe.sh` performs a normal Go build with one extra generated file:

1. Validates the YAML profile.
2. Generates `internal/cmd/safety_profile_baked_gen.go` with the profile content.
3. Builds with `-tags safety_profile`.
4. Runs the built binary with `--version` as a smoke test.
5. Deletes the generated file on exit.

Normal `go build` does not include a profile, so the stock `gog` binary is
unchanged.

At runtime, `gog` parses the command with Kong first. After parsing and before
any command handler or Google API call, it checks the baked profile:

1. Explicit deny rules win.
2. Allow rules permit matching commands.
3. If the profile has allow rules, everything not allowed is blocked.

That means a caller cannot re-enable a blocked baked command:

```bash
bin/gog-readonly --enable-commands gmail.send gmail send \
  --to a@example.com --subject Test --body Test
```

The command still fails because the baked policy is checked before runtime
allowlists.

## Preset Profiles

`safety-profiles/agent-safe.yaml`

Allows reading, searching, drafting, labeling, archiving, organizing files, and
other low-risk recoverable actions. Blocks sends, deletes, sharing changes, admin
operations, and auth writes.

Good for:

- inbox triage agents
- draft reply generation
- summarization/reporting jobs that may organize labels or files
- workflows where a human should review before anything is sent

`safety-profiles/readonly.yaml`

Allows read/list/search/get style commands only. Blocks mutations, sends, deletes,
sharing changes, auth writes, and local config writes.

Good for:

- reporting
- audits
- monitoring
- read-only agent context gathering

`safety-profiles/full.yaml`

Allows everything. This is mostly useful for smoke testing the build path or for
creating a `-safe` binary with the same command surface as stock `gog`.

## Profile Syntax

Profiles are YAML maps that mirror command paths:

```yaml
name: agent-safe

gmail:
  search: true
  send: false
  drafts:
    create: true
    send: false

aliases:
  send: false
```

Rules:

- `true` allows a command path.
- `false` blocks a command path.
- blocked rules override allowed parent rules.
- unlisted commands are blocked when the profile has any allow rules.
- command names are written as dot paths internally, such as `gmail.drafts.create`.
- `aliases:` controls root shortcuts such as `send`, `ls`, `search`, and `upload`.

Parent rules are prefix matches. For example, `drive: true` allows every `drive`
subcommand unless a child is explicitly blocked. For restrictive profiles, prefer
listing leaf commands so a parent allow does not accidentally include future
mutating subcommands:

```yaml
gmail:
  messages:
    search: true
    modify: false
```

## Choosing A Profile

Use `readonly` when the caller should never change Google or local `gog` state.

Use `agent-safe` when the caller may prepare work but should not perform
externally visible or hard-to-reverse actions. For example, it may create a Gmail
draft but cannot send it.

Use a custom profile when the preset is too broad or too narrow:

```bash
cp safety-profiles/readonly.yaml /tmp/my-agent.yaml
editor /tmp/my-agent.yaml
./build-safe.sh /tmp/my-agent.yaml -o bin/gog-my-agent
```

## Verifying A Safe Binary

Build and smoke test:

```bash
./build-safe.sh safety-profiles/readonly.yaml -o gog-readonly
./gog-readonly version
```

Check blocked commands:

```bash
./gog-readonly gmail messages modify msg-1 --add Label_1
./gog-readonly calendar alias set work abc123@group.calendar.google.com
./gog-readonly --enable-commands gmail.send gmail send \
  --to a@example.com --subject Test --body Test
```

Each should fail with exit code 2 before any handler or Google API call runs.

Check allowed commands:

```bash
./gog-readonly gmail search 'newer_than:1d'
./gog-readonly auth services
```

## Help And Schema Output

Safety-profiled binaries filter help and schema output to the baked profile.
Blocked commands are not listed in parent help menus or `gog schema` output.

For example, `agent-safe` shows `gmail drafts create` but not `gmail drafts send`.
If you ask for help for a blocked leaf command directly, the binary prints the
same baked-profile block message instead of the command documentation.

## Security Boundary

Help and schema filtering are usability layers for humans and tool-discovering
agents. The security boundary remains the pre-execution profile check: blocked
commands fail before any command handler or Google API call runs.

Safety profiles also do not replace OAuth scopes, account separation, or Google
Workspace policy. Use the narrowest practical OAuth scopes and account access,
then use a baked profile as an additional local execution guard.
