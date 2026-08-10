import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const durationMs = Number(process.env.PERSEUS_SOAK_DURATION_MS || 30 * 60 * 1000);
const outputDir = process.env.PERSEUS_SOAK_OUTPUT || 'test-results/soak';
const deadline = Date.now() + durationMs;
const reports = [];
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm';
let cycle = 0;

function runProcess(program, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(program, args, {
      ...options,
      // Soak runs many child processes. Keep their output bounded and off the
      // terminal; otherwise a long Windows run can exhaust the console host.
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    const appendTail = (current, chunk) => {
      const next = current + chunk.toString();
      return next.length > 8192 ? next.slice(-8192) : next;
    };
    child.stdout.on('data', (chunk) => { stdout = appendTail(stdout, chunk); });
    child.stderr.on('data', (chunk) => { stderr = appendTail(stderr, chunk); });
    child.once('error', reject);
    child.once('close', (code, signal) => resolve({
      status: code ?? 1,
      signal,
      stdout,
      stderr,
    }));
  });
}

function runNpm(args) {
  const windowsCommand = process.platform === 'win32';
  const program = windowsCommand ? (process.env.ComSpec || 'cmd.exe') : npmCommand;
  const commandArgs = windowsCommand ? ['/d', '/c', npmCommand, ...args] : args;
  return runProcess(program, commandArgs, { env: process.env });
}

await fs.rm(outputDir, { recursive: true, force: true });
await fs.mkdir(outputDir, { recursive: true });
while (Date.now() < deadline) {
  cycle += 1;
  if (cycle === 1) {
    const faultResult = await runNpm(['run', 'test:e2e:faults']);
    if (faultResult.error || faultResult.status !== 0) {
      process.stderr.write(`initial fault checks failed\n${faultResult.stderr || faultResult.stdout}\n`);
      process.exit(faultResult.status || 1);
    }
  }
  const reportPath = path.join(outputDir, `cycle-${String(cycle).padStart(3, '0')}.json`);
  const args = ['scripts/agent-load.mjs', ...(cycle % 5 === 0 ? ['--restart'] : [])];
  const result = await runProcess(process.execPath, args, {
    stdio: 'inherit',
    env: {
      ...process.env,
      PERSEUS_LOAD_USERS: process.env.PERSEUS_SOAK_USERS || '20',
      PERSEUS_LOAD_RUNS_PER_USER: process.env.PERSEUS_SOAK_RUNS_PER_USER || '5',
      PERSEUS_LOAD_REPORT: reportPath,
    },
  });
  if (result.status !== 0) {
    process.stderr.write(`soak cycle ${cycle} failed\n${result.stderr || result.stdout}\n`);
    process.exit(result.status || 1);
  }
  reports.push(JSON.parse(await fs.readFile(reportPath, 'utf8')));
}

const summary = {
  generated_at: new Date().toISOString(),
  duration_ms: durationMs,
  cycles: reports.length,
  total_runs: reports.reduce((sum, report) => sum + report.configuration.total_runs, 0),
  restart_cycles: reports.filter((report) => report.configuration.restart).length,
  max_api_health_ms_p95: Math.max(...reports.map((report) => report.results.api_health_ms_p95), 0),
  max_peak_rss_bytes: Math.max(...reports.map((report) => report.results.peak_rss_bytes), 0),
  database_growth_bytes: reports.reduce((sum, report) => sum + report.results.database_growth_bytes, 0),
  max_database_growth_per_run_bytes: Math.max(...reports.map((report) => report.results.database_growth_per_run_bytes), 0),
  max_provider_peak_concurrency: Math.max(...reports.map((report) => report.results.provider_peak_concurrency), 0),
  max_active_preview_rows: Math.max(...reports.map((report) => report.results.active_preview_rows), 0),
  failures: reports.flatMap((report) => report.failures || []),
};
await fs.writeFile(path.join(outputDir, 'summary.json'), `${JSON.stringify(summary, null, 2)}\n`);
process.stdout.write(`${JSON.stringify(summary, null, 2)}\n`);
if (summary.failures.length) process.exitCode = 1;
