import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { performance } from 'node:perf_hooks';

const source = resolve(process.argv[2]);
const variant = process.env.VARIANT;
const phase = process.env.PHASE;
const expected = process.env.SOURCE_SHA;
const worker = join(source, 'internal/tracking/worker');
const command = (executable, arguments_, cwd = source) => {
  const started = performance.now();
  const result = spawnSync(executable, arguments_, { cwd, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  if (result.error) throw result.error;
  return { status: result.status, ms: performance.now() - started, output: result.stdout + result.stderr };
};
const requireSuccess = (result, label) => {
  process.stdout.write(result.output);
  if (result.status !== 0) throw new Error(`${label} failed: ${result.status}`);
  return result;
};
const head = requireSuccess(command('git', ['rev-parse', 'HEAD']), 'source identity').output.trim();
if (head !== expected) throw new Error('Unexpected source revision');
const versions = {
  node: requireSuccess(command('node', ['--version']), 'Node version').output.trim(),
  pnpm: requireSuccess(command('pnpm', ['--version'], worker), 'pnpm version').output.trim(),
};
if (versions.node !== 'v26.12.1' || versions.pnpm !== '11.28.5') throw new Error('Unexpected toolchain');
const store = requireSuccess(command('pnpm', ['store', 'path'], worker), 'store identity').output.trim();
if (!store.startsWith(join(process.env.RUNNER_TEMP, 'proof-store'))) throw new Error('Store not isolated');
if (existsSync(join(worker, 'node_modules'))) throw new Error('Initial workspace is not fresh');
const tracked = requireSuccess(command('git', ['ls-files', 'internal/tracking/worker']), 'tracked source').output.trim().split('\n');
const digest = createHash('sha256');
for (const path of tracked) digest.update(path).update(readFileSync(join(source, path)));
const inputSha256 = digest.digest('hex');
const runPipeline = (directory, label) => {
  const steps = [];
  for (const arguments_ of [['install', '--frozen-lockfile'], ['lint'], ['typecheck'], ['build'], ['test']]) {
    const result = requireSuccess(command('pnpm', ['-C', directory, ...arguments_]), `${label} ${arguments_[0]}`);
    const clean = result.output.replace(/\u001b\[[0-9;]*m/g, '');
    const counts = [...clean.matchAll(/Tests\s+(\d+) passed/g)].map(match => Number(match[1]));
    if (arguments_[0] === 'test' && (counts.length !== 1 || counts[0] < 1 || /\d+ skipped/.test(clean))) throw new Error('Unexpected test result');
    if (arguments_[0] === 'build' && !/Total Upload:/.test(clean)) throw new Error('Actual package build output missing');
    steps.push({ command: arguments_.join(' '), ms: result.ms, tests: counts });
  }
  return { label, steps, tests: steps.at(-1).tests };
};
const initial = runPipeline(worker, `${variant}-${phase}`);
const result = { source: head, variant, phase, versions, inputSha256, initial, hot: [], controls: [] };
if (variant === 'candidate' && phase === 'fresh') {
  const candidateSource = resolve(process.env.HOT_CANDIDATE_SOURCE);
  const candidateHead = requireSuccess(command('git', ['rev-parse', 'HEAD'], candidateSource), 'hot source identity').output.trim();
  if (candidateHead !== expected) throw new Error('Unexpected hot source revision');
  const candidateDigest = createHash('sha256');
  for (const path of tracked) candidateDigest.update(path).update(readFileSync(join(candidateSource, path)));
  if (candidateDigest.digest('hex') !== inputSha256) throw new Error('Hot source differs');
  const hotRoots = { baseline: worker, candidate: join(candidateSource, 'internal/tracking/worker') };
  if (existsSync(join(hotRoots.candidate, 'node_modules'))) throw new Error('Hot candidate is not fresh');
  for (const [name, directory] of Object.entries(hotRoots)) {
    requireSuccess(command('pnpm', ['-C', directory, 'install', '--frozen-lockfile']), `${name} hot prime`);
  }
  for (let round = 0; round < 3; round += 1) {
    for (const name of round % 2 === 0 ? ['baseline', 'candidate'] : ['candidate', 'baseline']) {
      result.hot.push({ variant: name, round, ...runPipeline(hotRoots[name], `${name}-hot-${round}`) });
    }
  }
  for (const name of ['baseline', 'candidate']) {
    const directory = hotRoots[name];
    const badType = join(directory, 'src/proof-negative.ts');
    writeFileSync(badType, 'const proofInvalid: string = 42;\nexport { proofInvalid };\n');
    try {
      const rejected = command('pnpm', ['-C', directory, 'typecheck']);
      if (rejected.status === 0 || !/TS2322/.test(rejected.output)) throw new Error('Invalid TypeScript was not rejected');
      result.controls.push({ variant: name, kind: 'invalid TypeScript', status: rejected.status, expectedDiagnostic: 'TS2322' });
    } finally {
      unlinkSync(badType);
    }
    const badTest = join(directory, 'src/proof-negative.test.ts');
    writeFileSync(badTest, "import { expect, test } from 'vitest';\ntest('proof rejects invalid assertion', () => expect(true).toBe(false));\n");
    try {
      const rejected = command('pnpm', ['-C', directory, 'test']);
      if (rejected.status === 0 || !rejected.output.includes('proof rejects invalid assertion')) throw new Error('Failing test was not rejected');
      result.controls.push({ variant: name, kind: 'invalid assertion', status: rejected.status });
    } finally {
      unlinkSync(badTest);
    }
  }
}
console.log(`PROOF_JSON ${JSON.stringify(result)}`);
