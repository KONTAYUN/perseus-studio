# Backup And Restore

Perseus Studio keeps runtime state in `data/` by default. A complete backup needs both the SQLite database and per-user files.

## Stop The App

```powershell
docker compose stop
```

## Backup

Copy these paths together:

- `.env`
- `docker-compose.yml`
- `data/app.db`
- `data/app.db-wal` and `data/app.db-shm` if present
- `data/users/`

The safest option is to copy the whole `data/` directory while the container is stopped.

## Restore

1. Stop the app.
2. Replace the target `data/` directory with the backed-up `data/`.
3. Restore `.env` if needed.
4. Start the app:

```powershell
docker compose up -d
```

## Notes

- Do not run `docker compose down -v` unless you intentionally want to remove Docker volumes.
- The default compose file bind-mounts `./data:/data`, so normal backups can use regular filesystem copies.
- New versions reserve the database and user-data root with retained `.server.lock` files. Do not delete these to bypass an active owner. Lock ownership is released by the OS when the process exits; copied lock-file bytes do not retain the old process lock.
- Include each user's `.command-home` in full data backups if command-specific configuration matters. Never treat these homes as a sandbox or a place to copy server credentials.
- File-version and reversal-journal records live in SQLite; restoring only files or only the database can invalidate safe reversal. Restore a matched data directory with the app stopped, and verify login, workspace content, trash restoration, and `/api/ready` before accepting new work.
