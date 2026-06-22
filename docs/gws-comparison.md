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
quality. Timing is a local observation affected by caches and network state.
Do not rank tools by a single aggregate score. Inspect per-scenario output and
add task-specific, non-mutating scenarios when evaluating a real workflow.

Sources: [Google Workspace CLI repository](https://github.com/googleworkspace/cli),
[npm package](https://www.npmjs.com/package/@googleworkspace/cli), and gog's
[automation contract](automation.md).
