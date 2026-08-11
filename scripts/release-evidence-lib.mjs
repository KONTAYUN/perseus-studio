import crypto from 'node:crypto';
import fs from 'node:fs/promises';
import path from 'node:path';

export class EvidenceInputError extends Error {
  constructor(code, file, message) {
    super(message);
    this.name = 'EvidenceInputError';
    this.code = code;
    this.file = file;
  }
}

export async function readJSONStrict(file) {
  let raw;
  try {
    raw = await fs.readFile(file, 'utf8');
  } catch (error) {
    throw new EvidenceInputError('evidence_artifact_missing', file, `required evidence artifact is unavailable: ${error.message}`);
  }
  try {
    return JSON.parse(raw);
  } catch (error) {
    throw new EvidenceInputError('evidence_artifact_invalid_json', file, `evidence artifact is not valid UTF-8 JSON: ${error.message}`);
  }
}

export async function fileSHA256(file) {
  const raw = await fs.readFile(file);
  return crypto.createHash('sha256').update(raw).digest('hex');
}

export function evidenceBindingSHA256(evidence) {
  const artifacts = (evidence.artifacts || [])
    .filter((artifact) => artifact.role !== 'scorecard')
    .map(({ role, path: artifactPath, bytes, sha256, format }) => ({ role, path: artifactPath, bytes, sha256, format }))
    .sort((left, right) => `${left.role}:${left.path}`.localeCompare(`${right.role}:${right.path}`));
  const binding = {
    schema_version: evidence.schema_version,
    commit: evidence.commit,
    git_clean: evidence.git_clean,
    validation_status: evidence.validation_status,
    release_gate: evidence.release_gate,
    artifacts,
  };
  return crypto.createHash('sha256').update(JSON.stringify(binding)).digest('hex');
}

export async function describeArtifact(role, file) {
  const absolute = path.resolve(file);
  const stat = await fs.stat(absolute).catch((error) => {
    throw new EvidenceInputError('evidence_artifact_missing', absolute, `required ${role} artifact is unavailable: ${error.message}`);
  });
  const sha256 = await fileSHA256(absolute);
  return {
    role,
    path: path.relative(process.cwd(), absolute).replaceAll(path.sep, '/'),
    bytes: stat.size,
    sha256,
    format: path.extname(absolute).toLowerCase() === '.json' ? 'json' : 'binary',
  };
}

export async function verifyArtifact(artifact) {
  if (!artifact?.role || !artifact?.path || !artifact?.sha256) {
    throw new EvidenceInputError('evidence_artifact_manifest_invalid', artifact?.path || '', 'artifact manifest is incomplete');
  }
  const absolute = path.resolve(artifact.path);
  const actual = await fileSHA256(absolute).catch((error) => {
    throw new EvidenceInputError('evidence_artifact_missing', absolute, error.message);
  });
  if (actual !== artifact.sha256) {
    throw new EvidenceInputError('evidence_artifact_checksum_mismatch', absolute, `${artifact.role} artifact checksum changed after the gate step`);
  }
  return artifact.format === 'json' || path.extname(absolute).toLowerCase() === '.json'
    ? readJSONStrict(absolute)
    : { verified: true, path: absolute };
}

export function findArtifact(gate, role) {
  for (const step of gate?.steps || []) {
    for (const artifact of step.artifacts || []) {
      if (artifact.role === role) return artifact;
    }
  }
  return null;
}

export function summarizePlaywright(report) {
  const hasStats = report?.stats && typeof report.stats === 'object';
  const unexpected = hasStats && typeof report.stats.unexpected === 'number' ? report.stats.unexpected : null;
  const topLevelErrors = Array.isArray(report?.errors) ? report.errors.length : null;
  const status = report?.status || (
    hasStats && unexpected !== null && topLevelErrors !== null
      ? (unexpected === 0 && topLevelErrors === 0 ? 'passed' : 'failed')
      : 'unknown'
  );
  return {
    status,
    started_at: report?.stats?.startTime || null,
    duration_ms: report?.stats?.duration ?? null,
    expected: report?.stats?.expected ?? null,
    unexpected,
    flaky: report?.stats?.flaky ?? null,
    skipped: report?.stats?.skipped ?? null,
  };
}

export function validateCollectedEvidence(evidence, { requireMaintenance = true, requireScorecard = false } = {}) {
  const errors = [];
  const add = (code, field, message) => errors.push({ code, field, message });
  const requireFiniteNumber = (object, field, pathPrefix, code) => {
    if (typeof object?.[field] !== 'number' || !Number.isFinite(object[field])) {
      add(code, `${pathPrefix}.${field}`, `${field} must be a finite number`);
      return null;
    }
    return object[field];
  };
  const requireZeroConsistency = (object, fields, pathPrefix, code) => {
    for (const field of fields) {
      const value = requireFiniteNumber(object, field, pathPrefix, `${code}_missing`);
      if (value !== null && value !== 0) add(code, `${pathPrefix}.${field}`, `${field} must be zero`);
    }
  };
  const consistencyFields = ['sequence_gap_runs', 'head_mismatch_runs', 'final_barrier_mismatch_runs', 'duplicate_mutation_ids'];
  if (!/^[0-9a-f]{40}$/.test(String(evidence.commit || ''))) add('evidence_commit_invalid', 'commit', 'commit must be a full Git SHA');
  if (!evidence.git_clean) add('evidence_worktree_dirty', 'git_clean', 'release evidence requires a clean worktree');
  if (evidence.release_gate?.schema_version !== 2) add('release_gate_schema_invalid', 'release_gate.schema_version', 'release gate schema v2 is required');
  if (evidence.release_gate?.git_clean !== true) add('release_gate_worktree_dirty', 'release_gate.git_clean', 'release gate did not start from a clean worktree');
  if (evidence.release_gate?.status !== 'passed') add('release_gate_not_passed', 'release_gate.status', 'release gate did not pass');
  if (evidence.release_gate?.commit !== evidence.commit) add('release_commit_mismatch', 'release_gate.commit', 'release gate and evidence commits differ');
  const requiredSteps = ['backend', 'frontend-unit', 'frontend-ui', 'frontend-structure', 'frontend-build', 'e2e', 'faults', 'provider-matrix', 'load', 'soak'];
  const steps = new Map((evidence.release_gate?.steps || []).map((step) => [step.name.replace(/-docker$/, ''), step]));
  for (const name of requiredSteps) {
    if (steps.get(name)?.status !== 'passed') add('release_step_not_passed', `release_gate.steps.${name}`, `${name} did not pass`);
  }
  if (evidence.health?.status !== 200) add('release_health_failed', 'health.status', 'health endpoint did not return 200');
  if (evidence.readiness?.status !== 200) add('release_readiness_failed', 'readiness.status', 'readiness endpoint did not return 200');
  if (evidence.version?.status !== 200 || evidence.version?.body?.commit !== evidence.commit) {
    add('release_image_commit_mismatch', 'version.body.commit', 'running image commit does not match release evidence');
  }
  if (evidence.container_identity?.uid !== 10001 || evidence.container_identity?.gid !== 10001) {
    add('release_container_identity_invalid', 'container_identity', 'release container must run as UID/GID 10001');
  }
  if (evidence.sqlite?.integrity_mode !== 'full' || evidence.sqlite?.integrity !== 'ok' || evidence.sqlite?.schema !== 'ok') {
    add('release_sqlite_integrity_failed', 'sqlite', 'full SQLite integrity and schema checks must pass');
  }
  requireZeroConsistency(evidence.sqlite?.event_consistency, consistencyFields, 'sqlite.event_consistency', 'release_sqlite_consistency_failed');
  for (const [name, report] of Object.entries(evidence.tests || {})) {
    if (report?.status !== 'passed' || Number(report?.unexpected || 0) !== 0) add('release_playwright_failed', `tests.${name}`, `${name} Playwright evidence is not passing`);
  }
  if (!evidence.tests || Object.keys(evidence.tests).length < 3) add('release_tests_missing', 'tests', 'E2E, fault, and provider matrix evidence are required');
  const load = evidence.load;
  if (!load || (load.failures || []).length > 0) {
    add('release_load_failed', 'load', 'load evidence is missing or contains failures');
  } else {
    const results = load.results || {};
    if (results.sqlite_integrity !== 'ok') add('release_load_sqlite_failed', 'load.results.sqlite_integrity', 'load SQLite integrity did not pass');
    const loadLatency = requireFiniteNumber(results, 'api_health_ms_p95', 'load.results', 'release_load_metric_missing');
    const loadRSS = requireFiniteNumber(results, 'peak_rss_bytes', 'load.results', 'release_load_metric_missing');
    const loadGrowth = requireFiniteNumber(results, 'database_growth_per_run_bytes', 'load.results', 'release_load_metric_missing');
    const loadConcurrency = requireFiniteNumber(results, 'provider_peak_concurrency', 'load.results', 'release_load_metric_missing');
    if (loadLatency !== null && loadLatency >= 1000) add('release_load_latency_exceeded', 'load.results.api_health_ms_p95', 'load health p95 exceeded one second');
    if (loadRSS !== null && loadRSS >= 2 * 1024 ** 3) add('release_load_memory_exceeded', 'load.results.peak_rss_bytes', 'load RSS exceeded 2 GiB');
    if (loadGrowth !== null && loadGrowth >= 500 * 1024) add('release_load_growth_exceeded', 'load.results.database_growth_per_run_bytes', 'load database growth exceeded 500 KiB per Run');
    if (loadConcurrency !== null && loadConcurrency > 4) add('release_load_concurrency_exceeded', 'load.results.provider_peak_concurrency', 'load provider concurrency exceeded four');
    requireZeroConsistency(results, ['active_preview_rows', 'event_sequence_gap_runs', 'event_head_mismatch_runs', 'final_barrier_mismatch_runs', 'duplicate_mutation_ids'], 'load.results', 'release_load_consistency_failed');
    if ((results.terminal_failures || []).length > 0) add('release_load_terminal_failures', 'load.results.terminal_failures', 'all load Runs must succeed');
  }
  const soak = evidence.soak;
  if (!soak || (soak.failures || []).length > 0 || Number(soak.total_runs || 0) <= 0) {
    add('release_soak_failed', 'soak', 'soak evidence is missing or contains failures');
  } else {
    const soakLatency = requireFiniteNumber(soak, 'max_api_health_ms_p95', 'soak', 'release_soak_metric_missing');
    const soakRSS = requireFiniteNumber(soak, 'max_peak_rss_bytes', 'soak', 'release_soak_metric_missing');
    const soakGrowth = requireFiniteNumber(soak, 'max_database_growth_per_run_bytes', 'soak', 'release_soak_metric_missing');
    const soakConcurrency = requireFiniteNumber(soak, 'max_provider_peak_concurrency', 'soak', 'release_soak_metric_missing');
    if (soakLatency !== null && soakLatency >= 1000) add('release_soak_latency_exceeded', 'soak.max_api_health_ms_p95', 'soak health p95 exceeded one second');
    if (soakRSS !== null && soakRSS >= 2 * 1024 ** 3) add('release_soak_memory_exceeded', 'soak.max_peak_rss_bytes', 'soak RSS exceeded 2 GiB');
    if (soakGrowth !== null && soakGrowth >= 500 * 1024) add('release_soak_growth_exceeded', 'soak.max_database_growth_per_run_bytes', 'soak database growth exceeded 500 KiB per Run');
    if (soakConcurrency !== null && soakConcurrency > 4) add('release_soak_concurrency_exceeded', 'soak.max_provider_peak_concurrency', 'soak provider concurrency exceeded four');
    requireZeroConsistency(soak, ['max_active_preview_rows', 'max_event_sequence_gap_runs', 'max_event_head_mismatch_runs', 'max_final_barrier_mismatch_runs', 'max_duplicate_mutation_ids'], 'soak', 'release_soak_consistency_failed');
  }
  if (requireMaintenance) {
    if (evidence.maintenance?.status !== 'passed') add('release_maintenance_missing', 'maintenance', 'verified maintenance evidence is required');
    if (evidence.maintenance?.commit !== evidence.commit) add('maintenance_commit_mismatch', 'maintenance.commit', 'maintenance and release commits differ');
    if (!evidence.backup_verified) add('release_backup_unverified', 'backup_verified', 'backup verification is required');
    if (!evidence.restore_verified) add('release_restore_unverified', 'restore_verified', 'an actual temporary restore verification is required');
    if (evidence.maintenance?.before?.integrity !== 'ok' || evidence.maintenance?.after?.integrity !== 'ok') {
      add('release_maintenance_integrity_failed', 'maintenance', 'maintenance requires full integrity before and after');
    }
    for (const phase of ['before', 'after']) {
      requireZeroConsistency(evidence.maintenance?.[phase]?.event_consistency, consistencyFields, `maintenance.${phase}.event_consistency`, 'release_maintenance_consistency_failed');
    }
  }
  if (requireScorecard && !evidence.scorecard_generated) add('release_scorecard_missing', 'scorecard_generated', 'generated scorecard evidence is required');
  return errors;
}

export async function atomicWriteJSON(file, value) {
  const absolute = path.resolve(file);
  await fs.mkdir(path.dirname(absolute), { recursive: true });
  const temporary = `${absolute}.tmp-${process.pid}`;
  await fs.writeFile(temporary, `${JSON.stringify(value, null, 2)}\n`);
  await fs.rename(temporary, absolute);
}
