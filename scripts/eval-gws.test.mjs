import assert from "node:assert/strict";
import test from "node:test";

import { parseArgs, runCase } from "./eval-gws.mjs";

test("parseArgs validates timeout", () => {
  assert.throws(() => parseArgs(["--timeout-ms", "0"]), /positive/);
  assert.equal(parseArgs(["--gog", "/tmp/gog"]).gog, "/tmp/gog");
});

test("runCase records assertions and metrics", () => {
  // Use the evaluator's normal budget for this functional smoke.
  const result = runCase(process.execPath, {
    args: ["-e", "process.stdout.write(JSON.stringify({ok:true}))"],
    exit: [0],
    contains: ["ok"],
    json: true,
  }, parseArgs([]).timeoutMs, process.env);
  assert.equal(result.pass, true, JSON.stringify(result));
  assert.equal(result.exit_code, 0);
  assert.ok(result.stdout_bytes > 0);
  assert.ok(result.elapsed_ms >= 0);
});
