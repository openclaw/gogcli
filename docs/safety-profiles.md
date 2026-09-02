# Safety Profiles

Safety profiles build a dedicated `gog` binary with an embedded command policy.
Use them when `gog` is available to an agent, CI job, sandbox, or other caller
that should not be able to change its own command permissions at runtime.

Runtime guards such as `--enable-commands`, `--disable-commands`, and
`--gmail-no-send` are still useful for normal scripting. A baked safety profile is
stronger: the policy is compiled into the binary and cannot be changed with
flags, environment variables, config files, or shell arguments.

A profile controls which commands may run. Through `locked-flags` it can also fix
the value of individual flags, so settings such as sanitized output do not depend
on the caller passing them. See [Locked Flags](#locked-flags).

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

## Tamper Resistance

The generator emits the allow and deny rule sets as `switch` statements on the
FNV-64a hash of each dotted command path, not as raw YAML. The compiled rule
table never contains the rule strings themselves, so to re-enable a blocked
command an attacker has to patch compiled machine code rather than flip ASCII
bytes in a YAML blob; the cost goes from a one-line `sed` invocation to
disassembly-level work.

Note that command names may still appear in the binary from unrelated metadata
(API URLs, error message format strings, Kong help text). What this hardening
guarantees is that the rule set itself is no longer a contiguous, patchable
string. The profile name (e.g. `agent-safe`) is also embedded as a constant so
error messages can reference it.

## Preset Profiles

`safety-profiles/agent-safe.yaml`

Allows reading, searching, drafting, labeling, archiving, organizing files, and
other low-risk recoverable actions. Blocks sends, deletes, sharing changes, admin
operations, and auth writes. Keeps `schema` available for command, exit-code,
and effective safety-policy discovery.

Good for:

- inbox triage agents
- draft reply generation
- summarization/reporting jobs that may organize labels or files
- workflows where a human should review before anything is sent

`safety-profiles/readonly.yaml`

Allows read/list/search/get style commands only. Blocks mutations, sends, deletes,
sharing changes, auth writes, and local config writes. Keeps `schema` available
for command, exit-code, and effective safety-policy discovery.

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
- `locked-flags:` is not a command path; it locks flag values, see [Locked Flags](#locked-flags).

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

## Locked Flags

Command rules decide what may run. They do not decide how it runs. Sanitized
output, for example, happens only when the caller passes `--sanitize-content`,
and a flag that does take its value from the environment is still overridden by
the command line. When the command line is written by a model rather than a
person, neither is a setting you can rely on.

`locked-flags` fixes a flag's value in the profile:

```yaml
locked-flags:
  sanitize-content: true
  wrap-untrusted: true
  no-input: true
```

Locked values and the flags they target must be boolean. This intentionally keeps
the mechanism focused on safety policy rather than compiling account names, paths,
tokens, or other arbitrary configuration into a binary.

A locked flag behaves as follows:

- the value is applied before the command runs, so the caller need not pass it.
- setting that flag to a different value is an error, not an override.
- a lock matches the canonical flag name, so aliases are covered and it takes effect
  wherever the selected command has a flag of that name.
- a locked output mode wins over the competing one. With `json` locked, `GOG_PLAIN`
  gives way silently, since an environment default is a preference rather than a
  request, while an explicit `--plain` is refused for asking output the profile
  forbids.

A lock the binary cannot enforce is refused at build time rather than silently
ignored.

Lock presence, output-mode precedence, and diagnostic notes belong to each parsed
invocation; they are not shared between command executions.

Because a locked value reaches the command without appearing on the command line, a
usage error that names a locked flag carries a note saying where the value came from:

```text
--sanitize-content cannot be used with --format raw
note: --no-input, --sanitize-content, --wrap-untrusted locked by baked safety profile "agent-safe-locked"
```

Lock deliberately. A flag that participates in a conflict or a requirement removes
the command paths that contradict it. Locking `sanitize-content` makes
`gmail get --format raw` unavailable, which is an acceptable trade for an agent
profile because raw is the unsanitized dump.

Locking a boolean flag that requires another one is the case to avoid. `--reply-all` needs
a message or thread to reply to, so `locked-flags: {reply-all: true}` breaks an
ordinary draft:

```bash
bin/gog-my-agent gmail drafts create --to you@example.com --subject Hi --body Hi
```

The command is allowed by the profile and the caller passed nothing wrong, yet it
fails because the locked flag demands an argument they had no reason to supply:

```text
--reply-all requires --reply-to-message-id or --thread-id
```

Flags that only shape output are the safe candidates.

To make an existing preset stricter without changing its behavior for everyone,
copy it and add the locks you need. For example, an agent-oriented copy commonly
locks `sanitize-content`, `wrap-untrusted`, and `no-input` to `true`.

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
