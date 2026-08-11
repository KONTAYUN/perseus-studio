import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import {
  EvidenceInputError,
  describeArtifact,
  readJSONStrict,
  summarizePlaywright,
  validateCollectedEvidence,
  verifyArtifact,
} from '../../scripts/release-evidence-lib.mjs';

test('Playwright JSON status is derived from structured stats and errors', () => {
  assert.equal(summarizePlaywright({ stats: { expected: 4, unexpected: 0 }, errors: [] }).status, 'passed');
  assert.equal(summarizePlaywright({ stats: { expected: 3, unexpected: 1 }, errors: [] }).status, 'failed');
  assert.equal(summarizePlaywright({ stats: { expected: 4, unexpected: 0 }, errors: [{}] }).status, 'failed');
  assert.equal(summarizePlaywright({}).status, 'unknown');
});

test('strict JSON reader distinguishes missing, BOM, and corrupt evidence', async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'perseus-evidence-'));
  await assert.rejects(() => readJSONStrict(path.join(root, 'missing.json')), (error) => error instanceof EvidenceInputError && error.code === 'evidence_artifact_missing');
  const bom = path.join(root, 'bom.json');
  await fs.writeFile(bom, '\ufeff{"ok":true}');
  await assert.rejects(() => readJSONStrict(bom), (error) => error.code === 'evidence_artifact_invalid_json');
  const corrupt = path.join(root, 'corrupt.json');
  await fs.writeFile(corrupt, '{');
  await assert.rejects(() => readJSONStrict(corrupt), (error) => error.code === 'evidence_artifact_invalid_json');
});

test('artifact checksum detects mutation after a release step', async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'perseus-artifact-'));
  const file = path.join(root, 'report.json');
  await fs.writeFile(file, '{"status":"passed"}\n');
  const artifact = await describeArtifact('fixture', file);
  await verifyArtifact(artifact);
  await fs.writeFile(file, '{"status":"failed"}\n');
  await assert.rejects(() => verifyArtifact(artifact), (error) => error.code === 'evidence_artifact_checksum_mismatch');
});

test('strict validation rejects false-positive release evidence', () => {
  const evidence = {
    commit: 'a'.repeat(40), git_clean: true,
    release_gate: { schema_version: 2, commit: 'a'.repeat(40), git_clean: true, status: 'passed', steps: [] },
    health: { status: 500 }, readiness: { status: 200 },
    container_identity: { uid: 0, gid: 0 },
    sqlite: { integrity_mode: 'not_run', integrity: 'not_run', schema: 'ok' },
    tests: null, load: null, soak: null, maintenance: null,
  };
  const codes = validateCollectedEvidence(evidence, { requireMaintenance: true, requireScorecard: true }).map((item) => item.code);
  for (const code of ['release_step_not_passed', 'release_health_failed', 'release_container_identity_invalid', 'release_sqlite_integrity_failed', 'release_tests_missing', 'release_load_failed', 'release_soak_failed', 'release_maintenance_missing', 'release_restore_unverified', 'release_scorecard_missing']) {
    assert.ok(codes.includes(code), `missing validation error ${code}`);
  }
});

test('scorecard is generated from the evidence commit and runtime totals', async () => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'perseus-scorecard-'));
  execFileSync('git', ['init'], { cwd: root });
  execFileSync('git', ['config', 'user.email', 'scorecard@example.invalid'], { cwd: root });
  execFileSync('git', ['config', 'user.name', 'Scorecard Test'], { cwd: root });
  execFileSync('git', ['config', 'core.autocrlf', 'false'], { cwd: root });
  await fs.writeFile(path.join(root, 'tracked.txt'), 'fixture\n');
  await fs.writeFile(path.join(root, '.gitignore'), 'evidence.json\nscorecard.md\nscorecard.md.manifest.json\n');
  execFileSync('git', ['add', 'tracked.txt', '.gitignore'], { cwd: root });
  execFileSync('git', ['commit', '-m', 'fixture'], { cwd: root });
  const evidencePath = path.join(root, 'evidence.json');
  const output = path.join(root, 'scorecard.md');
  const currentCommit = execFileSync('git', ['rev-parse', 'HEAD'], { cwd: root, encoding: 'utf8' }).trim();
  const evidence = {
    schema_version: 2, validation_status: 'passed', git_clean: true,
    generated_at: '2026-08-11T00:00:00Z', commit: currentCommit,
    release_gate: { commit: currentCommit },
    provider_live: 'unverified',
    tests: { e2e: { expected: 46, skipped: 1 } },
    load: { configuration: { total_runs: 100 }, failures: [] },
    soak: { cycles: 73, total_runs: 7300, restart_cycles: 14, failures: [], max_peak_rss_bytes: 10, max_api_health_ms_p95: 2, max_database_growth_per_run_bytes: 3 },
    container_identity: { uid: 10001, gid: 10001 }, sqlite: { integrity_mode: 'full', integrity: 'ok' },
    backup_verified: true, restore_verified: true,
    maintenance: { before: { database_bytes: 20 }, after: { database_bytes: 10 }, vacuum_result: { reclaimed_bytes: 10 }, restore: { protected_tables: { users: {} }, user_data: { files: 2 } } },
  };
  await fs.writeFile(evidencePath, JSON.stringify(evidence));
  execFileSync(process.execPath, [path.resolve('scripts/generate-project-scorecard.mjs')], {
    cwd: root,
    env: { ...process.env, PERSEUS_EVIDENCE_INPUT: evidencePath, PERSEUS_SCORECARD_OUTPUT: output, PERSEUS_SCORECARD_MANIFEST: `${output}.manifest.json` },
  });
  const markdown = await fs.readFile(output, 'utf8');
  assert.match(markdown, new RegExp(currentCommit));
  assert.match(markdown, /73 cycles, 7300 Runs/);
  assert.doesNotMatch(markdown, /7400/);
  await fs.appendFile(path.join(root, 'tracked.txt'), 'dirty\n');
  assert.throws(() => execFileSync(process.execPath, [path.resolve('scripts/generate-project-scorecard.mjs')], {
    cwd: root,
    env: { ...process.env, PERSEUS_EVIDENCE_INPUT: evidencePath, PERSEUS_SCORECARD_OUTPUT: output, PERSEUS_SCORECARD_MANIFEST: `${output}.manifest.json` },
    stdio: ['ignore', 'pipe', 'pipe'],
  }));
});
