---
title: gog and gws evaluation
description: "Reproduce a safety-focused comparison between gog and Google's Discovery-driven Workspace CLI."
---

# gog and gws evaluation

Google's `gws` (`@googleworkspace/cli`) and `gog` optimize for different jobs.
`gws` derives a broad, regular command surface from Google Discovery documents.
`gog` invests more heavily in curated workflows, multi-account operation,
stable automation contracts, human-readable output, and layered runtime safety.

This repository keeps the comparison executable instead of relying on a static
feature table that drifts.

## Reproduce

```bash
make build
npm install --prefix /tmp/gws-eval @googleworkspace/cli@0.22.5
GWS_BIN=/tmp/gws-eval/node_modules/.bin/gws make eval-gws
```

The default suite is credential-free and read-only. It compares root discovery,
Gmail command discovery, method schema output, invalid-command exit behavior,
latency, and output size. The harness removes access-token environment variables
before spawning either CLI.

Scenarios live in `evals/gws/scenarios.json`.
The JSON report includes every argv, assertion, exit code, duration, and output
size. Use a different binary or scenario file without editing the runner:

```bash
node scripts/eval-gws.mjs \
  --gog ./bin/gog \
  --gws /tmp/gws-eval/node_modules/.bin/gws \
  --scenarios evals/gws/scenarios.json \
  --out /tmp/gog-gws-eval.json
```

## Live agent evaluation

The live suite gives Codex and OpenClaw the same tasks through a neutral
`workspace-cli` executable. Each agent/CLI pair gets a fresh workspace and
session. The suite records task assertions, input/output/cache tokens, tool
calls, and elapsed time without retaining API responses, file IDs, or raw agent
transcripts in the report. Local-only traces remain in the printed temporary
artifact directories for diagnosis. CLI order alternates by repetition to
counterbalance provider prompt-cache warming. The gog wrapper enables
`GOG_HELP=agent`, the documented compact root-help mode intended for agent
discovery; API commands and output remain unchanged.

Authorize the same disposable account in both CLIs with read-only Gmail,
Calendar, and Drive scopes. The live runner excludes secret-bearing environment
variables from agent processes. A credential-free local proxy retains auth in
the parent process and accepts only the suite's help, schema, Gmail-label,
calendar-list, and Drive-search commands. Then run:

```bash
GOG_EVAL_ACCOUNT=test-account@example.com \
GOG_EVAL_DRIVE_NAME='exact fixture file name' \
GWS_BIN=/tmp/gws-eval/node_modules/.bin/gws \
make eval-gws-agents
```

The Drive task is omitted when `GOG_EVAL_DRIVE_NAME` is unset. Before invoking
an agent, the harness queries both CLIs directly and refuses to score the run if
their normalized fixtures disagree or the requested Drive fixture is empty.
Correctness is the first comparison
criterion; ties use total tokens, tool calls, then latency. Results remain
stratified by agent because Codex and OpenClaw have different fixed prompt and
cache overhead. Two repetitions are the default so each CLI runs first once.
Pin models with `GOG_EVAL_CODEX_MODEL` and `GOG_EVAL_OPENCLAW_MODEL` when a
long-lived comparison must not follow the agents' configured defaults.

## What each project teaches us

What gog should retain:

- first-class workflows instead of exposing only raw API methods;
- stable JSON/TSV, exit codes, dry-run plans, command guards, no-send policy,
  untrusted-content wrapping, and baked safety profiles;
- named OAuth clients, account aliases, service accounts, keyring choices, and
  backup/restore workflows.

What gog should learn from gws:

- Discovery-backed coverage closes the long tail quickly;
- a regular `service resource method` grammar is easy to predict;
- schema lookup is an effective agent primitive;
- generic pagination, upload, download, and output-format contracts reduce
  per-command learning.

The intended direction is additive: preserve gog's curated and safety-oriented
surface while using Discovery as an explicit escape hatch, not as a replacement
for high-quality first-class commands.

## Interpretation limits

The default suite measures structural behavior, not API correctness or product
quality. The live suite covers real API reads, but its fixture and timing results
still depend on the selected account, model, caches, and network state. Compare
gog and gws within each agent, repeat runs before drawing conclusions, and keep
task success ahead of efficiency metrics.

Sources: [Google Workspace CLI repository](https://github.com/googleworkspace/cli),
[npm package](https://www.npmjs.com/package/@googleworkspace/cli), and gog's
[automation contract](automation.md).
