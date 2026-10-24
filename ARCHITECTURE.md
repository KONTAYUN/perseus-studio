# Perseus Studio Architecture

Document images: `DocumentImagePicker` provides upload and searchable workspace thumbnails without a URL-entry field. `/api/documents/images` validates bitmap bytes and stores editor uploads under the hidden `.perseus-images/` workspace directory; `/api/files/image` remains the authenticated display boundary. Workspace owns persisted leases and conservative reference collection; the usersvc worker follows server cancellation. Legacy UUID-prefixed `图片/` uploads remain readable and are hidden individually; ordinary user image files remain visible and are never collected. See `docs/DOCUMENT_IMAGES_20261024.md`.

Document editing closure: Markdown remains the sole content source. The document adapter/session owns remark/mdast conversion, root source mapping, revision-aware candidate checks and ProseMirror transactions. Backend document policy uses Goldmark and shared versioned fixtures. Writes retain hash conflicts, snapshots and atomic replacement. UI distinguishes preserved content, confirmed conversion problems and resource failures. See docs/DOCUMENT_EDITING_IMPLEMENTATION.md for staged evidence.

Design-audit migration (2026-10-20): only structured tool calls execute actions; completion consumes explicit outcomes and verified events. Error identity is retained across AI/API boundaries. Provider capability settings are explicit; reference attachments do not grant or restrict permissions. Workspace owns persistent patch recovery and bounded change observation; host commands are disabled unless explicitly enabled for trusted administrators. Chat utilities own authoritative terminal reconciliation. A full session reducer migration remains pending; existing facade/action interfaces are retained. These changes refine existing packages; the detailed migration and evidence are recorded in `docs/DESIGN_AUDIT_REMEDIATION.md`.

Single-container web IDE: React frontend + Go backend, served from one binary. Multi-user with per-user data isolation.

## Directory Layout

Markdown format checks are conservative, bounded frontend diagnostics. The editor header offers one icon to switch rich/source views and a warning icon only for suspected issues. Confirmed repair reuses chat submission with a dedicated formatting prompt; it first saves and validates the selected file snapshot through existing editor save actions. Local conflicts, save errors and target changes prevent submission. Scrollbar visuals are shared across application and portalled surfaces.

Markdown messages share a safe React Markdown renderer with GFM and copyable fenced code. Markdown documents can switch between source and a Tiptap document editor, using Markdown as the sole persisted format. The rich editor owns selection/block controls and local document transactions; existing editor hooks retain file identity, per-file display mode, autosave and conflict ownership. Keyboard saves reuse the manual save action. Merely opening a document must not rewrite it. Unsupported document syntax remains available in inert local preservation nodes, with editable surrounding prose and explicit local controls.

```
src/                        React frontend (Vite + Tailwind)
  components/
    ui/                     Shared UI primitives and control standards
    layout/                 AppHeader, MainPanels
      mainPanels/           Panel width math, active glow, FLIP/appear motion helpers
    sidebar/                SidebarPanel, FileTree
    editor/                 EditorPanel
    chat/                   ChatPanel
      composer/             Autosize textarea, fixed action dock, attachments/references, skill menu
      panel/                Chat header menus, context usage badge, scroll controller, rename dialog
    tasks/                  TaskPanel
    settings/
      SettingsModal.jsx     Main settings panel (model/search providers, skills)
      SettingsUI.jsx        Settings aliases for shared UI primitives
      UserManagement.jsx    Admin user management panel
  hooks/                    All state management (no Redux)
    useIdeState.js          Top-level orchestrator, composes sub-hooks
    useEditorTabs.js        Editor tab facade: open/close/switch, dirty tracking
    createEditorFileLoader.js Editor file read/open loader
    editorToolRefreshActions.js Refresh open tabs after agent workspace tools
    useFileTree.js          File tree facade: state composition and public API
    useFileSearchState.js   File search state and debounced query API
    useFileSelectionState.js File selection and range/multi-select state
    useFileDragState.js     File drag/drop state
    useFileTreeTrashActions.js File trash panel and restore/delete actions
    fileTreeMutationActions.js File CRUD/upload/reorder action group
    fileTreeDownloadActions.js File download action group
    fileTreeDownloadUtils.js File download and zip helpers
    useChatState.js         Chat state facade: send queue and public API
    chatConversationActions.js Conversation CRUD and context usage loading
    useChatReferenceActions.js Chat attachments, workspace refs, and Skill refs
    chatCompactionActions.js Context compaction workflow
    chatRegenerateResumeActions.js Regenerate and checkpoint-resume actions
    useChatLifecycleEffects.js Chat message/ref notice lifecycle effects
    chatRunStream.js        Agent run SSE/poll fallback merge logic
    useTasks.js             Background task polling
    useLayoutState.js       Panel sizing, drag ordering
    useSettingsData.js      Model/search provider config
    useSkills.js            Skill CRUD
    ideUtils.js             Shared utility functions
  api/
    client.js               Fetch wrapper for all /api/* calls
  styles/                   Ordered CSS sections imported by index.css
    tokens.css              Reset, app shell, variables, radius overrides
    surfaces.css            Reusable surfaces and low-depth material classes
    motion.css              Shared animation and interaction primitives
    controls.css            Shared button/icon/segmented/switch rules
    change-review.css       Inline file change review and reversal
    chat.css                Chat messages and chat progress surfaces
    chat-composer.css       Composer shell, input, action bar, model status, scroll-to-bottom control
    chat-composer-queue.css Pending input, running steer, and follow-up panels above the composer
    editor.css              Editor tabs/drop/textarea rules
    file-tree.css           File tree rows, labels, drag indicators
    settings.css            Settings modal/card/view animation rules
    utilities.css           Narrow global utility overrides

backend/
  cmd/server/               Entrypoint (main.go)
  internal/
    config/                 Env-based configuration
    store/                  SQLite persistence
      db.go                 Schema init, connection, migration helpers
      users.go              User CRUD, role management, registration toggle
      configs.go            Config store types/schema facade
      config_*              Model/search provider CRUD, validation, scanners
      conversations.go      Conversation store types/schema/facade
      conversation_*        Message CRUD, tool-call ledger, scanners
      tasks.go              Task store types/schema/facade
      task_*                Task CRUD, logs, JSON/scanner helpers
      skills.go             Skill metadata with public/private visibility
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
      config_handlers.go    Config request/response types
      config_*              Model provider CRUD, discovery/test, search provider CRUD
      debug_handlers.go     Admin-only debug endpoint, disabled by default
      api_utils.go          Attachment processing helpers
      conversation_run_*    Run execution, assistant segments, task completion, failure handling
    ai/                     LLM integration (OpenAI & Anthropic protocols)
      service.go            Entry point, generateReply routing
      provider_openai.go    OpenAI chat completion
      provider_anthropic.go Anthropic messages API
      provider_stream_*     SSE reader, OpenAI/Anthropic stream parsers, usage/fallback helpers
      provider_types.go     Shared response types
      messages.go           Message format conversion
      message_attachments.go  Attachment -> prompt assembly
      tool_schemas_*.go     Tool definitions per category
      tool_choice.go        Automatic tool selection logic
      agent_entrypoints.go  OpenAI/Anthropic agent runtime setup
      agent_model_turn.go   Provider turn request, context overflow retry
      agent_recovery.go     Checkpoint resume retry after transient provider failures
      agent_tool_loop.go    Tool loop, mutation de-dupe, tool validation
      agent_completion.go   attempt_completion validation and verified paths
      agent_followup.go     ask_followup_question workflow tool
      agent_no_tool.go      Missing-tool/empty-response correction loop
      tools_executor.go     Durable tool execution ledger and event emission
      tool_registry.go      Tool name -> implementation registry
      tool_arguments.go     Tool argument summaries and ledger truncation
      tool_recovery_hints.go Tool failure recovery hints
      tool_events.go        Tool/run event helpers
      context_*             Context build, pruning, transcript conversion, summary/estimation
      tools_exec_*.go       Tool implementations per category and workspace operation family
      workspace_intent.go   Intent classification for tool routing
      web_http.go           HTTP client for web search/extract
      web_tavily.go         Tavily provider
      web_brave.go          Brave provider
      fallback.go           Error recovery & retry
      prompts.go            System prompt construction
      debug.go              Debug test orchestration
      debug_openai.go       OpenAI debug path
      debug_anthropic.go    Anthropic debug path
    workspace/              File read/write/path operations, search, tree ordering, trash, command execution
    tasks/                  Background task broker, worker lifecycle, AI/mock execution, validation
    skills/                 Skill CRUD, zip install, safe extraction, SKILL.md parsing
    version/                Build-time version metadata
```

## Key Flows

Reliability refactor (2026-10): foreground runs enter a bounded `runqueue` through the API lifecycle facade. The queue serializes runs for each user, exposes cancellation before execution starts, and drains on shutdown. UI motion is centralized around shared duration tokens, reduced-motion preference, and cancellable animation ownership; panel focus decoration stays attached to its panel.

### User Authentication

1. First user registers via `/api/auth/register` and is auto-promoted to `admin`
2. Login via `/api/auth/login` creates a SQLite-backed session; `perseus_session` cookie returned. Sessions survive server recreation.
3. All subsequent API calls authenticated via session cookie middleware
4. `/api/auth/me` returns current user info including role and registration-open status

### Multi-User Data Isolation

1. `usersvc.Factory` provides per-user `workspace.Service` and `skills.Service` instances
2. Each user's files stored under `<USER_DATA_ROOT>/<user_id>/workspace/`
3. Workspace files, conversations, tasks, and uploads remain user-scoped. Model providers, search providers, and Skills are admin-managed resources with `private`/`public` visibility: private resources are admin-only, public enabled resources are usable by ordinary users.
4. API middleware extracts `userID` from session and passes it to all service calls

File edits use `apply_workspace_patch`, exposed as an ordinary function tool on both supported protocols. Assistant prose and fenced patch examples are display-only; only structured tool calls execute edits. Tool results include every changed path and move/trash mapping so all editor tabs can synchronize. Editor saves send `expected_hash`; the workspace service compares and writes under one per-user write lock and returns HTTP 409 for stale edits. This lock coordinates content writes inside one service process, not shell commands or multiple server replicas. Cross-origin browser requests require the same request host or an explicit `CORS_ALLOWED_ORIGINS` entry.

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

Skills are zip archives containing a `SKILL.md` front-matter file. Upload via `/api/skills/upload`, or install from a chat attachment. Skill installation and management is admin-only. Enabled public Skills are visible/usable by ordinary users; private Skills remain admin-only.

### Background Tasks

`tasks.Manager` polls the DB for pending tasks. `tasks.Broker` provides pub-sub for SSE streaming. Task state: pending -> running -> done/failed/cancelled.

### Admin Operations

Admin-only APIs under `/api/admin/` allow user management (CRUD, role changes, disable/enable, password reset) and system settings (registration toggle). Session invalidation is triggered on role change, disable, and delete. The last admin cannot be demoted, disabled, or deleted.

## Data Persistence

All state lives in `/data/` (Docker volume mount):
- `app.db` — SQLite database (users, tasks, conversations, configs, skills metadata)
- `users/<user_id>/workspace/` — Per-user workspace files
- `users/<user_id>/uploads/` — Per-user chat attachment blobs
- `users/<user_id>/skills/` — Installed Skill directories owned by the installing admin/user record; DB visibility controls whether enabled Skills are shared

## Debug API

`/api/debug/model-tool-test` exercises the full tool-calling round-trip against configured model providers. It is disabled by default with `ENABLE_DEBUG_API=false` and requires an admin session when enabled.

## Project Hygiene

Runtime data belongs in `data/`. Local exports, binary builds, and deployment packages belong in `_local_artifacts/` and are ignored by Git and Docker build context.

## Frontend UI Governance

Markdown rich editing supports balanced single-line HTML spans through a source-preserving inline Tiptap node. Its recognizer is shared with the local preservation tokenizer. HTML wrappers remain inert data while child text/marks remain editable; block HTML, frontmatter and other extensions use local preservation nodes rather than locking the entire document. Both modes retain the existing Markdown save/conflict pipeline.

Frontend UI changes should follow [docs/UI_DESIGN_SYSTEM.md](docs/UI_DESIGN_SYSTEM.md). That document is the source of truth for UI tokens, button/input/menu/dialog standards, panel headers, chat composer behavior, file tree interaction, animation rules, dark-mode parity, and future component extraction.

Do not add new one-off Tailwind styling for common controls when an existing shared utility or primitive can be used.

`npm run test:ui` enforces the lightweight UI governance rules in `scripts/check-ui-standards.mjs` and is part of CI. `npm run test:structure` enforces file budgets, CSS section shape, and structure anti-regressions.

## Project Health Governance

The stable structure standard is [docs/STRUCTURE_STANDARDS.md](docs/STRUCTURE_STANDARDS.md). The active score and remaining risks are tracked in [docs/PROJECT_HEALTH_SCORECARD.md](docs/PROJECT_HEALTH_SCORECARD.md).

Key rules:

- Refactors are zero-behavior unless a task explicitly asks for behavior changes.
- Ordinary business files target fewer than 450 lines. Exceptions must be documented in the scorecard.
- Frontend state machines and algorithms belong in hooks/utilities, not large render components.
- Backend agent runtime follows mature agent boundaries: provider turn, recovery, tool loop, registry, ledger/events, completion, and follow-up question are separate modules.
- Do not periodically re-split the project by taste. Continue within the boundaries in `docs/STRUCTURE_STANDARDS.md`; changing a boundary requires updating the standard and scorecard first.


### 2026-10-20 admission refinement

Conversation send and regeneration share `store` transactional run preparation (message, run, initial events, request identity) and the API run lifecycle facade. Regeneration truncates the assistant tail only after scheduler admission, immediately before execution. Frontend request controls own cancellation through the submission-to-stream handoff; API request deadlines remain separate from durable run lifetimes. This closes the synchronous regeneration bypass without introducing a second agent runner.


### Verified change review and reversal

Host command subprocesses receive an explicit runtime environment allowlist and a per-user command home. Service API keys, auth/admin configuration, proxy credentials, Git configuration injection, and loader hooks are not inherited. Commands still run with the service OS identity; filesystem/network containment requires a separate worker sandbox and remains an open deployment boundary.

Server startup acquires OS file locks on the canonical database path and user-data root before migrations or recovery. Locks remain open until process shutdown and are released by the OS after a crash; the lock files are retained to avoid unlink/recreate races. This enforces single-process ownership on supported local filesystems. It does not provide distributed locking over filesystems without reliable OS locks.

Conversation-title generation is optional auxiliary work: the API owns a separate one-worker/sixteen-admission queue, deduplicates by conversation, passes shutdown cancellation into provider requests, and drains it before releasing storage. Foreground workspace execution has its own queue. The frontend title-refresh hook cancels timers/requests on replacement or unmount and stops polling when a title arrives; background list refresh never clears current run recovery state.

Conversation deletion is serialized with foreground/background admission in SQLite: active work prevents deletion, background AI tasks require an owned existing conversation, and resume cannot revive deleted history. The API also checks registered foreground owners, including canceled executions still draining. This preserves the existing store/API responsibilities without adding a second lifecycle system.

Background task startup recovery is conservative: cancellation remains canceled, interrupted AI/unknown work fails with an explicit artifact-verification requirement, and only mock jobs can requeue automatically. Recovery state and logs are atomic. Full background task checkpoint/resume remains unimplemented.

`workspace` owns bounded change previews and hash-checked reversal of one persisted tool change. `api/workspace_changes` resolves the authenticated user's tool ledger entry and schedules reversals through the shared workspace queue. A durable reversal journal prevents replay after an ambiguous filesystem/database failure. Chat renders this through a dedicated change-review component; it does not trust client-supplied paths, hashes or snapshots. This is per-file tool-change recovery, not a shell-command or whole-workspace checkpoint.

### Extended Markdown editing — 2026-10-23

Unsupported Markdown is represented by inert block/inline preservation nodes rather than a whole-document read-only preview. Raw content never becomes executable DOM. Local editing and insertion use normal editor transactions, including undo. Long documents use source-preserving sections to bound the mounted rich editor. Opening or navigating sections does not write files.
