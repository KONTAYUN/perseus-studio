# Perseus Studio

English | [简体中文](README.zh-CN.md)

Browser-based IDE with integrated AI chat, background task execution, and a skill/plugin system. Multi-user support with per-user data isolation. Single-container deployment via Docker.

Some internal package names still use the historical `webide-*` naming for compatibility with existing module imports and lockfiles. Product-facing documentation and UI use **Perseus Studio**.

## Features

- **Multi-user** — registration, login, per-user data isolation
- **Admin panel** — user management, registration toggle, role management
- **File editor** with tabs, syntax highlighting, drag-drop upload
- **AI chat** supporting OpenAI and Anthropic protocols with tool calling
- **Background tasks** with real-time SSE streaming
- **Skill system** — install via zip upload or chat attachment
- **Web search/extract** via Tavily or Brave providers
- **Workspace file management** with soft-delete (`回收站`), per-user file API paths and durable patch recovery; host commands are disabled by default

## Quick Start

```bash
cp .env.example .env
docker compose up -d --build
# Open http://localhost:8080
# The first registered user automatically becomes admin.
```

## Docker Image

Pushes to `master` automatically build and publish a container image to GitHub Container Registry:

```text
ghcr.io/kontayun/perseus-studio:latest
ghcr.io/kontayun/perseus-studio:<commit-sha>
```

For private packages, log in on the server with a GitHub token that has `read:packages` permission:

```bash
docker login ghcr.io
docker pull ghcr.io/kontayun/perseus-studio:latest
```

You can deploy with an image-based Compose file and update manually:

```bash
docker compose pull
docker compose up -d
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

Use Node.js 22.12+ (CI/Docker use Node 22). The dependency engine range also permits Node 20.19+, but Node 20 is no longer the deployment default.

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

Workspace commands require `PERSEUS_COMMAND_EXECUTION=trusted_host` and an administrator account. This explicitly enables execution as the service OS identity, including its filesystem and network permissions; it does not provide a sandbox. Leave it unset for file-tool-only use. A restricted OS worker is still required before offering command execution to untrusted users.

File edits execute through structured tool calls. Markdown examples and patch text in assistant prose are display-only. In provider advanced settings, `System message layout` explicitly selects one combined system message (portable default) or multiple system messages; display names do not determine runtime capabilities. Configure the actual context window for your provider instead of relying on model-name guesses.

Completion has `success`, `partial` and `blocked` task outcomes, independent of the run transport finishing successfully. Unexpected command exit codes remain unresolved until an exact command rerun succeeds; expected nonzero codes can be declared explicitly. Each invocation has shared provider request/time limits (foreground: 192 requests/20 minutes; background: 384 requests/2 hours). Retries and fallbacks consume the same request budget; a new user-initiated resume starts a new invocation budget. These are not monetary spending limits.

Patch recovery records original/intended content before writes and stops on conflicting subsequent edits. This covers tested process-exit stages, not a filesystem/SQLite ACID transaction or power-loss guarantee. See [design audit remediation](docs/DESIGN_AUDIT_REMEDIATION.md) for evidence and remaining boundaries.

Builds use Go 1.27 in CI/Docker; the source module retains its Go 1.25 minimum. The Vite dev proxy works with the default same-host origin policy. For a separate frontend connecting directly to the API, list its exact URL in `CORS_ALLOWED_ORIGINS`.

### Tests

```bash
npm run test:ui
npm run test:unit
npm run build
cd backend && go test ./...

# Optional full local smoke after starting Docker
npm run test:e2e
```

Browser tests can also run against a native backend using `PERSEUS_E2E_BASE_URL`, `PERSEUS_E2E_USERNAME`, and `PERSEUS_E2E_PASSWORD`. Use a disposable test database and workspace: tests create model providers and files. The agent editing tests use a local fake provider and require an admin account, with no paid API credentials. See [the project diagnosis](docs/PROJECT_DIAGNOSIS.md) for verified fixes and remaining risks.

Live provider editing tests are opt-in and make billable API requests. Set `PERSEUS_LIVE_MODEL_BASE_URL`, `PERSEUS_LIVE_MODEL_IDS` (comma-separated), and `PERSEUS_LIVE_MODEL_API_KEY`, then run `npx playwright test tests/e2e/live-agent-edit.spec.js` with the backend/account variables above. They verify changes to an existing file, an empty file, and a new file, plus editor refresh. Use a disposable backend: provider credentials are stored in its database until test cleanup. An in-memory authenticated proxy can inject the real key while the backend stores only a placeholder. Ordinary CI skips these tests.

## Markdown Messages and Documents

Assistant replies render Markdown headings, emphasis, lists, tables and fenced code blocks. Each code block has a copy button; commands are copied as text and are never executed by that button. Existing Markdown replies also use the renderer. User messages retain their literal input.

Open a `.md`, `.markdown` or `.mdown` file to edit it as a rich document. Use the single mode icon in the editor header to switch rich/source views; hover for the current mode and action. The editor download button has been removed; file-tree downloads remain available. Select text for the floating formatting toolbar; click the six-dot paragraph handle for headings, lists, tasks, quotes, code blocks and tables. Table menus can add rows/columns or remove the table. Ctrl/Cmd+S and autosave use the existing file-save and conflict-protection flow.

An amber warning icon appears for suspected Markdown mistakes, such as unclosed code fences/emphasis/links or inconsistent table columns. Open it to review line-numbered hints and explicitly request an Agent repair. The request saves and validates the current file first, preserves composer drafts, and asks the current Agent to read the file and make minimal formatting changes. Save errors/conflicts prevent submission. These are conservative hints, not a complete Markdown validator; no automatic repair runs merely because a warning appears. See [the editor polish review](docs/EDITOR_POLISH_REVIEW.md).

Files are still saved as Markdown. Formatting an edited document may normalize Markdown syntax, while opening or changing views does not rewrite the file. Frontmatter, raw HTML and other unsupported extensions remain in a read-only preview with source editing available. Documents over 512K UTF-16 code units also use this fallback. Relative workspace images are not currently resolved through the authenticated file API; external images can render. Arbitrary HTML formatting, text colors and block dragging are not included.

See [the Markdown editor review](docs/MARKDOWN_EDITOR_REVIEW.md) for implementation boundaries, regression coverage and remaining limits.

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

- [docs/PROJECT_DIRECTOR_REVIEW.md](docs/PROJECT_DIRECTOR_REVIEW.md) — Architecture comparison, reliability gaps, and a phased improvement plan
- [docs/OVERNIGHT_RELIABILITY_REVIEW.md](docs/OVERNIGHT_RELIABILITY_REVIEW.md) — Latest fixes, validation and remaining limits
- [docs/MATURE_AGENT_COMPARISON.md](docs/MATURE_AGENT_COMPARISON.md) — Source-based comparison with Codex, Claude Code, Roo Code and OpenClaw
- [ARCHITECTURE.md](ARCHITECTURE.md) — Module layout, key flows, data model
- [DEPLOY.md](DEPLOY.md) — Docker deployment, configuration, reverse proxy, security
- [HARNESS.md](HARNESS.md) — Agent behavior rules for workspace, tools, context, tasks, diagnostics, and frontend UI governance
- [docs/UI_DESIGN_SYSTEM.md](docs/UI_DESIGN_SYSTEM.md) — Frontend UI standards for buttons, inputs, panels, motion, color, and reusable components
- [docs/BACKUP.md](docs/BACKUP.md) — Backup and restore checklist


### Runtime controls

`AGENT_CONCURRENCY` (default 2) limits active agent jobs; `AGENT_QUEUE_CAPACITY` (default 64) limits all admitted running and queued jobs. Chat and background jobs share the scheduler and serialize by user workspace. Extra admissions receive a retryable service error. These controls are per server process; run only one server per database and workspace root.

`GET /api/health` reports process liveness; `GET /api/ready` checks database connectivity and scheduler admission. Run recovery is conservative: missing command results or corrupt checkpoints require checking the workspace before continuing. See [the implementation report](docs/PROJECT_OPTIMIZATION.md) for limits and validation.

Startup enforces one server owner per canonical database and user-data root using OS file locks before migrations or recovery. Keep the lock files in place; the OS releases ownership when the process exits. Reliable locking on a local filesystem is required; this is not a distributed deployment protocol.

Optional conversation-title generation has a separate queue (one worker, sixteen admissions, deduplicated by conversation). Saturation can skip title generation without failing the main run. Shutdown cancels/drains both queues; title work cannot occupy foreground execution slots.

### Change review and request recovery

Successful tool file changes can be reviewed and individually reverted from chat. The server resolves the authenticated tool ledger, checks the current content hash, and refuses to overwrite later manual edits. A durable reversal journal prevents blind replay after an uncertain result. Preview/reversal supports files and baselines up to 512 KiB. This does not rewind a whole turn, shell commands, or the entire workspace. Regenerating a reply does not roll back files.

Message submission uses `Idempotency-Key`, committing the input, run and initial events together. `GET /api/agent-runs/by-request/:requestId` can recover an accepted submission after its response is lost. Resume markers are committed with the state transition and carry request identity so reconnects skip previous failed attempts. Total network loss still requires checking the conversation when connectivity returns.

Deleting a conversation returns HTTP 409 while its foreground run or related background task is active, including a canceled foreground execution still cleaning up. Storage serializes deletion with admission; deleted conversations cannot be resumed or receive new background AI tasks. Deletion does not undo workspace changes.

After a service restart, interrupted background AI tasks fail explicitly and require checking existing artifacts before creating another task. They are not automatically replayed without a durable checkpoint contract. Cancellation survives restart; recovery logs and state commit together. Fresh pending tasks remain eligible to execute.

### Resource and command limits

API bodies default to 2 MiB; file upload/create/write bodies allow 65 MiB, multipart chat messages 80 MiB and Skill uploads 27 MiB. Individual chat attachments and workspace reads/writes are limited to 64 MiB; model text reads and existing-text edits read at most 8 MiB, and Skill archives at most 26 MiB. Oversized content is rejected rather than silently truncated. Multipart parsing uses 8 MiB memory before temporary-file spooling.

Host commands receive an explicit runtime environment allowlist and a per-user command home. They do not inherit service API keys, admin passwords, proxy credentials or loader/Git injection variables. Host command execution still requires trusted users: it runs as the service OS identity and has no OS filesystem/network sandbox. Configure dependencies through the command environment's own home rather than relying on credentials in the service account's home.
