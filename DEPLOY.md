# Deployment Guide

## Quick Start (Docker Compose)

```bash
cp .env.example .env
# Edit .env as needed
docker compose up -d --build
# Open http://localhost:8080
# The first registered user automatically becomes admin.
```

## Configuration

All settings are environment variables. See `.env.example` for the full list.

| Variable | Default | Description |
|---|---|---|
| `BACKEND_ADDR` | `:8080` | Listen address |
| `BACKEND_DB_PATH` | `/data/app.db` | SQLite database path |
| `USER_DATA_ROOT` | `/data/users` | Per-user data root (workspaces, uploads, skills) |
| `FRONTEND_DIST` | `/app/dist` | Static frontend assets |
| `TASK_WORKER_COUNT` | `2` | Background task workers |
| `ENABLE_DEBUG_API` | `false` | Enable admin-only model/tool debug endpoint |
| `PERSEUS_ADMIN_USERNAME` | _(empty)_ | Auto-create admin on first startup (optional) |
| `PERSEUS_ADMIN_PASSWORD` | _(empty)_ | Password for auto-created admin |
| `IMAGE_REGISTRY` | `docker.io` | Docker base image registry |
| `APP_VERSION` | `dev` | Optional build-time version label |
| `APP_COMMIT` | `unknown` | Optional build-time commit label |
| `APP_BUILT_AT` | `unknown` | Optional build-time build timestamp |

## User System

- The **first registered user** automatically becomes `admin`.
- Admin can open/close registration in Settings > User Management.
- Admin can create, disable, enable, reset password, promote/demote, and delete users.
- **Deleting a user permanently removes** their account and all data under `<USER_DATA_ROOT>/<user_id>/`.
- Each user has an independent workspace, uploads, skills, conversations, tasks, and model/search providers.
- When no users exist, registration is always allowed regardless of the toggle setting (prevents lockout).

## Data Directory

The compose file mounts `./data:/data`. This single directory holds all persistent state:

```
data/
  app.db                       SQLite database (users, tasks, conversations, configs, skills)
  users/
    <user_id>/
      workspace/               Per-user workspace files
      uploads/                 Per-user attachment blobs
      skills/                  Per-user installed skill packages
```

Back up this directory to preserve all state. See [docs/BACKUP.md](docs/BACKUP.md) for a stop-backup-restore checklist.

## Registry Mirrors

If `docker.io` is slow or blocked, set `IMAGE_REGISTRY` in `.env` to a compatible mirror (e.g. `docker.m.daocloud.io`). The Dockerfile uses `${IMAGE_REGISTRY}/library/...` for all base images.

## Reverse Proxy

When placing behind nginx or Caddy:

- Proxy all traffic to `127.0.0.1:8080`
- Enable WebSocket/SSE pass-through (the app uses SSE for task streaming)
- Set appropriate timeouts for long-running SSE connections

Example nginx snippet:

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_read_timeout 300s;
}
```

## Security Notes

- Model provider API keys are stored in the SQLite database; protect the `data/` directory accordingly
- The workspace allows arbitrary file read/write within each user's workspace directory — treat it as a per-user sandbox
- Sessions are stored in-memory and lost on container restart; users must re-login after restart
- `/api/debug/model-tool-test` is disabled by default and requires admin login when enabled
- All admin operations require `admin` role and are enforced server-side

## Common Operations

```bash
# Rebuild after code changes
docker compose up -d --build --force-recreate

# View logs
docker compose logs -f webide

# Stop without deleting data
docker compose stop

# Stop and remove containers/network, keeping bind-mounted ./data
docker compose down

# Stop and remove data
docker compose down -v
```
