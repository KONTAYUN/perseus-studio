# Perseus Studio Architecture

Single-container web IDE: React frontend + Go backend, served from one binary. Multi-user with per-user data isolation.

## Directory Layout

```
src/                        React frontend (Vite + Tailwind)
  components/
    layout/                 AppHeader, MainPanels
    sidebar/                SidebarPanel, FileTree
    editor/                 EditorPanel
    chat/                   ChatPanel
    tasks/                  TaskPanel
    settings/
      SettingsModal.jsx     Main settings panel (model/search providers, skills)
      SettingsUI.jsx        Shared UI primitives (Badge, Card, Field, etc.)
      UserManagement.jsx    Admin user management panel
  hooks/                    All state management (no Redux)
    useIdeState.js          Top-level orchestrator, composes sub-hooks
    useEditorTabs.js        Tab open/close/switch, dirty tracking
    useFileTree.js          File CRUD, drag-drop, upload, 回收站
    useChatState.js         Conversations, messages, SSE streaming
    useTasks.js             Background task polling
    useLayoutState.js       Panel sizing, drag ordering
    useSettingsData.js      Model/search provider config
    useSkills.js            Skill CRUD
    ideUtils.js             Shared utility functions
  api/
    client.js               Fetch wrapper for all /api/* calls

backend/
  cmd/server/               Entrypoint (main.go)
  internal/
    config/                 Env-based configuration
    store/                  SQLite persistence
      db.go                 Schema init, connection, migration helpers
      users.go              User CRUD, role management, registration toggle
      configs.go            Model & search provider CRUD (per-user)
      conversations.go      Conversation + message storage (per-user)
      tasks.go              Task state machine (per-user)
      skills.go             Skill metadata (per-user)
    usersvc/                Per-user service factory
      factory.go            Caches workspace/skills services per user, manages user data dirs
    api/                    HTTP layer (net/http)
      server.go             Router registration, static file serving, CORS
      auth_handlers.go      Registration, login, logout, /me endpoint
      admin_handlers.go     Admin user management, settings, session invalidation
      file_handlers.go      Workspace file operations
      conversation_handlers.go  Conversation API, agent run creation, SSE lifecycle
      task_handlers.go      Task CRUD + SSE streaming
      skill_handlers.go     Skill install/upload/CRUD
      config_handlers.go    Model & search provider management
      debug_handlers.go     Admin-only debug endpoint, disabled by default
      api_utils.go          Attachment processing helpers
    ai/                     LLM integration (OpenAI & Anthropic protocols)
      service.go            Entry point, generateReply routing
      provider_openai.go    OpenAI chat completion
      provider_anthropic.go Anthropic messages API
      provider_types.go     Shared response types
      messages.go           Message format conversion
      message_attachments.go  Attachment -> prompt assembly
      tool_schemas_*.go     Tool definitions per category
      tool_choice.go        Automatic tool selection logic
      tools_executor.go     Tool call dispatcher
      tools_exec_*.go       Tool implementations per category
      workspace_intent.go   Intent classification for tool routing
      web_http.go           HTTP client for web search/extract
      web_tavily.go         Tavily provider
      web_brave.go          Brave provider
      fallback.go           Error recovery & retry
      prompts.go            System prompt construction
      debug.go              Debug test orchestration
      debug_openai.go       OpenAI debug path
      debug_anthropic.go    Anthropic debug path
    workspace/              File system operations, tree ordering, trash, command execution
    tasks/                  Background task broker & worker
    skills/                 Skill zip install, SKILL.md parsing
    version/                Build-time version metadata
```

## Key Flows

### User Authentication

1. First user registers via `/api/auth/register` and is auto-promoted to `admin`
2. Login via `/api/auth/login` creates an in-memory session; `perseus_session` cookie returned
3. All subsequent API calls authenticated via session cookie middleware
4. `/api/auth/me` returns current user info including role and registration-open status

### Multi-User Data Isolation

1. `usersvc.Factory` provides per-user `workspace.Service` and `skills.Service` instances
2. Each user's files stored under `<USER_DATA_ROOT>/<user_id>/workspace/`
3. All DB queries (configs, conversations, tasks, skills) filtered by `user_id` column
4. API middleware extracts `userID` from session and passes it to all service calls

### Agent Chat Run

1. Frontend `useChatState` POSTs to `/api/conversations/{id}/messages` (multipart or JSON)
2. `conversation_handlers.go` stores the user message, creates an `agent_run`, and returns the run id
3. Frontend opens `/api/agent-runs/{id}/stream` with `EventSource`
4. Backend calls `ai.Service.GenerateReply`, which runs an OpenAI/Anthropic agent loop
5. Provider streaming deltas emit `assistant_message_started`, `assistant_message_delta`, and `assistant_message_completed`
6. Tool calls run through `tools_executor.go` and emit `tool_call_started`, `tool_call_completed`, and workspace change events
7. Successful runs end with `run_completed`; failed/cancelled runs emit terminal run status events
8. Frontend renders real assistant messages and uses a temporary thinking shell only as a waiting sidecar

### File Operations

All workspace files live under each user's workspace directory. Deletes move to `回收站/` (soft delete). The file tree is served as nested JSON from `/api/files/tree`, with optional per-folder manual order persisted in SQLite.

### Skill System

Skills are zip archives containing a `SKILL.md` front-matter file. Upload via `/api/skills/upload`, or install from a chat attachment. Skills are stored per-user and can be enabled/disabled per conversation.

### Background Tasks

`tasks.Manager` polls the DB for pending tasks. `tasks.Broker` provides pub-sub for SSE streaming. Task state: pending -> running -> done/failed/cancelled.

### Admin Operations

Admin-only APIs under `/api/admin/` allow user management (CRUD, role changes, disable/enable, password reset) and system settings (registration toggle). Session invalidation is triggered on role change, disable, and delete. The last admin cannot be demoted, disabled, or deleted.

## Data Persistence

All state lives in `/data/` (Docker volume mount):
- `app.db` — SQLite database (users, tasks, conversations, configs, skills metadata)
- `users/<user_id>/workspace/` — Per-user workspace files
- `users/<user_id>/uploads/` — Per-user chat attachment blobs
- `users/<user_id>/skills/` — Per-user installed skill directories

## Debug API

`/api/debug/model-tool-test` exercises the full tool-calling round-trip against configured model providers. It is disabled by default with `ENABLE_DEBUG_API=false` and requires an admin session when enabled.

## Project Hygiene

Runtime data belongs in `data/`. Local exports, binary builds, and deployment packages belong in `_local_artifacts/` and are ignored by Git and Docker build context.
