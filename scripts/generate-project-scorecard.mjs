import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { atomicWriteJSON, evidenceBindingSHA256, fileSHA256, readJSONStrict } from './release-evidence-lib.mjs';

const input = process.env.PERSEUS_EVIDENCE_INPUT || 'test-results/release-evidence.json';
const output = path.resolve(process.env.PERSEUS_SCORECARD_OUTPUT || 'test-results/PROJECT_HEALTH_SCORECARD.generated.md');
const manifestPath = process.env.PERSEUS_SCORECARD_MANIFEST || `${output}.manifest.json`;
const evidence = await readJSONStrict(input);
if (evidence.schema_version !== 2 || evidence.validation_status !== 'passed' || !evidence.git_clean) {
  throw new Error('scorecard requires validated schema v2 evidence from a clean commit');
}
if (evidence.release_gate?.commit !== evidence.commit) throw new Error('scorecard evidence commit mismatch');
const currentCommit = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const gitStatus = execFileSync('git', ['status', '--porcelain'], { encoding: 'utf8' }).trim();
if (currentCommit !== evidence.commit) throw new Error('scorecard evidence does not match the current Git commit');
if (gitStatus !== '') throw new Error('scorecard generation requires a clean worktree');

const liveProviderVerified = evidence.provider_live === 'passed';
const deductions = [
  { value: 0.2, reason: 'Single-instance SQLite remains an explicit deployment boundary.' },
  { value: 0.1, reason: 'In-memory abuse counters reset after a process restart by design.' },
  { value: 0.1, reason: 'Several broad orchestration files remain in the responsibility review register.' },
  { value: 0.2, reason: 'Remote CI execution is not part of this local evidence bundle.' },
  ...(!liveProviderVerified ? [{ value: 0.3, reason: 'Live third-party provider evaluation is unverified.' }] : []),
];
const score = Math.max(0, 10 - deductions.reduce((sum, item) => sum + item.value, 0));
const tests = Object.values(evidence.tests || {});
const expected = tests.reduce((sum, item) => sum + Number(item.expected || 0), 0);
const skipped = tests.reduce((sum, item) => sum + Number(item.skipped || 0), 0);
const maintenance = evidence.maintenance;
const markdown = `# Perseus Studio Generated Release Scorecard

This report is generated from validated release evidence. Do not edit it manually.

## Result

- Commit: \`${evidence.commit}\`
- Generated: ${evidence.generated_at}
- Evidence validation: **${evidence.validation_status}**
- Evidence-backed score: **${score.toFixed(1)} / 10**
- Worktree clean: **${evidence.git_clean ? 'yes' : 'no'}**

## Runtime Evidence

- Playwright: ${expected} passed expectations, ${skipped} skipped, 0 unexpected.
- Load: ${evidence.load.configuration?.total_runs || 0} Runs, ${(evidence.load.failures || []).length} failures.
- Soak: ${evidence.soak.cycles || 0} cycles, ${evidence.soak.total_runs || 0} Runs, ${evidence.soak.restart_cycles || 0} restart cycles, ${(evidence.soak.failures || []).length} failures.
- Peak RSS: ${evidence.soak.max_peak_rss_bytes || 0} bytes.
- Maximum health p95: ${evidence.soak.max_api_health_ms_p95 || 0} ms.
- Maximum database growth per Run: ${evidence.soak.max_database_growth_per_run_bytes || 0} bytes.
- Container: UID ${evidence.container_identity.uid}, GID ${evidence.container_identity.gid}, image commit \`${evidence.version?.body?.commit || 'unknown'}\`.
- SQLite: ${evidence.sqlite.integrity_mode} integrity \`${evidence.sqlite.integrity}\`.

## Maintenance Evidence

- Backup verified: **${evidence.backup_verified ? 'yes' : 'no'}**.
- Temporary restore verified: **${evidence.restore_verified ? 'yes' : 'no'}**.
- Database before maintenance: ${maintenance.before?.database_bytes || 0} bytes.
- Database after maintenance: ${maintenance.after?.database_bytes || 0} bytes.
- Reclaimed by VACUUM: ${maintenance.vacuum_result?.reclaimed_bytes || 0} bytes.
- Protected tables verified: ${Object.keys(maintenance.restore?.protected_tables || {}).length}.
- User files in restore manifest: ${maintenance.restore?.user_data?.files || 0}.

## Explicit Deductions

${deductions.map((item) => `- -${item.value.toFixed(1)}: ${item.reason}`).join('\n')}

## Optional Evidence

- Live provider: **${evidence.provider_live}**.
`;

await fs.mkdir(path.dirname(output), { recursive: true });
const temporary = `${output}.tmp-${process.pid}`;
await fs.writeFile(temporary, markdown);
await fs.rename(temporary, output);
const sha256 = await fileSHA256(output);
const manifest = {
  schema_version: 1,
  generated_at: new Date().toISOString(),
  commit: evidence.commit,
  evidence_path: path.resolve(input),
  evidence_binding_sha256: evidenceBindingSHA256(evidence),
  path: output,
  sha256,
  score,
};
await atomicWriteJSON(manifestPath, manifest);
process.stdout.write(`${JSON.stringify(manifest, null, 2)}\n`);
