import { execFileSync } from 'node:child_process';
import crypto from 'node:crypto';
import { atomicWriteJSON } from './release-evidence-lib.mjs';

function commandOutput(program, args) {
  return execFileSync(program, args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function commandJSON(program, args) {
  const raw = commandOutput(program, args);
  try {
    return JSON.parse(raw);
  } catch (error) {
    throw new Error(`${program} returned invalid JSON: ${error.message}`);
  }
}

export async function runMaintenanceEvidence({ composeArgs, commit, output, userDataRoot = '/data/users' }) {
  const before = commandJSON('docker', [...composeArgs, 'exec', '-T', 'webide', 'perseus-maintain', 'inspect', '--full-integrity']);
  commandOutput('docker', [...composeArgs, 'stop', 'webide']);
  let serviceStopped = true;
  try {
    const backup = commandJSON('docker', [...composeArgs, 'run', '--rm', '--no-deps', 'webide', 'perseus-maintain', 'backup']);
    const restore = commandJSON('docker', [...composeArgs, 'run', '--rm', '--no-deps', 'webide', 'perseus-maintain', 'verify-restore', '--backup', backup.backup_path, '--user-data-root', userDataRoot]);
    const archive = commandJSON('docker', [...composeArgs, 'run', '--rm', '--no-deps', 'webide', 'perseus-maintain', 'archive', '--before', '90d']);
    commandOutput('docker', [...composeArgs, 'run', '--rm', '--no-deps', 'webide', 'perseus-maintain', 'vacuum', '--apply']);
    commandOutput('docker', [...composeArgs, 'up', '-d', '--wait', 'webide']);
    serviceStopped = false;
    const after = commandJSON('docker', [...composeArgs, 'exec', '-T', 'webide', 'perseus-maintain', 'inspect', '--full-integrity']);
    const protectedTables = Object.fromEntries(Object.entries(restore.protected_tables || {}).sort(([left], [right]) => left.localeCompare(right)));
    const report = {
      schema_version: 2,
      generated_at: new Date().toISOString(),
      commit,
      status: 'passed',
      before,
      after,
      database_fingerprint: {
        path: before.database_path,
        page_size: before.page_size,
        before_pages: before.page_count,
        after_pages: after.page_count,
        backup_sha256: backup.sha256,
        protected_tables_sha256: crypto.createHash('sha256').update(JSON.stringify(protectedTables)).digest('hex'),
      },
      backup,
      backup_verified: backup.integrity === 'ok' && Boolean(backup.sha256),
      restore,
      restore_verified: restore.restore_verified === true && restore.temporary_restore_removed === true,
      archive_manifest: archive,
      vacuum_result: {
        before_bytes: before.database_bytes,
        after_bytes: after.database_bytes,
        reclaimed_bytes: before.database_bytes - after.database_bytes,
        before_integrity: before.integrity,
        after_integrity: after.integrity,
      },
    };
    if (!report.backup_verified || !report.restore_verified || before.integrity !== 'ok' || after.integrity !== 'ok') {
      throw new Error('maintenance verification did not satisfy release requirements');
    }
    await atomicWriteJSON(output, report);
    return report;
  } finally {
    if (serviceStopped) commandOutput('docker', [...composeArgs, 'up', '-d', '--wait', 'webide']);
  }
}
