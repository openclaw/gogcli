import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const makefile = readFileSync(new URL("../Makefile", import.meta.url), "utf8");

function runMake(directives, docker, args) {
  const directory = mkdtempSync(join(tmpdir(), "gog-docker-version-"));
  try {
    writeFileSync(join(directory, "Makefile"), makefile);
    writeFileSync(join(directory, "go.mod"), `module example.com/test\n\n${directives}\n`);
    writeFileSync(join(directory, "Dockerfile"), `ARG GO_VERSION=${docker}\n`);
    const result = spawnSync("make", ["--no-print-directory", ...args], {
      cwd: directory,
      encoding: "utf8",
    });
    assert.ifError(result.error);
    return result;
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

for (const fixture of [
  { name: "matches the minimum without a toolchain", directives: "go 1.26.0", docker: "1.26.0", passes: true },
  { name: "matches the preferred toolchain", directives: "go 1.26.0\ntoolchain go1.27.0", docker: "1.27.0", passes: true },
  { name: "rejects a stale Docker toolchain", directives: "go 1.26.0\ntoolchain go1.27.0", docker: "1.26.0", passes: false },
  { name: "rejects a mismatch without a toolchain", directives: "go 1.26.0", docker: "1.25.5", passes: false },
  { name: "uses the minimum for toolchain default", directives: "go 1.26.0\ntoolchain default", docker: "1.26.0", passes: true },
  { name: "requires a minimum Go version", directives: "toolchain go1.27.0", docker: "1.27.0", passes: false },
]) {
  test(`docker-version-check ${fixture.name}`, () => {
    const result = runMake(fixture.directives, fixture.docker, ["docker-version-check"]);
    assert.equal(result.status, fixture.passes ? 0 : 2, result.stdout + result.stderr);
    if (!fixture.passes) {
      assert.match(result.stderr, /must match go.mod build toolchain/);
    }
  });
}

test("lint targets the minimum Go version instead of the preferred toolchain", () => {
  const goMod = readFileSync(new URL("../go.mod", import.meta.url), "utf8");
  const lintConfig = readFileSync(new URL("../.golangci.yml", import.meta.url), "utf8");
  const minimum = goMod.match(/^go (\S+)$/m)?.[1];
  const lintVersion = lintConfig.match(/^run:\r?\n(?:  #.*\r?\n)*  go: "([^"]+)"$/m)?.[1];
  assert.ok(minimum, "go.mod must declare a minimum Go version");
  assert.equal(lintVersion, minimum);
});
