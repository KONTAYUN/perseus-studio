import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

const baseURL = process.env.PERSEUS_LOAD_BASE_URL || 'http://127.0.0.1:8080';
const userCount = Number(process.env.PERSEUS_LOAD_USERS || 20);
const runsPerUser = Number(process.env.PERSEUS_LOAD_RUNS_PER_USER || 5);
const restart = process.argv.includes('--restart');
const reportPath = process.env.PERSEUS_LOAD_REPORT || 'test-results/load-report.json';
const terminal = new Set(['succeeded', 'failed', 'canceled', 'delegated', 'waiting_for_user']);

class Client {
  constructor(cookie = '') { this.cookie = cookie; }

  async request(method, pathname, body) {
    const response = await fetch(`${baseURL}${pathname}`, {
      method,
      headers: {
        Origin: baseURL,
        ...(this.cookie ? { Cookie: this.cookie } : {}),
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const setCookie = response.headers.get('set-cookie');
    if (setCookie) this.cookie = setCookie.split(';', 1)[0];
    const text = await response.text();
    if (!response.ok) throw new Error(`${method} ${pathname}: ${response.status} ${text}`);
    return text ? JSON.parse(text) : null;
  }
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const quantile = (values, q) => {
  if (!values.length) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.ceil(sorted.length * q) - 1)];
};

async function waitReady(timeoutMs = 60_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${baseURL}/api/ready`);
      if (response.ok) return;
    } catch {}
    await sleep(250);
  }
  throw new Error('Perseus readiness timeout');
}

function compose(...args) {
  return execFileSync('docker', ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.e2e.yml', ...args], {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
  });
}

function containerInspect() {
  return JSON.parse(compose('exec', '-T', 'webide', 'perseus-maintain', 'inspect'));
}

function containerRSSBytes() {
  const containerID = compose('ps', '-q', 'webide').trim();
  if (!containerID) return 0;
  const raw = execFileSync('docker', ['stats', '--no-stream', '--format', '{{.MemUsage}}', containerID], { encoding: 'utf8' }).trim();
  const match = raw.match(/^([0-9.]+)([KMG]iB)/);
  if (!match) return 0;
  const scale = { KiB: 1024, MiB: 1024 ** 2, GiB: 1024 ** 3 }[match[2]];
  return Number(match[1]) * scale;
}

async function main() {
  await waitReady();
  const before = containerInspect();
  await fetch('http://127.0.0.1:9090/stats/reset', { method: 'POST' }).catch(() => {});

  const admin = new Client();
  await admin.request('POST', '/api/auth/login', {
    username: process.env.PERSEUS_E2E_USERNAME || 'admin',
    password: process.env.PERSEUS_E2E_PASSWORD || 'password',
  });
  const stamp = `${Date.now()}-${process.pid}`;
  const provider = await admin.request('POST', '/api/model-providers', {
    name: `Load fixture ${stamp}`,
    visibility: 'public', protocol: 'openai', adapter_mode: 'openai_compatible', auth_mode: 'bearer',
    supports_tools: 'supported', supports_parallel_tool_calls: 'unsupported', tool_argument_stream_mode: 'delta',
    supports_vision: 'supported', capability_source: 'user', base_url: 'http://provider-fixture:9090/v1',
    api_key: 'fixture-key', model_id: `fixture-load-${stamp}`, enabled: true, is_default: true,
    reasoning_capability: 'disabled', context_window_tokens: 32768,
  });

  const users = [];
  for (let index = 0; index < userCount; index += 1) {
    const username = `load-${stamp}-${index}`;
    const password = `Load-${stamp}-${index}!`;
    await admin.request('POST', '/api/admin/users', { username, password, role: 'user' });
    const client = new Client();
    await client.request('POST', '/api/auth/login', { username, password });
    users.push({ username, client });
  }

  const healthLatencies = [];
  let peakRSSBytes = 0;
  let sampling = true;
  const sampler = (async () => {
    while (sampling) {
      const started = performance.now();
      try {
        await fetch(`${baseURL}/api/health`);
        healthLatencies.push(performance.now() - started);
        peakRSSBytes = Math.max(peakRSSBytes, containerRSSBytes());
      } catch {}
      await sleep(250);
    }
  })();

  const submitted = [];
  await Promise.all(users.map(async ({ username, client }) => {
    for (let index = 0; index < runsPerUser; index += 1) {
      const conversation = await client.request('POST', '/api/conversations', {
        title: `Load ${index}`, selected_model_provider_id: provider.id,
      });
      const result = await client.request('POST', `/api/conversations/${conversation.id}/messages`, {
        content: `[fixture:load] ${username} run ${index}`,
        model_provider_id: provider.id,
      });
      submitted.push({ username, client, runID: result.run.id, submittedAt: Date.now() });
    }
  }));

  if (restart) {
    await sleep(700);
    compose('restart', 'webide');
    await waitReady();
  }

  await Promise.all(submitted.map(async (item) => {
    const deadline = Date.now() + 120_000;
    while (Date.now() < deadline) {
      try {
        const run = await item.client.request('GET', `/api/agent-runs/${item.runID}`);
        if (terminal.has(run.status)) {
          item.run = run;
          item.completedAt = Date.now();
          return;
        }
      } catch {}
      await sleep(150);
    }
    throw new Error(`run timeout: ${item.runID}`);
  }));

  sampling = false;
  await sampler;
  const after = containerInspect();
  const fixtureStats = await fetch('http://127.0.0.1:9090/stats').then((response) => response.json()).catch(() => ({}));
  const statuses = submitted.reduce((groups, item) => {
    (groups[item.run.status] ||= []).push(item);
    return groups;
  }, {});
  const report = {
    generated_at: new Date().toISOString(),
    configuration: { user_count: userCount, runs_per_user: runsPerUser, total_runs: submitted.length, restart },
    results: {
      statuses: Object.fromEntries(Object.entries(statuses).map(([status, items]) => [status, items.length])),
      duration_ms_p50: quantile(submitted.map((item) => item.completedAt - item.submittedAt), 0.5),
      duration_ms_p95: quantile(submitted.map((item) => item.completedAt - item.submittedAt), 0.95),
      api_health_ms_p95: quantile(healthLatencies, 0.95),
      peak_rss_bytes: Math.round(peakRSSBytes),
      provider_peak_concurrency: fixtureStats.peakActiveRequests || 0,
      provider_total_requests: fixtureStats.totalRequests || 0,
      database_growth_bytes: after.database_bytes - before.database_bytes,
      database_growth_per_run_bytes: Math.round((after.database_bytes - before.database_bytes) / submitted.length),
      sqlite_integrity: after.integrity,
      agent_event_payload_growth_bytes: (after.tables?.agent_events?.payload_bytes || 0) - (before.tables?.agent_events?.payload_bytes || 0),
      active_preview_rows: after.tables?.agent_edit_previews?.rows || 0,
    },
  };

  const failures = [];
  if (report.results.statuses.succeeded !== submitted.length) failures.push('not all runs succeeded');
  if (report.results.provider_peak_concurrency > 4) failures.push('global concurrency exceeded 4');
  if (report.results.api_health_ms_p95 >= 1000) failures.push('health p95 exceeded 1 second');
  if (report.results.peak_rss_bytes >= 2 * 1024 ** 3) failures.push('RSS exceeded 2 GiB');
  if (report.results.database_growth_per_run_bytes >= 500 * 1024) failures.push('database growth exceeded 500 KiB per run');
  if (report.results.sqlite_integrity !== 'ok') failures.push('SQLite integrity check failed');
  if (report.results.active_preview_rows !== 0) failures.push('completed runs retained active previews');
  report.failures = failures;
  await fs.mkdir(path.dirname(reportPath), { recursive: true });
  await fs.writeFile(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  if (failures.length) process.exitCode = 1;
}

main().catch((error) => {
  process.stderr.write(`${error.stack || error}\n`);
  process.exitCode = 1;
});
