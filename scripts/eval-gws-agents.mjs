#!/usr/bin/env node

import { spawn } from "node:child_process";
import { constants as fsConstants } from "node:fs";
import { access, chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

export function parseArgs(argv, environment = process.env) {
  const options = {
    gog: path.join(root, "bin", "gog"),
    gws: "gws",
    account: environment.GOG_EVAL_ACCOUNT ?? "",
    driveName: environment.GOG_EVAL_DRIVE_NAME ?? "",
    codexModel: environment.GOG_EVAL_CODEX_MODEL ?? "",
    openclawModel: environment.GOG_EVAL_OPENCLAW_MODEL ?? "",
    agents: ["codex", "openclaw"],
    repetitions: 2,
    timeoutMs: 300_000,
    out: "",
  };
  for (let index = 0; index < argv.length; index += 1) {
    const key = argv[index];
    const value = argv[index + 1];
    switch (key) {
      case "--gog": options.gog = value; index += 1; break;
      case "--gws": options.gws = value; index += 1; break;
      case "--account": options.account = value; index += 1; break;
      case "--drive-name": options.driveName = value; index += 1; break;
      case "--codex-model": options.codexModel = value; index += 1; break;
      case "--openclaw-model": options.openclawModel = value; index += 1; break;
      case "--agents": options.agents = value.split(",").filter(Boolean); index += 1; break;
      case "--repetitions": options.repetitions = Number(value); index += 1; break;
      case "--timeout-ms": options.timeoutMs = Number(value); index += 1; break;
      case "--out": options.out = value; index += 1; break;
      default: throw new Error(`unknown argument: ${key}`);
    }
  }
  if (!options.account) throw new Error("--account or GOG_EVAL_ACCOUNT is required");
  if (!Number.isInteger(options.repetitions) || options.repetitions <= 0) {
    throw new Error("--repetitions must be a positive integer");
  }
  if (!Number.isFinite(options.timeoutMs) || options.timeoutMs <= 0) {
    throw new Error("--timeout-ms must be positive");
  }
  const supported = new Set(["codex", "openclaw"]);
  if (options.agents.length === 0 || options.agents.some((agent) => !supported.has(agent))) {
    throw new Error("--agents must contain codex and/or openclaw");
  }
  return options;
}

function runProcess(command, args, { cwd, env, timeoutMs }) {
  return new Promise((resolve) => {
    const started = process.hrtime.bigint();
    const child = spawn(command, args, { cwd, env, stdio: ["ignore", "pipe", "pipe"] });
    const stdout = [];
    const stderr = [];
    child.stdout.on("data", (chunk) => stdout.push(chunk));
    child.stderr.on("data", (chunk) => stderr.push(chunk));
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      child.kill("SIGTERM");
      setTimeout(() => child.kill("SIGKILL"), 2_000).unref();
    }, timeoutMs);
    child.on("error", (error) => {
      clearTimeout(timer);
      resolve({
        exitCode: -1,
        elapsedMs: Number(process.hrtime.bigint() - started) / 1e6,
        stdout: Buffer.concat(stdout).toString("utf8"),
        stderr: Buffer.concat(stderr).toString("utf8"),
        timedOut,
        error: error.message,
      });
    });
    child.on("close", (exitCode, signal) => {
      clearTimeout(timer);
      resolve({
        exitCode: exitCode ?? -1,
        signal,
        elapsedMs: Number(process.hrtime.bigint() - started) / 1e6,
        stdout: Buffer.concat(stdout).toString("utf8"),
        stderr: Buffer.concat(stderr).toString("utf8"),
        timedOut,
      });
    });
  });
}

async function runJson(command, args, environment, timeoutMs) {
  const result = await runProcess(command, args, {
    cwd: root,
    env: environment,
    timeoutMs,
  });
  if (result.exitCode !== 0) {
    throw new Error(`${path.basename(command)} fixture query failed with exit ${result.exitCode}`);
  }
  try {
    return JSON.parse(result.stdout);
  } catch {
    throw new Error(`${path.basename(command)} fixture query returned invalid JSON`);
  }
}

function sortedUnique(values) {
  return [...new Set(values)].sort();
}

function shellQuote(value) {
  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

async function resolveExecutable(command, searchPath) {
  if (command.includes(path.sep)) return path.resolve(command);
  for (const directory of (searchPath ?? "").split(path.delimiter).filter(Boolean)) {
    const candidate = path.join(directory, command);
    try {
      await access(candidate, fsConstants.X_OK);
      return candidate;
    } catch {}
  }
  throw new Error(`executable not found on PATH: ${command}`);
}

function exactDriveQuery(query, driveName) {
  if (!driveName || typeof query !== "string") return false;
  const escaped = driveName.replaceAll("'", "\\'");
  return query.replaceAll(/\s+/g, " ").trim() === `name = '${escaped}' and trashed = false`;
}

function allowedGogFlags(args, allowed) {
  return args.every((arg) => allowed.has(arg));
}

function allowedGogDriveArgs(args, driveName) {
  if (!exactDriveQuery(args[2], driveName)) return false;
  if (!args.includes("--raw-query")) return false;
  if (!args.includes("--no-all-drives")) return false;
  for (let index = 3; index < args.length; index += 1) {
    const arg = args[index];
    if (["--json", "--results-only", "--no-input", "--no-all-drives", "--raw-query"].includes(arg)) continue;
    if (arg === "--max") {
      const value = Number(args[index + 1]);
      if (!Number.isInteger(value) || value < 1 || value > 1000) return false;
      index += 1;
      continue;
    }
    if (arg.startsWith("--max=")) {
      const value = Number(arg.slice("--max=".length));
      if (!Number.isInteger(value) || value < 1 || value > 1000) return false;
      continue;
    }
    return false;
  }
  return true;
}

function parseGwsParams(args, validate) {
  let params = {};
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === "--page-all") continue;
    if (arg === "--format") {
      if (args[index + 1] !== "json") return false;
      index += 1;
      continue;
    }
    if (arg === "--page-limit") {
      const value = Number(args[index + 1]);
      if (!Number.isInteger(value) || value < 1 || value > 100) return false;
      index += 1;
      continue;
    }
    if (arg !== "--params" || index + 1 >= args.length) return false;
    try {
      params = JSON.parse(args[index + 1]);
    } catch {
      return false;
    }
    index += 1;
  }
  return validate(params);
}

export function allowedAgentArgs(cli, args, driveName = "") {
  if (!Array.isArray(args) || args.some((arg) => typeof arg !== "string")) return false;
  if (args.length === 1 && (args[0] === "--help" || args[0] === "--version")) return true;
  if (args[0] === "schema") return true;
  if (["gmail", "calendar", "drive"].includes(args[0]) && args.includes("--help")) return true;
  if (cli === "gog") {
    if (args.slice(0, 3).join(" ") === "gmail labels list") {
      return allowedGogFlags(args.slice(3), new Set(["--json", "--results-only", "--no-input"]));
    }
    if (args.slice(0, 2).join(" ") === "calendar calendars") {
      return allowedGogFlags(args.slice(2), new Set(["--json", "--results-only", "--no-input", "--all"]));
    }
    if (args.slice(0, 2).join(" ") === "drive search") return allowedGogDriveArgs(args, driveName);
    return false;
  }
  if (args.slice(0, 4).join(" ") === "gmail users labels list") {
    return parseGwsParams(args.slice(4), (params) =>
      Object.keys(params).every((key) => key === "userId") && params.userId === "me");
  }
  if (args.slice(0, 3).join(" ") === "calendar calendarList list") {
    return parseGwsParams(args.slice(3), (params) =>
      Object.keys(params).every((key) => ["maxResults", "showDeleted", "showHidden"].includes(key))
      && (params.maxResults == null || (Number.isInteger(params.maxResults) && params.maxResults >= 1 && params.maxResults <= 250))
      && (params.showDeleted == null || params.showDeleted === false)
      && (params.showHidden == null || typeof params.showHidden === "boolean"));
  }
  if (args.slice(0, 3).join(" ") === "drive files list") {
    const allowedFields = new Set([
      "files(id,name)", "files(id,name,trashed)",
      "nextPageToken,files(id,name)", "nextPageToken,files(id,name,trashed)",
      "nextPageToken,incompleteSearch,files(id,name,trashed)",
    ]);
    return parseGwsParams(args.slice(3), (params) =>
      Object.keys(params).every((key) => ["q", "pageSize", "fields", "spaces", "corpora", "supportsAllDrives"].includes(key))
      && exactDriveQuery(params.q, driveName)
      && (params.pageSize == null || (Number.isInteger(params.pageSize) && params.pageSize >= 1 && params.pageSize <= 1000))
      && (params.fields == null || allowedFields.has(params.fields.replaceAll(/\s+/g, "")))
      && (params.spaces == null || params.spaces === "drive")
      && (params.corpora == null || params.corpora === "user")
      && (params.supportsAllDrives == null || params.supportsAllDrives === true));
  }
  return false;
}

async function startCliProxy(cli, options, environment) {
  const executable = await resolveExecutable(cli === "gog" ? options.gog : options.gws, environment.PATH);
  const socketDirectory = await mkdtemp(path.join(os.tmpdir(), "gog-agent-proxy-"));
  await chmod(socketDirectory, 0o700);
  const socketPath = path.join(socketDirectory, "cli.sock");
  const sockets = new Set();
  const server = net.createServer((socket) => {
    sockets.add(socket);
    socket.once("close", () => sockets.delete(socket));
    let request = "";
    socket.setEncoding("utf8");
    socket.on("data", async (chunk) => {
      request += chunk;
      if (!request.includes("\n")) return;
      socket.pause();
      try {
        const args = JSON.parse(request.slice(0, request.indexOf("\n")));
        if (!allowedAgentArgs(cli, args, options.driveName)) {
          socket.end(`${JSON.stringify({ exitCode: 2, stdout: "", stderr: "command blocked by eval read-only policy\n" })}\n`);
          return;
        }
        const commandArgs = cli === "gog"
          ? ["--account", options.account, "--no-input", ...args]
          : args;
        const result = await runProcess(executable, commandArgs, {
          cwd: root,
          env: cli === "gog" ? { ...environment, GOG_HELP: "agent" } : environment,
          timeoutMs: options.timeoutMs,
        });
        socket.end(`${JSON.stringify({
          exitCode: result.exitCode,
          stdout: result.stdout,
          stderr: result.stderr,
        })}\n`);
      } catch {
        socket.end(`${JSON.stringify({ exitCode: 2, stdout: "", stderr: "invalid eval proxy request\n" })}\n`);
      }
    });
  });
  try {
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(socketPath, resolve);
    });
    await chmod(socketPath, 0o600);
  } catch (error) {
    await rm(socketDirectory, { recursive: true, force: true });
    throw error;
  }
  return {
    socketPath,
    close: async () => {
      for (const socket of sockets) socket.destroy();
      await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
      await rm(socketDirectory, { recursive: true, force: true });
    },
  };
}

export function fixturesAgree(gog, gws) {
  return JSON.stringify(gog) === JSON.stringify(gws);
}

async function loadFixtures(options, environment) {
  const gogBase = ["--account", options.account, "--no-input"];
  const gogLabels = await runJson(options.gog, [
    ...gogBase, "gmail", "labels", "list", "--json", "--results-only",
  ], environment, options.timeoutMs);
  const gwsLabels = await runJson(options.gws, [
    "gmail", "users", "labels", "list", "--params", JSON.stringify({ userId: "me" }),
    "--format", "json",
  ], environment, options.timeoutMs);
  const normalizeLabel = (labels) => (labels ?? [])
    .filter((label) => label.id === "INBOX")
    .map(({ id, name, type }) => ({ id, name, type }));

  const gogCalendars = await runJson(options.gog, [
    ...gogBase, "calendar", "calendars", "--json", "--results-only",
  ], environment, options.timeoutMs);
  const gwsCalendars = await runJson(options.gws, [
    "calendar", "calendarList", "list", "--params", JSON.stringify({ maxResults: 250 }),
    "--format", "json",
  ], environment, options.timeoutMs);
  const normalizeCalendar = (calendars) => (calendars ?? [])
    .filter((calendar) => calendar.primary === true)
    .map(({ id, timeZone }) => ({ id, timeZone }));

  const fixtures = {
    gmail: normalizeLabel(gogLabels),
    calendar: normalizeCalendar(gogCalendars),
  };
  const peerFixtures = {
    gmail: normalizeLabel(gwsLabels.labels),
    calendar: normalizeCalendar(gwsCalendars.items),
  };

  if (options.driveName) {
    const exactQuery = `name = '${options.driveName.replaceAll("'", "\\'")}' and trashed = false`;
    const gogDrive = await runJson(options.gog, [
      ...gogBase, "drive", "search", exactQuery, "--raw-query", "--no-all-drives", "--json", "--results-only", "--max", "100",
    ], environment, options.timeoutMs);
    const escaped = options.driveName.replaceAll("'", "\\'");
    const gwsDrive = await runJson(options.gws, [
      "drive", "files", "list", "--params", JSON.stringify({
        q: `name = '${escaped}' and trashed = false`,
        pageSize: 100,
        fields: "files(id,name)",
      }),
      "--format", "json",
    ], environment, options.timeoutMs);
    fixtures.drive = {
      name: options.driveName,
      ids: sortedUnique(gogDrive.filter((file) => file.name === options.driveName).map((file) => file.id)),
    };
    peerFixtures.drive = {
      name: options.driveName,
      ids: sortedUnique((gwsDrive.files ?? []).filter((file) => file.name === options.driveName).map((file) => file.id)),
    };
  }

  if (!fixturesAgree(fixtures, peerFixtures)) {
    throw new Error("gog and gws fixture queries disagree; refusing to score agent output");
  }
  if (fixtures.gmail.length !== 1 || fixtures.calendar.length !== 1) {
    throw new Error("test account is missing a unique INBOX label or primary calendar");
  }
  if (fixtures.drive && fixtures.drive.ids.length === 0) {
    throw new Error("the requested Drive fixture does not exist; refusing to score an empty task");
  }
  return fixtures;
}

export function buildPrompt(driveName = "", executable = "workspace-cli") {
  const driveTask = driveName
    ? `\n3. Search the default Drive corpus for every non-trashed file whose name is exactly ${JSON.stringify(driveName)}. Do not broaden to shared-drive or all-drive corpora. Return sorted unique file IDs.`
    : "";
  const driveShape = driveName ? ',"drive":{"name":"string","ids":["string"]}' : "";
  return `Use only the shell executable ${JSON.stringify(executable)} to inspect the authenticated Google Workspace account. Do not use a browser, web search, MCP connector, another Google CLI, or prior knowledge. Inspect that executable's help or schema when needed.

Tasks:
1. Find the Gmail system INBOX label. Return its id, name, and type.
2. Find the primary calendar. Return its id and timeZone.${driveTask}

Return only one JSON object with this exact shape:
{"gmail":{"id":"string","name":"string","type":"string"},"calendar":{"id":"string","timeZone":"string"}${driveShape}}
Do not include prose or markdown.`;
}

export function extractJson(text) {
  const trimmed = text.trim();
  const unfenced = trimmed.replace(/^```(?:json)?\s*/i, "").replace(/\s*```$/, "");
  const start = unfenced.indexOf("{");
  const end = unfenced.lastIndexOf("}");
  if (start < 0 || end < start) throw new Error("agent did not return a JSON object");
  return JSON.parse(unfenced.slice(start, end + 1));
}

export function scoreAnswers(answers, fixtures) {
  const checks = [
    {
      name: "gmail.inbox",
      pass: JSON.stringify(answers?.gmail) === JSON.stringify(fixtures.gmail[0]),
    },
    {
      name: "calendar.primary",
      pass: JSON.stringify(answers?.calendar) === JSON.stringify(fixtures.calendar[0]),
    },
  ];
  if (fixtures.drive) {
    checks.push({
      name: "drive.exact_name",
      pass: answers?.drive?.name === fixtures.drive.name
        && JSON.stringify(sortedUnique(answers?.drive?.ids ?? [])) === JSON.stringify(fixtures.drive.ids),
    });
  }
  return { checks, passed: checks.filter((check) => check.pass).length, total: checks.length };
}

export function parseCodex(stdout) {
  const events = stdout.split("\n").filter(Boolean).map((line) => JSON.parse(line));
  const messages = events
    .filter((event) => event.type === "item.completed" && event.item?.type === "agent_message")
    .map((event) => event.item.text)
    .filter(Boolean);
  const usage = [...events].reverse().find((event) => event.type === "turn.completed")?.usage ?? {};
  const toolTypes = new Set([
    "command_execution", "local_shell_call", "mcp_tool_call", "web_search", "web_search_call",
  ]);
  const toolCalls = events.filter((event) => event.type === "item.completed" && toolTypes.has(event.item?.type)).length;
  const input = usage.input_tokens ?? 0;
  const cacheRead = usage.cached_input_tokens ?? 0;
  const output = usage.output_tokens ?? 0;
  return {
    text: messages.at(-1) ?? "",
    metrics: {
      input_tokens: input,
      cache_read_tokens: cacheRead,
      output_tokens: output,
      reasoning_tokens: usage.reasoning_output_tokens ?? 0,
      total_tokens: input + output,
      uncached_tokens: Math.max(0, input - cacheRead) + output,
      tool_calls: toolCalls,
    },
  };
}

async function countOpenClawTools(sessionId) {
  const sessionPath = path.join(os.homedir(), ".openclaw", "agents", "main", "sessions", `${sessionId}.jsonl`);
  try {
    const lines = (await readFile(sessionPath, "utf8")).split("\n").filter(Boolean);
    let count = 0;
    for (const line of lines) {
      const event = JSON.parse(line);
      for (const content of event.message?.content ?? []) {
        if (content.type === "toolCall") count += 1;
      }
    }
    return count;
  } catch {
    return null;
  }
}

async function parseOpenClaw(stdout, sessionId) {
  const result = JSON.parse(stdout);
  const meta = result.result?.meta ?? {};
  const usage = meta.agentMeta?.usage ?? {};
  return {
    text: meta.finalAssistantVisibleText
      ?? result.result?.payloads?.map((payload) => payload.text).filter(Boolean).at(-1)
      ?? "",
    model: meta.agentMeta?.model ?? null,
    provider: meta.agentMeta?.provider ?? null,
    metrics: {
      input_tokens: usage.input ?? 0,
      cache_read_tokens: usage.cacheRead ?? 0,
      output_tokens: usage.output ?? 0,
      reasoning_tokens: 0,
      total_tokens: usage.total ?? ((usage.input ?? 0) + (usage.cacheRead ?? 0) + (usage.output ?? 0)),
      uncached_tokens: (usage.input ?? 0) + (usage.output ?? 0),
      tool_calls: await countOpenClawTools(sessionId),
    },
  };
}

async function createRunDirectory(options, socketPath) {
  const directory = await mkdtemp(path.join(os.tmpdir(), "workspace-agent-"));
  const binDirectory = path.join(directory, "bin");
  await mkdir(binDirectory);
  const wrapper = path.join(binDirectory, "workspace-cli");
  const client = path.join(binDirectory, "workspace-cli-client.mjs");
  const script = `#!/bin/sh\nexec ${shellQuote(process.execPath)} "$(dirname "$0")/workspace-cli-client.mjs" "$@"\n`;
  await writeFile(client, `import net from "node:net";
const socket = net.createConnection(${JSON.stringify(socketPath)});
let response = "";
socket.setEncoding("utf8");
socket.on("connect", () => socket.write(JSON.stringify(process.argv.slice(2)) + "\\n"));
socket.on("data", (chunk) => { response += chunk; });
socket.on("end", () => {
  try {
    const result = JSON.parse(response.trim());
    process.stdout.write(result.stdout ?? "");
    process.stderr.write(result.stderr ?? "");
    process.exitCode = result.exitCode ?? 1;
  } catch {
    process.stderr.write("invalid eval proxy response\\n");
    process.exitCode = 2;
  }
});
socket.on("error", () => {
  process.stderr.write("eval proxy unavailable\\n");
  process.exitCode = 2;
});
`);
  await writeFile(wrapper, script);
  await chmod(wrapper, 0o755);
  const schema = path.join(directory, "answer-schema.json");
  await writeFile(schema, `${JSON.stringify({
    type: "object",
    additionalProperties: false,
    required: ["gmail", "calendar", ...(options.driveName ? ["drive"] : [])],
    properties: {
      gmail: {
        type: "object",
        additionalProperties: false,
        required: ["id", "name", "type"],
        properties: { id: { type: "string" }, name: { type: "string" }, type: { type: "string" } },
      },
      calendar: {
        type: "object",
        additionalProperties: false,
        required: ["id", "timeZone"],
        properties: { id: { type: "string" }, timeZone: { type: "string" } },
      },
      ...(options.driveName ? {
        drive: {
          type: "object",
          additionalProperties: false,
          required: ["name", "ids"],
          properties: { name: { type: "string" }, ids: { type: "array", items: { type: "string" } } },
        },
      } : {}),
    },
  }, null, 2)}\n`);
  return { directory, binDirectory, schema, wrapper };
}

export function agentEnvironment(environment, binDirectory) {
  const allowed = [
    "HOME", "USER", "LOGNAME", "SHELL", "LANG", "LC_ALL", "LC_CTYPE", "TERM", "COLORTERM",
    "TMPDIR", "TMP", "TEMP", "NO_COLOR", "CODEX_HOME", "OPENCLAW_HOME",
    "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
  ];
  const safe = {};
  for (const key of allowed) {
    if (environment[key] != null) safe[key] = environment[key];
  }
  safe.PATH = `${binDirectory}:${environment.PATH ?? "/usr/bin:/bin"}`;
  return safe;
}

async function runAgent(agent, cli, repetition, fixtures, options, environment) {
  const proxy = await startCliProxy(cli, options, environment);
  const runDirectory = await createRunDirectory(options, proxy.socketPath);
  const prompt = buildPrompt(options.driveName, runDirectory.wrapper);
  const env = agentEnvironment(environment, runDirectory.binDirectory);
  const sessionId = `gog-eval-${cli}-${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
  const command = agent === "codex" ? "codex" : "openclaw";
  const args = agent === "codex"
    ? [
      "exec", "--ephemeral", "--skip-git-repo-check", "--json", "--sandbox", "danger-full-access",
      "-c", 'model_reasoning_effort="low"', "-C", runDirectory.directory,
      ...(options.codexModel ? ["--model", options.codexModel] : []),
      "--output-schema", runDirectory.schema, prompt,
    ]
    : [
      "agent", "--agent", "main", "--session-id", sessionId, "--message", prompt,
      ...(options.openclawModel ? ["--model", options.openclawModel] : []),
      "--thinking", "low", "--json", "--timeout", String(Math.ceil(options.timeoutMs / 1000)),
    ];
  let processResult;
  try {
    processResult = await runProcess(command, args, {
      cwd: runDirectory.directory,
      env,
      timeoutMs: options.timeoutMs,
    });
  } finally {
    await proxy.close();
  }
  await writeFile(path.join(runDirectory.directory, "agent.stdout.log"), processResult.stdout);
  await writeFile(path.join(runDirectory.directory, "agent.stderr.log"), processResult.stderr);
  process.stderr.write(`artifacts=${runDirectory.directory}\n`);
  const base = {
    agent,
    cli,
    repetition,
    exit_code: processResult.exitCode,
    elapsed_ms: Math.round(processResult.elapsedMs),
    timed_out: processResult.timedOut,
  };
  try {
    if (processResult.exitCode !== 0) throw new Error(`agent exited ${processResult.exitCode}`);
    const parsed = agent === "codex"
      ? parseCodex(processResult.stdout)
      : await parseOpenClaw(processResult.stdout, sessionId);
    const answers = extractJson(parsed.text);
    const score = scoreAnswers(answers, fixtures);
    return {
      ...base,
      model: parsed.model ?? (agent === "codex" ? options.codexModel || "default" : "unknown"),
      provider: parsed.provider ?? null,
      metrics: { ...parsed.metrics, elapsed_ms: base.elapsed_ms },
      checks: score.checks,
      passed: score.passed,
      total: score.total,
      success: score.passed === score.total,
    };
  } catch (error) {
    return {
      ...base,
      metrics: { elapsed_ms: base.elapsed_ms },
      checks: Object.keys(fixtures).map((name) => ({ name, pass: false })),
      passed: 0,
      total: Object.keys(fixtures).length,
      success: false,
      error: error.message,
    };
  }
}

function median(values) {
  const sorted = values.filter((value) => Number.isFinite(value)).sort((a, b) => a - b);
  if (sorted.length === 0) return null;
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 1 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
}

function aggregate(runs) {
  return {
    runs: runs.length,
    successes: runs.filter((run) => run.success).length,
    median_total_tokens: median(runs.map((run) => run.metrics.total_tokens)),
    median_uncached_tokens: median(runs.map((run) => run.metrics.uncached_tokens)),
    median_tool_calls: median(runs.map((run) => run.metrics.tool_calls)),
    median_elapsed_ms: median(runs.map((run) => run.metrics.elapsed_ms)),
  };
}

export function compareAggregates(gog, gws) {
  if (gog.successes === 0 && gws.successes === 0) {
    return { winner: "tie", criterion: "no_correct_runs" };
  }
  const criteria = [
    ["successes", "higher"],
    ["median_total_tokens", "lower"],
    ["median_tool_calls", "lower"],
    ["median_elapsed_ms", "lower"],
  ];
  for (const [field, direction] of criteria) {
    const left = gog[field];
    const right = gws[field];
    if (left == null || right == null || left === right) continue;
    const gogWins = direction === "higher" ? left > right : left < right;
    return { winner: gogWins ? "gog" : "gws", criterion: field };
  }
  return { winner: "tie", criterion: null };
}

export function summarize(runs, agents) {
  const byAgent = {};
  for (const agent of agents) {
    const gog = aggregate(runs.filter((run) => run.agent === agent && run.cli === "gog"));
    const gws = aggregate(runs.filter((run) => run.agent === agent && run.cli === "gws"));
    byAgent[agent] = { gog, gws, comparison: compareAggregates(gog, gws) };
  }
  const comparisons = Object.values(byAgent).map((entry) => entry.comparison.winner);
  return {
    by_agent: byAgent,
    gog_better: comparisons.every((winner) => winner === "gog" || winner === "tie")
      && comparisons.some((winner) => winner === "gog"),
  };
}

export async function runEvaluation(options, environment = process.env) {
  const fixtures = await loadFixtures(options, environment);
  const runs = [];
  for (let repetition = 1; repetition <= options.repetitions; repetition += 1) {
    for (const agent of options.agents) {
      const cliOrder = repetition % 2 === 1 ? ["gog", "gws"] : ["gws", "gog"];
      for (const cli of cliOrder) {
        process.stderr.write(`agent=${agent} cli=${cli} repetition=${repetition}\n`);
        runs.push(await runAgent(agent, cli, repetition, fixtures, options, environment));
      }
    }
  }
  return {
    schema_version: 1,
    generated_at: new Date().toISOString(),
    configuration: {
      agents: options.agents,
      repetitions: options.repetitions,
      tasks: Object.keys(fixtures),
      requested_models: {
        codex: options.codexModel || "default",
        openclaw: options.openclawModel || "default",
      },
    },
    runs,
    summary: summarize(runs, options.agents),
  };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const report = await runEvaluation(options);
  const output = `${JSON.stringify(report, null, 2)}\n`;
  if (options.out) await writeFile(options.out, output);
  process.stdout.write(output);
  if (!report.summary.gog_better) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await main();
}
