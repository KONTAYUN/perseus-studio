import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const output = process.env.PERSEUS_EVIDENCE_OUTPUT || 'test-results/release-evidence.json';
const baseURL = process.env.PERSEUS_E2E_BASE_URL || 'http://127.0.0.1:8080';

const command = (program, args) => {
  try {
    return execFileSync(program, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  } catch (error) {
    return { error: error.stderr?.toString().trim() || error.message };
  }
};

const readJSON = async (file) => {
  try { return JSON.parse(await fs.readFile(file, 'utf8')); } catch { return null; }
};

const fetchJSON = async (pathname) => {
  try {
    const response = await fetch(`${baseURL}${pathname}`);
    return { status: response.status, body: await response.json() };
  } catch (error) {
    return { status: 0, error: error.message };
  }
};

const composeArgs = ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.e2e.yml'];
const inspectRaw = command('docker', [...composeArgs, 'exec', '-T', 'webide', 'perseus-maintain', 'inspect']);
let sqlite = inspectRaw;
if (typeof inspectRaw === 'string') {
  try { sqlite = JSON.parse(inspectRaw); } catch {}
}

const playwright = await readJSON('test-results/playwright-results.json');
const load = await readJSON('test-results/load-report.json');
const evidence = {
  schema_version: 1,
  generated_at: new Date().toISOString(),
  commit: command('git', ['rev-parse', 'HEAD']),
  worktree: command('git', ['status', '--short']),
  health: await fetchJSON('/api/health'),
  readiness: await fetchJSON('/api/ready'),
  container_identity: command('docker', [...composeArgs, 'exec', '-T', 'webide', 'id']),
  sqlite,
  tests: playwright ? {
    status: playwright.status,
    started_at: playwright.stats?.startTime,
    duration_ms: playwright.stats?.duration,
    expected: playwright.stats?.expected,
    unexpected: playwright.stats?.unexpected,
    flaky: playwright.stats?.flaky,
    skipped: playwright.stats?.skipped,
  } : null,
  load,
};

await fs.mkdir(path.dirname(output), { recursive: true });
await fs.writeFile(output, `${JSON.stringify(evidence, null, 2)}\n`);
process.stdout.write(`${JSON.stringify(evidence, null, 2)}\n`);
