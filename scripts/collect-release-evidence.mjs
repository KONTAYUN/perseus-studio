import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import {
  EvidenceInputError,
  atomicWriteJSON,
  describeArtifact,
  evidenceBindingSHA256,
  fileSHA256,
  findArtifact,
  readJSONStrict,
  summarizePlaywright,
  validateCollectedEvidence,
  verifyArtifact,
} from './release-evidence-lib.mjs';

const output = process.env.PERSEUS_EVIDENCE_OUTPUT || 'test-results/release-evidence.json';
const gatePath = process.env.PERSEUS_RELEASE_GATE_OUTPUT || 'test-results/release-gate.json';
const baseURL = process.env.PERSEUS_E2E_BASE_URL || 'http://127.0.0.1:8080';
const requireMaintenance = process.env.PERSEUS_REQUIRE_MAINTENANCE !== '0';
const requireScorecard = process.env.PERSEUS_REQUIRE_SCORECARD === '1';

function command(program, args, code) {
  try {
    return execFileSync(program, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  } catch (error) {
    throw new EvidenceInputError(code, program, error.stderr?.toString().trim() || error.message);
  }
}

async function fetchJSON(pathname) {
  try {
    const response = await fetch(`${baseURL}${pathname}`);
    let body = null;
    try {
      body = await response.json();
    } catch (error) {
      throw new EvidenceInputError('release_endpoint_invalid_json', pathname, `${pathname} did not return JSON: ${error.message}`);
    }
    return { status: response.status, body };
  } catch (error) {
    if (error instanceof EvidenceInputError) throw error;
    throw new EvidenceInputError('release_endpoint_unavailable', pathname, error.message);
  }
}

function parseContainerIdentity(raw) {
  const uid = Number(raw.match(/(?:^|\s)uid=(\d+)/)?.[1]);
  const gid = Number(raw.match(/(?:^|\s)gid=(\d+)/)?.[1]);
  return { raw, uid: Number.isFinite(uid) ? uid : null, gid: Number.isFinite(gid) ? gid : null };
}

async function loadGateArtifact(gate, role) {
  const artifact = findArtifact(gate, role);
  if (!artifact) throw new EvidenceInputError('evidence_artifact_manifest_missing', gatePath, `release gate did not register ${role}`);
  return { artifact, report: await verifyArtifact(artifact) };
}

async function collect() {
  const gate = await readJSONStrict(gatePath);
  await Promise.all((gate.steps || []).flatMap((step) => step.artifacts || []).map((artifact) => verifyArtifact(artifact)));
  const currentCommit = command('git', ['rev-parse', 'HEAD'], 'evidence_git_commit_unavailable');
  const gitStatus = command('git', ['status', '--porcelain'], 'evidence_git_status_unavailable');
  const composeArgs = process.env.PERSEUS_EVIDENCE_PRODUCTION === '1'
    ? ['compose']
    : ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.e2e.yml'];

  const roles = ['playwright_e2e', 'playwright_faults', 'playwright_provider_matrix', 'load', 'soak'];
  const loaded = Object.fromEntries(await Promise.all(roles.map(async (role) => [role, await loadGateArtifact(gate, role)])));
  let maintenanceArtifact;
  let maintenance;
  const explicitMaintenance = process.env.PERSEUS_MAINTENANCE_REPORT;
  if (explicitMaintenance) {
    maintenance = await readJSONStrict(explicitMaintenance);
    maintenanceArtifact = await describeArtifact('maintenance', explicitMaintenance);
  } else {
    const loadedMaintenance = await loadGateArtifact(gate, 'maintenance');
    maintenance = loadedMaintenance.report;
    maintenanceArtifact = loadedMaintenance.artifact;
  }

  let scorecardGenerated = false;
  let scorecardArtifact = null;
  if (process.env.PERSEUS_SCORECARD_MANIFEST) {
    const manifest = await readJSONStrict(process.env.PERSEUS_SCORECARD_MANIFEST);
    if (manifest.commit !== currentCommit) throw new EvidenceInputError('scorecard_commit_mismatch', process.env.PERSEUS_SCORECARD_MANIFEST, 'scorecard commit does not match current commit');
    const actual = await fileSHA256(manifest.path);
    if (actual !== manifest.sha256) throw new EvidenceInputError('scorecard_checksum_mismatch', manifest.path, 'generated scorecard checksum does not match its manifest');
    const existingEvidence = await readJSONStrict(output);
    const evidenceBinding = evidenceBindingSHA256(existingEvidence);
    if (!manifest.evidence_binding_sha256 || manifest.evidence_binding_sha256 !== evidenceBinding) {
      throw new EvidenceInputError('scorecard_evidence_checksum_mismatch', output, 'scorecard was not generated from the current validated evidence');
    }
    scorecardArtifact = await describeArtifact('scorecard', manifest.path);
    scorecardGenerated = true;
  }

  const inspectRaw = command('docker', [...composeArgs, 'exec', '-T', 'webide', 'perseus-maintain', 'inspect', '--full-integrity'], 'release_sqlite_inspect_failed');
  let sqlite;
  try {
    sqlite = JSON.parse(inspectRaw);
  } catch (error) {
    throw new EvidenceInputError('release_sqlite_inspect_invalid_json', 'perseus-maintain inspect', error.message);
  }
  const identityRaw = command('docker', [...composeArgs, 'exec', '-T', 'webide', 'id'], 'release_container_identity_unavailable');
  const tests = {
    e2e: summarizePlaywright(loaded.playwright_e2e.report),
    faults: summarizePlaywright(loaded.playwright_faults.report),
    provider_matrix: summarizePlaywright(loaded.playwright_provider_matrix.report),
  };
  const artifactsByKey = new Map();
  for (const artifact of (gate.steps || []).flatMap((step) => step.artifacts || [])) {
    artifactsByKey.set(`${artifact.role}:${artifact.path}`, artifact);
  }
  artifactsByKey.set(`${maintenanceArtifact.role}:${maintenanceArtifact.path}`, maintenanceArtifact);
  if (scorecardArtifact) artifactsByKey.set(`${scorecardArtifact.role}:${scorecardArtifact.path}`, scorecardArtifact);
  const artifacts = [...artifactsByKey.values()];
  const evidence = {
    schema_version: 2,
    generated_at: new Date().toISOString(),
    commit: currentCommit,
    git_clean: gitStatus === '',
    worktree: gitStatus,
    validation_status: 'pending',
    validation_errors: [],
    release_gate: gate,
    artifacts,
    health: await fetchJSON('/api/health'),
    readiness: await fetchJSON('/api/ready'),
    version: await fetchJSON('/api/version'),
    container_identity: parseContainerIdentity(identityRaw),
    sqlite,
    tests,
    load: loaded.load.report,
    soak: loaded.soak.report,
    provider_live: gate.provider_live || 'unverified',
    backup_verified: maintenance?.backup_verified === true,
    restore_verified: maintenance?.restore_verified === true,
    maintenance_before: maintenance?.before || null,
    maintenance_after: maintenance?.after || null,
    archive_manifest: maintenance?.archive_manifest || null,
    vacuum_result: maintenance?.vacuum_result || null,
    scorecard_generated: scorecardGenerated,
    maintenance,
  };
  evidence.validation_errors = validateCollectedEvidence(evidence, { requireMaintenance, requireScorecard });
  evidence.validation_status = evidence.validation_errors.length === 0 ? 'passed' : 'failed';
  if (evidence.validation_errors.length > 0) {
    throw Object.assign(new EvidenceInputError('release_evidence_validation_failed', output, 'release evidence did not satisfy the strict gate'), { evidence });
  }
  await atomicWriteJSON(output, evidence);
  process.stdout.write(`${JSON.stringify(evidence, null, 2)}\n`);
}

try {
  await collect();
} catch (error) {
  const failure = error.evidence || {
    schema_version: 2,
    generated_at: new Date().toISOString(),
    validation_status: 'failed',
    validation_errors: [{ code: error.code || 'release_evidence_collection_failed', field: error.file || '', message: error.message }],
  };
  const failurePath = `${output}.invalid.json`;
  await atomicWriteJSON(failurePath, failure).catch(() => {});
  process.stderr.write(`${JSON.stringify({ code: error.code || 'release_evidence_collection_failed', file: error.file || '', message: error.message, failure_report: path.resolve(failurePath) })}\n`);
  process.exitCode = 1;
}
