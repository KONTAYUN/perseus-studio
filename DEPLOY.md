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
| `CORS_ALLOWED_ORIGINS` | _(empty)_ | Explicit browser origins; empty means same-origin only |
| `SESSION_COOKIE_SECURE` | `auto` | `auto`, `true`, or `false`; auto follows direct TLS or a trusted proxy |
| `TRUSTED_PROXY_CIDRS` | _(empty)_ | CIDRs allowed to supply forwarded scheme/client IP headers |
| `HTTP_READ_HEADER_TIMEOUT` | `10s` | Maximum request-header read time |
| `HTTP_IDLE_TIMEOUT` | `120s` | Keep-alive idle timeout |
| `HTTP_REQUEST_TIMEOUT` | `30s` | Timeout for ordinary JSON routes; SSE and uploads are not wrapped |
| `AUTH_LOGIN_RATE_PER_MINUTE` | `5` | Sustained login attempts per IP and username each minute |
| `AUTH_LOGIN_RATE_BURST` | `10` | Login burst capacity per IP and username |
| `AUTH_REGISTER_RATE_PER_HOUR` | `3` | Registration attempts per client IP each hour |
| `UPLOAD_RATE_PER_MINUTE` | `30` | Chat attachment uploads per authenticated user each minute |
| `AGENT_MAX_CONCURRENT_GLOBAL` | `4` | Global running Agent limit |
| `AGENT_MAX_CONCURRENT_PER_USER` | `2` | Per-user running Agent limit |
| `AGENT_MAX_QUEUED_GLOBAL` | `100` | Global persistent queue limit |
| `AGENT_MAX_QUEUED_PER_USER` | `10` | Per-user persistent queue limit |
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

The image runs as UID/GID `10001`. On Linux bind mounts, make the data directory
writable by that identity before startup:

```bash
sudo mkdir -p data
sudo chown -R 10001:10001 data
```

Startup fails explicitly when `/data` is not writable; it never falls back to root.

When upgrading an older root-running container, stop the service and repair the
existing bind mount once before starting the non-root image:

```bash
docker compose stop webide
docker run --rm --user 0 -v "$(pwd)/data:/data" debian:bookworm-slim chown -R 10001:10001 /data
docker compose up -d --force-recreate --wait
```

This migration changes ownership only; it does not rewrite the database or user files.

## Registry Mirrors

If `docker.io` is slow or blocked, set `IMAGE_REGISTRY` in `.env` to a compatible mirror (e.g. `docker.m.daocloud.io`). The Dockerfile uses `${IMAGE_REGISTRY}/library/...` for all base images.

## Reverse Proxy

When placing behind nginx or Caddy:

- Proxy all traffic to `127.0.0.1:8080`
- Enable WebSocket/SSE pass-through (the app uses SSE for task streaming)
- Set appropriate timeouts for long-running SSE connections
- Add the proxy address range to `TRUSTED_PROXY_CIDRS` before relying on forwarded HTTPS or client IP headers
- Set `CORS_ALLOWED_ORIGINS` only when a separate frontend origin is required

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
- Sessions are stored in SQLite and survive container restarts until expiry or explicit invalidation
- Cookie-authenticated write requests validate `Origin`; do not strip the header at the reverse proxy
- CORS is same-origin by default and never reflects arbitrary credentialed origins
- Login, registration and upload endpoints have configurable in-process abuse limits
- `/api/debug/model-tool-test` is disabled by default and requires admin login when enabled
- All admin operations require `admin` role and are enforced server-side

## Health And Capacity

- `/api/health` is a lightweight liveness endpoint.
- `/api/ready` verifies SQLite access, writable user storage and a started Agent scheduler.
- Agent Runs above the configured concurrency limit remain in the persistent `queued` state and are scheduled fairly across users.
- Background tasks retain their separate worker pool and do not consume Agent slots.

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
