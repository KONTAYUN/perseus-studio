# Perseus Structure Standards

This document is the stable structure contract for Perseus. Future AI agents and developers must use these boundaries before doing structural refactors. The goal is not periodic rewrites; the goal is small maintenance inside known seams.

## Core Principle

Responsibility boundaries come first. Line count is an alarm, not a personal style preference.

When a file grows too large, split it by the responsibility it already contains. Do not invent a new architecture because a different agent prefers different names.

## Change Rules

- Refactors are zero-behavior unless the user explicitly asks for behavior changes.
- Keep public APIs, tool names, HTTP routes, JSON fields, SSE event names, CSS class names, and user-visible UI behavior stable during structure work.
- Thin facade files may compose modules and expose a stable API, but must not contain business detail.
- If a boundary itself needs to change, update this document, `ARCHITECTURE.md`, and `docs/PROJECT_HEALTH_SCORECARD.md` first, with the migration reason.
- Do not do scheduled big rewrites. Split only when a feature touches an area, a file exceeds budget, or responsibilities are mixed.

## File Budgets

- Frontend production JS/JSX: fewer than 450 lines.
- Backend ordinary production Go: fewer than 450 lines.
- CSS section files: fewer than 450 lines.
- Large integration tests and fixtures may exceed 450 lines, but production structure scores do not use test exceptions to hide runtime complexity.

## Frontend Boundaries

- React components render UI and own only local interaction.
- Markdown display belongs in shared `components/markdown`; editable Markdown belongs in `components/editor/richText` with a dedicated editor hook. Both consume Markdown strings. Rich editing emits Markdown into the existing autosave/hash-conflict pipeline; it must not introduce a second persistence format or save on mount. Unsupported Markdown constructs keep their source and use a preview/source fallback. Editor tab state owns per-file display mode so hiding a panel does not reset that preference; keyboard saves call the same save action as other manual saves.
- Balanced single-line HTML spans use an inline Tiptap container under `editor/richText`, with a shared pure recognizer under `markdown`. Preserve opening/closing wrappers as inert data, edit Markdown children, and never inject original HTML attributes into the DOM. Other unsupported extensions retain the preview/source fallback.
- Hooks/actions own state flow, async actions, and cross-component coordination.
- Settings use `SettingsWorkspace` for category navigation and `SettingsNavigation` for leave protection. Keep the workbench mounted while settings are visible so drafts and active runs survive. Each category registers one complete dirty/busy guard; `useSettingsEditor` owns provider drafts and submission state. Treat successful writes with failed list refreshes as committed writes, with an explicit refresh action. Long setting forms and price editing stay inline; short confirmations use `AppDialog` with `useModalFocus`. Do not restore inert state owned by another component when closing a dialog.
- Markdown diagnostics and repair prompts are pure utilities under `components/markdown`. A dedicated editor hook debounces diagnostics and coordinates explicit repair confirmation with existing chat submission. Before submission the editor save boundary must persist and revalidate the exact file snapshot; a conflict, failed save or changed target cancels submission. Diagnosis never rewrites content or treats supported extensions as errors.
- Utilities contain pure functions only.
- Chat stream merging belongs in chat stream utilities/actions, not render components.
- Running input queue semantics belong in chat state/actions: queued follow-up messages, pending steers, rejected steers, and committed steer messages must stay separate, following the Codex-style input queue documented in `HARNESS.md`.
- File tree selection, drag/drop, mutation, trash, search, and download behavior stay in their existing hook/action modules.
- Layout math and FLIP/animation helpers stay in layout modules.
- Chat Composer changes stay inside `src/components/chat/composer/`; the wrapper remains a facade.

## CSS Boundaries

`src/index.css` is an ordered stylesheet entrypoint only. CSS lives in:

- `src/styles/tokens.css`: reset, global variables, radius token overrides, app shell background.
- `src/styles/surfaces.css`: reusable surfaces, cards, overlays, panel glow, non-motion visual primitives.
- `src/styles/motion.css`: animation keyframes/classes, motion primitives, shared interactive control motion.
- `src/styles/controls.css`: shared interactive control geometry and states.
- `src/styles/change-review.css`: bounded inline change preview and recovery controls.
- `src/styles/chat.css`: chat messages, assistant progress, recovery notices, and chat-only message surfaces.
- `src/styles/chat-composer.css`: composer shell, input, bottom action bar, model status, and scroll-to-bottom control.
- `src/styles/chat-composer-queue.css`: pending input queue, running steer previews, and follow-up question panels above the composer.
- `src/styles/editor.css`: editor tabs, editor textarea, editor drop states.
- `src/styles/markdown.css`: shared Markdown typography, tables and copyable code blocks.
- `src/styles/rich-editor.css`: document editing surface, selection toolbar and block controls.
- `src/styles/file-tree.css`: file tree rows, file labels, inline rename/drop indicators.
- `src/styles/settings.css`: settings modal/card/view-specific transitions.
- `src/styles/utilities.css`: narrow global utility overrides that do not fit a domain.
- Shared scrollbar styling belongs in `utilities.css`, covering app surfaces and portalled overlays in both themes; avoid inline App stylesheet fragments or per-menu omissions. Preserve intentional hidden tab scrollbars.

Composer visual contract: Codex-style bottom action bar inside the composer shell, with attachments on the lower left, model/reasoning status near the lower right, and send/stop on the far lower right. Text and attachment previews sit above the action bar. No Pretext/contenteditable/mirror layer and no dynamic text-measurement avoidance logic.

## Backend Boundaries

2026-10-20 design-audit implementation: assistant prose is display data, never an implicit edit transport. Structured tool calls own execution. Completion uses explicit outcomes and verified events; local failures preserve typed error identity across runner/ledger/API boundaries. Provider capabilities use explicit configuration with conservative defaults, never display-name inference. Optional path scope is an explicit run policy, independent of reference attachments. Command execution defaults closed and trusted-host mode requires explicit operator enablement and administrator role; this mode is not a sandbox. Workspace owns durable patch recovery journals and bounded, cancelable command change observation. Frontend stream utilities distinguish active merging from terminal authoritative reconciliation; existing chat facade/action interfaces remain; full session reducer migration is deferred. These are authorized behavioral changes, with compatibility migrations tested in isolation.

2026-10 reliability refinement: `runqueue` owns bounded admission, per-user serialization, cancellation, and draining. API handlers submit work through a run lifecycle facade instead of starting untracked goroutines. Workspace change verification and recovery remain separate from provider adapters. Shared frontend motion helpers own reduced-motion preferences and interruptible animation resources; render functions must not schedule timers or mutate animation bookkeeping.

- `api`: HTTP concerns only: auth/user extraction, request decoding, response shape, SSE/run handler orchestration.
- `store`: SQLite persistence only: schema, CRUD, scans, ledger/checkpoint storage.
- `ai/provider_*`: protocol adapters and stream parsing only.
- `ai/context_*`: context building, estimation, pruning, freshness, and provider message conversion.
- `ai/agent_*`: agent loop, model turn, tool loop, completion/follow-up, recovery, runtime state.
- `ai/tool_*` and `ai/tools_exec_*`: tool registry, policy/scope, argument ledger, event emission, verifier, concrete tool implementations.
- `workspace`: filesystem operations, search, trash, ordering, command execution.

Command subprocess environment construction belongs in `workspace/command_environment.go`: inherit a small runtime-variable allowlist, assign a per-user command home, and exclude service credentials. This reduces ambient credential inheritance; it is not an OS sandbox or a filesystem permission boundary.

Startup ownership belongs in `store/runtime_lock*`: acquire OS file locks for the canonical database and user-data root before migrations, stale-run recovery or workers start. File locks enforce the existing single-server contract, rather than pretending an in-memory queue coordinates replicas.

Optional conversation-title inference uses a separate bounded queue owned by the API server (one worker, sixteen admissions, one job per conversation). It cannot consume foreground execution slots, and shutdown cancels/drains both queues. Frontend title refresh owns its timers and abort signal in a dedicated hook; listing conversations must not clear another run's error/recovery state.

History removal belongs in the existing store lifecycle boundary: check active foreground runs and related background tasks in the deletion transaction. Background task admission checks conversation ownership in its transaction, and resume requires a surviving conversation. The API additionally retains history until a canceled registered execution owner has drained.

Startup task recovery also belongs in store: interrupted AI/unknown tasks become explicit failures requiring artifact verification; only side-effect-free mock tasks may be requeued automatically. Recovery logs and states commit together, and cancellation survives restart. Background tasks do not yet have the foreground run checkpoint contract.

Tool concepts must stay separate: registry, policy/scope, argument normalization, durable ledger, runner, event emitter, verifier, and result budget.

## Split Procedure

1. Read this file, `ARCHITECTURE.md`, and `HARNESS.md`.
2. Identify the existing responsibilities in the target file.
3. Move code into same-package files first; avoid import churn unless there is a clear boundary win.
4. Keep names/signatures stable unless the public contract is explicitly changing.
5. Add or preserve focused tests for extracted pure logic.
6. Run `npm run test:structure` plus the relevant unit/build/backend checks.

## Structure Gate

`npm run test:structure` enforces this contract:

- frontend production JS/JSX budget,
- backend production Go budget,
- CSS entrypoint and section budgets,
- composer anti-regression patterns,
- Pretext residue ban.

If the gate fails, fix the boundary or document a deliberate exception before continuing.


### 2026-10-20 admission refinement

Conversation send and regeneration share `store` transactional run preparation (message, run, initial events, request identity) and the API run lifecycle facade. Regeneration truncates the assistant tail only after scheduler admission, immediately before execution. Frontend request controls own cancellation through the submission-to-stream handoff; API request deadlines remain separate from durable run lifetimes. This closes the synchronous regeneration bypass without introducing a second agent runner.


### Verified change review and reversal

`workspace` owns bounded change previews and hash-checked reversal of one persisted tool change. `api/workspace_changes` resolves the authenticated user's tool ledger entry and schedules reversals through the shared workspace queue. A durable reversal journal prevents replay after an ambiguous filesystem/database failure. Chat renders this through a dedicated change-review component; it does not trust client-supplied paths, hashes or snapshots. This is per-file tool-change recovery, not a shell-command or whole-workspace checkpoint.
