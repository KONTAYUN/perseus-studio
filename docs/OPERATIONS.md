# Perseus Operations

## Non-root data upgrade

The production service always runs as UID/GID `10001`. Check an existing data
directory before upgrading:

```bash
docker compose --profile maintenance run --rm data-permissions-check
```

If the check fails because an older container created root-owned files, run the
explicit repair and check again:

```bash
docker compose --profile maintenance run --rm data-permissions-fix
docker compose --profile maintenance run --rm data-permissions-check
```

Set `PERSEUS_DATA_PATH` for a non-default data directory. The normal `webide`
service never elevates itself or changes ownership automatically.

## Database inspection

Run inspection inside the production container:

```bash
docker compose exec webide perseus-maintain inspect
```

The JSON report includes integrity status, database size, free pages, row counts,
and payload bytes for high-volume tables.

## Archiving run telemetry

Archiving is dry-run by default. It never removes messages, Turn Items, tool
calls, file history, attachments, or user files.

```bash
docker compose exec webide perseus-maintain archive --before 90d --output /data/archives/runs-90d.jsonl.gz
```

The dry-run uses a read-only connection. Apply it only in a maintenance window:

```bash
docker compose stop webide
docker compose run --rm --no-deps webide perseus-maintain archive --before 90d --output /data/archives/runs-90d.jsonl.gz --apply
docker compose up -d --wait webide
```

The apply form writes a gzip JSONL archive and SHA-256 manifest first. Only
after verification does one transaction compact terminal Run events and old
diagnostics. Event identity and sequence barriers remain in SQLite.

## Offline vacuum

Stop the service before reclaiming SQLite pages:

```bash
docker compose stop webide
docker compose run --rm --no-deps webide perseus-maintain vacuum --apply
docker compose up -d --wait webide
```

`vacuum` performs integrity checks before and after the operation. It is never
started automatically by an online request.

## Release evidence

Fast deterministic release gates:

```bash
npm run test:e2e:faults
npm run test:provider-matrix
npm run test:load
npm run test:release-evidence
```

Nightly soak defaults to 30 minutes and periodically restarts the service:

```bash
npm run test:soak
```

External provider verification is opt-in and reads temporary configuration only
from environment variables:

```bash
npm run test:provider-live
```
