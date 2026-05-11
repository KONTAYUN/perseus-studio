# Perseus Studio

Browser-based IDE with integrated AI chat, background task execution, and a skill/plugin system. Multi-user support with per-user data isolation. Single-container deployment via Docker.

## Features

- **Multi-user** — registration, login, per-user data isolation
- **Admin panel** — user management, registration toggle, role management
- **File editor** with tabs, syntax highlighting, drag-drop upload
- **AI chat** supporting OpenAI and Anthropic protocols with tool calling
- **Background tasks** with real-time SSE streaming
- **Skill system** — install via zip upload or chat attachment
- **Web search/extract** via Tavily or Brave providers
- **Workspace sandbox** with soft-delete (`回收站`), per-user isolation

## Quick Start

```bash
cp .env.example .env
docker compose up -d --build
# Open http://localhost:8080
# The first registered user automatically becomes admin.
```

## User System

- The **first registered user** automatically becomes `admin`.
- Admin can **open/close registration** in Settings → User Management.
- Admin can **create, disable, enable, reset password, promote/demote, delete** users.
- **Deleting a user permanently removes** their account and all data under `/data/users/<user_id>/`. This is irreversible.
- Each user has an independent workspace, uploads, skills, conversations, tasks, and model/search providers.
- When no users exist, registration is always allowed (prevents lockout).

## Local Development

### Frontend

```bash
npm install
npm run dev          # Vite dev server on :5173
```

### Backend

```bash
cd backend
go run ./cmd/server  # Listens on :8080
```

Set `FRONTEND_DIST=../dist` and configure other env vars per `.env.example`.

### Tests

```bash
npm run test:unit
npm run build
cd backend && go test ./...

# Optional full local smoke after starting Docker
npm run test:e2e
```

## Data Persistence

All runtime data lives under `./data/` (mounted as `/data` in Docker):

| Path | Content |
|---|---|
| `data/app.db` | SQLite — users, tasks, conversations, configs, skills |
| `data/users/<user_id>/workspace/` | Per-user workspace files |
| `data/users/<user_id>/uploads/` | Per-user uploads |
| `data/users/<user_id>/skills/` | Per-user installed skill packages |

Legacy paths (`data/workspace/`, `data/uploads/`, `data/skills/`) are no longer used by new installations.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — Module layout, key flows, data model
- [DEPLOY.md](DEPLOY.md) — Docker deployment, configuration, reverse proxy, security
- [docs/BACKUP.md](docs/BACKUP.md) — Backup and restore checklist
