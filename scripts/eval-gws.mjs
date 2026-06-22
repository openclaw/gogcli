#!/usr/bin/env node

import { readFile, writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

export function parseArgs(argv) {
  const options = {
    gog: path.join(root, "bin", "gog"),
    gws: "gws",
    scenarios: path.join(root, "evals", "gws", "scenarios.json"),
    out: "",
    timeoutMs: 30_000,
  };
  for (let index = 0; index < argv.length; index += 1) {
    const key = argv[index];
    const value = argv[index + 1];
    switch (key) {
      case "--gog": options.gog = value; index += 1; break;
      case "--gws": options.gws = value; index += 1; break;
      case "--scenarios": options.scenarios = value; index += 1; break;
      case "--out": options.out = value; index += 1; break;
      case "--timeout-ms": options.timeoutMs = Number(value); index += 1; break;
      default: throw new Error(`unknown argument: ${key}`);
    }
  }
  if (!Number.isFinite(options.timeoutMs) || options.timeoutMs <= 0) {
    throw new Error("--timeout-ms must be positive");
  }
  return options;
}

export function runCase(binary, spec, timeoutMs, environment = process.env) {
  const started = process.hrtime.bigint();
  const result = spawnSync(binary, spec.args, {
    encoding: "utf8",
    timeout: timeoutMs,
    env: environment,
  });
  const elapsedMs = Number(process.hrtime.bigint() - started) / 1e6;
  const stdout = result.stdout ?? "";
  const stderr = result.stderr ?? "";
  const exitCode = result.status ?? (result.error ? -1 : 0);
  const checks = [];
  checks.push({name: "exit", pass: (spec.exit ?? [0]).includes(exitCode), actual: exitCode, expected: spec.exit ?? [0]});
  for (const expected of spec.contains ?? []) {
    checks.push({name: `contains:${expected}`, pass: `${stdout}\n${stderr}`.includes(expected)});
  }
  if (spec.json) {
    let valid = false;
    try { JSON.parse(stdout); valid = true; } catch {}
    checks.push({name: "json", pass: valid});
  }
  return {
    args: spec.args,
    exit_code: exitCode,
    elapsed_ms: Math.round(elapsedMs * 100) / 100,
    stdout_bytes: Buffer.byteLength(stdout),
    stderr_bytes: Buffer.byteLength(stderr),
    stdout_lines: stdout === "" ? 0 : stdout.trimEnd().split("\n").length,
    checks,
    pass: checks.every((check) => check.pass),
    error: result.error?.message,
  };
}

export async function runEvaluation(options) {
  const suite = JSON.parse(await readFile(options.scenarios, "utf8"));
  if (suite.schema_version !== 1 || !Array.isArray(suite.scenarios)) {
    throw new Error("invalid scenario file");
  }
  const environment = {...process.env};
  delete environment.GOG_ACCESS_TOKEN;
  delete environment.GOOGLE_WORKSPACE_CLI_TOKEN;
  const scenarios = suite.scenarios.map((scenario) => ({
    id: scenario.id,
    description: scenario.description,
    gog: runCase(options.gog, scenario.gog, options.timeoutMs, environment),
    gws: runCase(options.gws, scenario.gws, options.timeoutMs, environment),
  }));
  return {
    schema_version: 1,
    generated_at: new Date().toISOString(),
    host: {platform: process.platform, arch: process.arch, node: process.version},
    binaries: {gog: options.gog, gws: options.gws},
    scenarios,
    summary: {
      gog_passed: scenarios.filter((scenario) => scenario.gog.pass).length,
      gws_passed: scenarios.filter((scenario) => scenario.gws.pass).length,
      total: scenarios.length,
    },
  };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const report = await runEvaluation(options);
  const output = `${JSON.stringify(report, null, 2)}\n`;
  if (options.out) await writeFile(options.out, output);
  process.stdout.write(output);
  if (report.summary.gog_passed !== report.summary.total || report.summary.gws_passed !== report.summary.total) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await main();
}
