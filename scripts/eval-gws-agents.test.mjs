import assert from "node:assert/strict";
import test from "node:test";

import {
  agentEnvironment,
  allowedAgentArgs,
  buildPrompt,
  compareAggregates,
  extractJson,
  fixturesAgree,
  parseCodex,
  parseArgs,
  scoreAnswers,
} from "./eval-gws-agents.mjs";

test("parseCodex counts legacy and current tool item types", () => {
  const events = [
    { type: "item.completed", item: { type: "command_execution" } },
    { type: "item.completed", item: { type: "local_shell_call" } },
    { type: "item.completed", item: { type: "web_search_call" } },
    { type: "item.completed", item: { type: "agent_message", text: '{"ok":true}' } },
    { type: "turn.completed", usage: { input_tokens: 10, cached_input_tokens: 4, output_tokens: 2 } },
  ];
  const parsed = parseCodex(events.map((event) => JSON.stringify(event)).join("\n"));
  assert.equal(parsed.metrics.tool_calls, 3);
  assert.equal(parsed.metrics.total_tokens, 12);
  assert.equal(parsed.metrics.uncached_tokens, 8);
});

test("allowedAgentArgs blocks credential and mutation commands", () => {
  assert.equal(allowedAgentArgs("gog", ["gmail", "labels", "list", "--json"]), true);
  assert.equal(allowedAgentArgs("gog", [
    "drive", "search", "name = 'fixture' and trashed = false", "--raw-query", "--no-all-drives", "--max", "100", "--json",
  ], "fixture"), true);
  assert.equal(allowedAgentArgs("gws", [
    "drive", "files", "list", "--params",
    '{"q":"name = \'fixture\' and trashed = false","fields":"files(id,name)","spaces":"drive","corpora":"user"}',
    "--format", "json", "--page-limit", "100",
  ], "fixture"), true);
  assert.equal(allowedAgentArgs("gog", ["auth", "tokens", "export"]), false);
  assert.equal(allowedAgentArgs("gws", ["auth", "export", "--unmasked"]), false);
  assert.equal(allowedAgentArgs("gog", ["gmail", "labels", "delete", "LABEL_1"]), false);
  assert.equal(allowedAgentArgs("gog", ["drive", "delete", "file-id"]), false);
  assert.equal(allowedAgentArgs("gog", ["drive", "search", "other file", "--json"], "fixture"), false);
  assert.equal(allowedAgentArgs("gog", ["drive", "search", "fixture", "--json"], "fixture"), false);
  assert.equal(allowedAgentArgs("gog", [
    "drive", "search", "name = 'fixture' and trashed = false", "--raw-query", "--json",
  ], "fixture"), false);
  assert.equal(allowedAgentArgs("gws", [
    "drive", "files", "list", "--params",
    '{"q":"trashed = false","fields":"files(id,name,owners)"}',
  ], "fixture"), false);
});

test("agentEnvironment excludes unrelated credentials", () => {
  const env = agentEnvironment({
    HOME: "/home/test",
    PATH: "/usr/bin",
    LANG: "C",
    GITHUB_TOKEN: "secret",
    GOG_KEYRING_PASSWORD: "secret",
    OPENAI_API_KEY: "secret",
  }, "/tmp/eval-bin");
  assert.deepEqual(env, {
    HOME: "/home/test",
    LANG: "C",
    PATH: "/tmp/eval-bin:/usr/bin",
  });
});

test("parseArgs requires an account and validates agents", () => {
  assert.throws(() => parseArgs([], {}), /account/);
  assert.throws(() => parseArgs(["--account", "test@example.com", "--agents", "other"], {}), /agents/);
  assert.equal(parseArgs(["--account", "test@example.com", "--repetitions", "2"], {}).repetitions, 2);
});

test("buildPrompt adds optional Drive task without revealing answers", () => {
  const prompt = buildPrompt("fixture doc");
  assert.match(prompt, /fixture doc/);
  assert.match(prompt, /workspace-cli/);
  assert.doesNotMatch(prompt, /clawdbot/);
});

test("extractJson accepts plain and fenced JSON", () => {
  assert.deepEqual(extractJson('{"ok":true}'), { ok: true });
  assert.deepEqual(extractJson('```json\n{"ok":true}\n```'), { ok: true });
});

test("scoreAnswers requires exact task results", () => {
  const fixtures = {
    gmail: [{ id: "INBOX", name: "INBOX", type: "system" }],
    calendar: [{ id: "primary@example.com", timeZone: "UTC" }],
    drive: { name: "fixture", ids: ["a", "b"] },
  };
  const score = scoreAnswers({
    gmail: fixtures.gmail[0],
    calendar: fixtures.calendar[0],
    drive: { name: "fixture", ids: ["b", "a", "a"] },
  }, fixtures);
  assert.equal(score.passed, 3);
  assert.equal(score.total, 3);
});

test("fixture agreement is order-sensitive after normalization", () => {
  assert.equal(fixturesAgree({ ids: ["a", "b"] }, { ids: ["a", "b"] }), true);
  assert.equal(fixturesAgree({ ids: ["a", "b"] }, { ids: ["b", "a"] }), false);
});

test("comparison prioritizes correctness over efficiency", () => {
  assert.deepEqual(compareAggregates(
    { successes: 0, median_total_tokens: null, median_tool_calls: null, median_elapsed_ms: 10 },
    { successes: 0, median_total_tokens: null, median_tool_calls: null, median_elapsed_ms: 20 },
  ), { winner: "tie", criterion: "no_correct_runs" });
  assert.deepEqual(compareAggregates(
    { successes: 2, median_total_tokens: 200, median_tool_calls: 4, median_elapsed_ms: 10 },
    { successes: 1, median_total_tokens: 10, median_tool_calls: 1, median_elapsed_ms: 1 },
  ), { winner: "gog", criterion: "successes" });
  assert.deepEqual(compareAggregates(
    { successes: 2, median_total_tokens: 100, median_tool_calls: 4, median_elapsed_ms: 10 },
    { successes: 2, median_total_tokens: 150, median_tool_calls: 1, median_elapsed_ms: 1 },
  ), { winner: "gog", criterion: "median_total_tokens" });
});
