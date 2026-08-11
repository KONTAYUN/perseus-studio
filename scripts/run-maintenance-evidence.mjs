import { execFileSync } from 'node:child_process';
import process from 'node:process';
import { runMaintenanceEvidence } from './maintenance-evidence-lib.mjs';

const production = process.env.PERSEUS_MAINTENANCE_PRODUCTION === '1';
const composeArgs = production
  ? ['compose']
  : ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.e2e.yml'];
const output = process.env.PERSEUS_MAINTENANCE_REPORT || 'test-results/maintenance-report.json';
const commit = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const report = await runMaintenanceEvidence({ composeArgs, commit, output, userDataRoot: process.env.USER_DATA_ROOT || '/data/users' });
process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
