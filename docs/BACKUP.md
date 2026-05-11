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
