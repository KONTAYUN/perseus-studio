import { execFileSync, spawnSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { atomicWriteJSON, describeArtifact } from './release-evidence-lib.mjs';
import { runMaintenanceEvidence } from './maintenance-evidence-lib.mjs';

const outputPath = process.env.PERSEUS_RELEASE_GATE_OUTPUT || 'test-results/release-gate.json';
const evidencePath = process.env.PERSEUS_EVIDENCE_OUTPUT || 'test-results/release-evidence.json';
const artifactRoot = path.resolve(process.env.PERSEUS_RELEASE_ARTIFACTS || 'test-results/release-artifacts');
const scorecardPath = process.env.PERSEUS_SCORECARD_OUTPUT || 'test-results/PROJECT_HEALTH_SCORECARD.generated.md';
const scorecardManifestPath = process.env.PERSEUS_SCORECARD_MANIFEST || `${scorecardPath}.manifest.json`;
const composeArgs = ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.e2e.yml'];
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm';
const liveProviderConfigured = Boolean(
  process.env.PERSEUS_LIVE_PROVIDER_BASE_URL
  && process.env.PERSEUS_LIVE_PROVIDER_API_KEY
  && process.env.PERSEUS_LIVE_PROVIDER_MODEL_ID,
);
let providerLiveStatus = liveProviderConfigured ? 'configured' : 'unverified';

function commandOutput(program, args) {
  return execFileSync(program, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

const hasExecutable = (program) => spawnSync(program, ['version'], { stdio: 'ignore' }).status === 0;

async function run(name, program, args, options = {}) {
  const startedAt = new Date().toISOString();
  const env = { ...process.env, ...(options.env || {}) };
  const windowsCommand = process.platform === 'win32' && program.toLowerCase().endsWith('.cmd');
  const spawnProgram = windowsCommand ? (process.env.ComSpec || 'cmd.exe') : program;
  const spawnArgs = windowsCommand ? ['/d', '/c', program, ...args] : args;
  const result = spawnSync(spawnProgram, spawnArgs, {
    cwd: options.cwd || process.cwd(), env, stdio: 'inherit',
  });
  const status = result.error ? 1 : (result.status ?? 1);
  const item = {
    name, status: status === 0 ? 'passed' : 'failed', exit_code: status,
    started_at: startedAt, completed_at: new Date().toISOString(), artifacts: [],
  };
  if (result.error) item.error = result.error.message;
  if (status === 0) {
    try {
      item.artifacts = await Promise.all((options.artifacts || []).map(({ role, file }) => describeArtifact(role, file)));
    } catch (error) {
      item.status = 'failed';
      item.exit_code = 1;
      item.error = error.message;
    }
  }
  if (item.status !== 'passed' && !options.continueOnFailure) {
    throw Object.assign(new Error(`${name} failed`), { gateItem: item });
  }
  return item;
}

function playwrightPaths(name) {
  const root = path.join(artifactRoot, name);
  return {
    json: path.join(root, 'results.json'),
    output: path.join(root, 'raw'),
    html: path.join(root, 'html'),
  };
}

function runNpm(name, script, options = {}) {
  const playwright = options.playwright ? playwrightPaths(name) : null;
  return run(name, npmCommand, ['run', script], {
    ...options,
    env: {
      ...(playwright ? {
        PERSEUS_PLAYWRIGHT_JSON_OUTPUT: playwright.json,
        PERSEUS_PLAYWRIGHT_OUTPUT_DIR: playwright.output,
        PERSEUS_PLAYWRIGHT_HTML_OUTPUT: playwright.html,
      } : {}),
      ...(options.env || {}),
    },
    artifacts: [
      ...(playwright ? [
        { role: options.artifactRole, file: playwright.json },
        { role: `${options.artifactRole}_html`, file: path.join(playwright.html, 'index.html') },
      ] : []),
      ...(options.artifacts || []),
    ],
  });
}

const steps = [];
const commit = commandOutput('git', ['rev-parse', 'HEAD']);
const writeGateReport = async (status, error = null) => {
  const report = {
    schema_version: 2,
    generated_at: new Date().toISOString(),
    commit,
    git_clean: commandOutput('git', ['status', '--porcelain']) === '',
    status,
    artifact_root: path.relative(process.cwd(), artifactRoot).replaceAll(path.sep, '/'),
    steps,
    provider_live: providerLiveStatus,
    error,
  };
  await atomicWriteJSON(outputPath, report);
  return report;
};

let stackStarted = false;
let fatalError = null;
try {
  if (commandOutput('git', ['status', '--porcelain'])) throw new Error('release gate requires a clean worktree before execution');
  await fs.rm(artifactRoot, { recursive: true, force: true });
  await fs.mkdir(artifactRoot, { recursive: true });
  stackStarted = true;
  execFileSync('docker', [...composeArgs, '--profile', 'e2e', 'up', '-d', '--build', '--wait'], {
    stdio: 'inherit',
    env: { ...process.env, APP_COMMIT: commit, APP_BUILT_AT: new Date().toISOString() },
  });

  if (hasExecutable('go')) {
    steps.push(await run('backend', 'go', ['test', './...'], { cwd: path.join(process.cwd(), 'backend') }));
  } else {
    const backendImage = process.env.PERSEUS_BACKEND_TEST_IMAGE || 'golang:1.25-bookworm';
    steps.push(await run('backend-docker', 'docker', ['run', '--rm', '-v', `${path.join(process.cwd(), 'backend')}:/src`, '-w', '/src', backendImage, 'go', 'test', './...']));
  }
  steps.push(await runNpm('frontend-unit', 'test:unit'));
  steps.push(await runNpm('frontend-ui', 'test:ui'));
  steps.push(await runNpm('frontend-structure', 'test:structure'));
  steps.push(await runNpm('frontend-build', 'build'));
  steps.push(await runNpm('e2e', 'test:e2e', { playwright: true, artifactRole: 'playwright_e2e' }));
  steps.push(await runNpm('faults', 'test:e2e:faults', { playwright: true, artifactRole: 'playwright_faults' }));
  steps.push(await runNpm('provider-matrix', 'test:provider-matrix', { playwright: true, artifactRole: 'playwright_provider_matrix' }));
  const loadReport = path.join(artifactRoot, 'load', 'report.json');
  steps.push(await runNpm('load', 'test:load', { env: { PERSEUS_LOAD_REPORT: loadReport }, artifacts: [{ role: 'load', file: loadReport }] }));
  const soakRoot = path.join(artifactRoot, 'soak');
  steps.push(await runNpm('soak', 'test:soak', {
    playwright: true,
    artifactRole: 'playwright_soak_faults',
    env: { PERSEUS_SOAK_OUTPUT: soakRoot },
    artifacts: [{ role: 'soak', file: path.join(soakRoot, 'summary.json') }],
  }));
  if (liveProviderConfigured) {
    steps.push(await runNpm('provider-live', 'test:provider-live', { playwright: true, artifactRole: 'playwright_provider_live' }));
    providerLiveStatus = 'passed';
  } else {
    steps.push({ name: 'provider-live', status: 'unverified', reason: 'provider environment is not configured', artifacts: [] });
  }
  const maintenancePath = path.join(artifactRoot, 'maintenance', 'report.json');
  const maintenance = await runMaintenanceEvidence({ composeArgs, commit, output: maintenancePath });
  steps.push({
    name: 'maintenance', status: 'passed', exit_code: 0,
    started_at: maintenance.generated_at, completed_at: new Date().toISOString(),
    artifacts: [await describeArtifact('maintenance', maintenancePath)],
  });

  await writeGateReport('passed');
  const collectEnvironment = {
    ...process.env,
    PERSEUS_RELEASE_GATE_OUTPUT: outputPath,
    PERSEUS_EVIDENCE_OUTPUT: evidencePath,
    PERSEUS_REQUIRE_MAINTENANCE: '1',
    PERSEUS_REQUIRE_SCORECARD: '0',
  };
  let result = spawnSync(process.execPath, ['scripts/collect-release-evidence.mjs'], { env: collectEnvironment, stdio: 'inherit' });
  if ((result.status ?? 1) !== 0) throw new Error('strict release evidence collection failed');
  result = spawnSync(process.execPath, ['scripts/generate-project-scorecard.mjs'], {
    env: { ...process.env, PERSEUS_EVIDENCE_INPUT: evidencePath, PERSEUS_SCORECARD_OUTPUT: scorecardPath, PERSEUS_SCORECARD_MANIFEST: scorecardManifestPath },
    stdio: 'inherit',
  });
  if ((result.status ?? 1) !== 0) throw new Error('scorecard generation failed');
  result = spawnSync(process.execPath, ['scripts/collect-release-evidence.mjs'], {
    env: { ...collectEnvironment, PERSEUS_REQUIRE_SCORECARD: '1', PERSEUS_SCORECARD_MANIFEST: scorecardManifestPath },
    stdio: 'inherit',
  });
  if ((result.status ?? 1) !== 0) throw new Error('final strict release evidence collection failed');
  if (commandOutput('git', ['status', '--porcelain'])) throw new Error('release gate changed the tracked worktree');
} catch (error) {
  fatalError = error;
  if (error.gateItem) steps.push(error.gateItem);
}

if (stackStarted) {
  try {
    execFileSync('docker', [...composeArgs, '--profile', 'e2e', 'down', '-v'], { stdio: 'inherit' });
  } catch (error) {
    fatalError ||= error;
  }
}
let finalReport;
if (fatalError) {
  finalReport = await writeGateReport('failed', fatalError.message);
  await fs.rm(evidencePath, { force: true }).catch(() => {});
} else {
  finalReport = JSON.parse(await fs.readFile(outputPath, 'utf8'));
}
process.stdout.write(`${JSON.stringify(finalReport, null, 2)}\n`);
if (fatalError) process.exitCode = 1;
