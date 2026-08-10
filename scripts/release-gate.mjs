import { execFileSync, spawnSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const outputPath = process.env.PERSEUS_RELEASE_GATE_OUTPUT || 'test-results/release-gate.json';
const composeArgs = ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.e2e.yml'];
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm';
const liveProviderConfigured = Boolean(
  process.env.PERSEUS_LIVE_PROVIDER_BASE_URL
  && process.env.PERSEUS_LIVE_PROVIDER_API_KEY
  && process.env.PERSEUS_LIVE_PROVIDER_MODEL_ID,
);

const commandOutput = (program, args) => {
  try {
    return execFileSync(program, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  } catch (error) {
    return error.stderr?.toString().trim() || error.message;
  }
};

const hasExecutable = (program) => {
  const result = spawnSync(program, ['version'], { stdio: 'ignore' });
  return result.status === 0;
};

const run = async (name, program, args, options = {}) => {
  const startedAt = new Date().toISOString();
  const env = { ...process.env, ...(options.env || {}) };
  const windowsCommand = process.platform === 'win32' && program.toLowerCase().endsWith('.cmd');
  const spawnProgram = windowsCommand ? (process.env.ComSpec || 'cmd.exe') : program;
  const spawnArgs = windowsCommand ? ['/d', '/c', program, ...args] : args;
  const result = spawnSync(spawnProgram, spawnArgs, {
    cwd: options.cwd || process.cwd(),
    env,
    stdio: 'inherit',
  });
  const status = result.error ? 1 : (result.status ?? 1);
  const item = {
    name,
    status: status === 0 ? 'passed' : 'failed',
    exit_code: status,
    started_at: startedAt,
    completed_at: new Date().toISOString(),
  };
  if (result.error) item.error = result.error.message;
  if (status !== 0 && !options.continueOnFailure) throw Object.assign(new Error(`${name} failed with exit code ${status}`), { gateItem: item });
  return item;
};

const steps = [];
const runNpm = (name, script, options = {}) => run(
  name,
  npmCommand,
  ['run', script],
  { ...options, env: { PERSEUS_PLAYWRIGHT_JSON_OUTPUT: `test-results/playwright-${name}.json`, ...(options.env || {}) } },
);

const writeGateReport = async (status, error = null) => {
  const report = {
    schema_version: 1,
    generated_at: new Date().toISOString(),
    commit: commandOutput('git', ['rev-parse', 'HEAD']),
    git_clean: !commandOutput('git', ['status', '--porcelain']),
    status,
    steps,
    provider_live: liveProviderConfigured ? 'configured' : 'unverified',
    error,
  };
  await fs.mkdir(path.dirname(outputPath), { recursive: true });
  await fs.writeFile(outputPath, `${JSON.stringify(report, null, 2)}\n`);
  return report;
};

let stackStarted = false;
let fatalError = null;
try {
  const status = commandOutput('git', ['status', '--porcelain']);
  if (status) throw new Error('release gate requires a clean worktree before execution');

  execFileSync('docker', [...composeArgs, '--profile', 'e2e', 'up', '-d', '--build', '--wait'], { stdio: 'inherit' });
  stackStarted = true;

  if (hasExecutable('go')) {
    steps.push(await run('backend', 'go', ['test', './...'], { cwd: path.join(process.cwd(), 'backend') }));
  } else {
    const backendImage = process.env.PERSEUS_BACKEND_TEST_IMAGE || 'golang:1.25-bookworm';
    steps.push(await run('backend-docker', 'docker', [
      'run', '--rm', '-v', `${path.join(process.cwd(), 'backend')}:/src`, '-w', '/src',
      backendImage, 'go', 'test', './...',
    ]));
  }
  steps.push(await runNpm('frontend-unit', 'test:unit'));
  steps.push(await runNpm('frontend-ui', 'test:ui'));
  steps.push(await runNpm('frontend-structure', 'test:structure'));
  steps.push(await runNpm('frontend-build', 'build'));
  steps.push(await runNpm('e2e', 'test:e2e'));
  steps.push(await runNpm('faults', 'test:e2e:faults'));
  steps.push(await runNpm('provider-matrix', 'test:provider-matrix'));
  steps.push(await runNpm('load', 'test:load'));
  steps.push(await runNpm('soak', 'test:soak'));
  if (liveProviderConfigured) {
    steps.push(await runNpm('provider-live', 'test:provider-live'));
  } else {
    steps.push({ name: 'provider-live', status: 'unverified', reason: 'provider environment is not configured' });
  }
  await writeGateReport('running');
  steps.push(await runNpm('release-evidence', 'test:release-evidence'));
} catch (error) {
  fatalError = error;
  if (error.gateItem) steps.push(error.gateItem);
}

if (!fatalError) {
  await writeGateReport('passed');
  const finalEvidence = await run('release-evidence-final', npmCommand, ['run', 'test:release-evidence'], {
    env: { PERSEUS_PLAYWRIGHT_JSON_OUTPUT: 'test-results/playwright-release-evidence.json' },
    continueOnFailure: true,
  });
  steps.push(finalEvidence);
  if (finalEvidence.status !== 'passed') {
    fatalError = new Error('release evidence collection failed');
  }
}

if (stackStarted) {
  try {
    execFileSync('docker', [...composeArgs, '--profile', 'e2e', 'down', '-v'], { stdio: 'inherit' });
  } catch (error) {
    steps.push({ name: 'stack-cleanup', status: 'failed', error: error.message });
    fatalError ||= error;
  }
}

const report = await writeGateReport(fatalError ? 'failed' : 'passed', fatalError?.message || null);
process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
if (fatalError) process.exitCode = 1;
