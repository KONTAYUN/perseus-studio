# Perseus Studio Project Health Scorecard

This scorecard records verifiable engineering evidence. The release target is **9.0 / 10 or better**, but a score is not published unless the listed runtime gates have passed for the same revision.

## Current Score

Baseline at commit `e45afc5`: **8.6 / 10**.

Current candidate: **9.2 / 10 (local deterministic evidence, 2026-08-10)**.

The candidate score describes the current working tree on top of `e45afc5`.
Remote CI, the scheduled 30-minute soak, and live-provider evaluation remain
separate release evidence and cannot be inferred from local success. The score
must be attached to a commit and rerun by CI before it is a published release
score.

| Gate | Status | Evidence required |
|---|---|---|
| Single Agent runtime | proven | One `kernel_v3 + runTurnEngine` production path; legacy data is read-only migration input. |
| Transactional terminal event | proven | Store/API concurrency and idempotency tests for `FinalizeRunWithEvent`. |
| HTTP and container security | proven | CORS, CSRF, trusted proxy, body-limit and rate-limit tests; image runs as UID/GID 10001. |
| Persistent multi-user scheduling | proven | Unit tests plus two Docker load runs. The primary run completed 100/100 Runs for 20 users with provider peak concurrency 4; the restart run completed 10/10 without duplicate mutation. |
| Browser Agent lifecycle | proven | 46 Docker Playwright scenarios passed and one optional live-provider case skipped. Coverage includes offline preview recovery, slow multi-image persistence, three provider stream protocols, queue/commentary, replay, 429, EOF, note edit/one history, user input/choice, and workspace UI. |
| Release image | proven | The tested image reports health/readiness 200, runs as UID/GID 10001, and reports SQLite `integrity_check=ok`. Normal-data replacement is verified separately at the end of local rollout. |

## Weighted Evidence

| Area | Weight | Score | Evidence | Remaining risk |
|---|---:|---:|---|---|
| Agent architecture and maintainability | 2.0 | 1.8 | One `kernel_v3 + runTurnEngine` production path, canonical `ToolManifest`, typed item history, old runtime paths removed from execution. | Several intentionally broad orchestration files remain under responsibility review. |
| Event, data, and recovery integrity | 2.0 | 1.9 | Transactional terminal event/barrier, shared durable Run/Task tailer, contiguous SSE replay, offline preview recovery, restart recovery, 10,000-event and concurrent finalizer tests. | Single-instance SQLite is an explicit deployment boundary. |
| HTTP, authentication, and operations | 2.0 | 1.8 | Same-origin default, CSRF/CORS tests, trusted proxy policy, rate limits, bounded bodies/timeouts, non-root image, readiness, explicit permission repair, inspect/archive/vacuum tools, and Compose limits. | In-memory abuse limits reset on process restart by design; archive application is intentionally an operator action. |
| Product correctness and resilience | 2.0 | 1.9 | Browser fault injection proves preview reconnect, final authoritative handoff, reading-anchor retention, no duplicate history, slow multi-image persistence, and protocol-equivalent provider behavior. | Real third-party gateways can still deviate from their declared protocol. |
| Automated verification and delivery | 2.0 | 1.8 | Full Go suite, 157 frontend unit tests, UI/structure gates, production build, 46 passing Playwright cases, 100-Run load gate, release evidence JSON, and nightly soak workflow. | Remote CI, the scheduled soak, and optional live-provider eval have not run for this working tree. |

## Optimization Baseline

This project is optimized by runtime and maintenance impact, not by deleting lines.

| Area | Baseline | Optimization Direction |
|---|---|---|
| Frontend build | Before this pass, Vite emitted one main JS chunk at **1,283.84 kB** minified / **387.27 kB** gzip. Current release verification emits a **348.48 kB** / **102.55 kB** entry, with **363.36 kB** editor, **367.66 kB** general vendor, and **141.99 kB** React chunks. | Keep editor/main layout on the first screen, lazy-load low-frequency overlays, and use stable vendor chunks for React, Tiptap, icons, and general vendor code. |
| Test maintainability | Chat run stream tests repeated EventSource/window setup and `waitForAgentRunStream` boilerplate in each case. AI integration tests repeated request payload decode setup. | Move repeated test harness work into package-local/unit-test helpers while preserving every scenario assertion. |
| Runtime rendering | Resize and active panel behavior already avoid React state updates on every drag frame. Chat Markdown, message bubbles, and code blocks already use memo boundaries. | Keep memo limited to proven repeated render boundaries; avoid broad memo wrappers that add complexity without reliable wins. |

| Area | Status | Evidence | Remaining Risk |
|---|---|---|---|
| Structure standard | proven | `docs/STRUCTURE_STANDARDS.md` treats 450 lines as a review threshold, not a hard split rule. `npm run test:structure` requires a responsibility review entry for threshold crossings. | Future agents must not reintroduce mechanical line-count splits. |
| Frontend boundaries | proven | Rich text editor behavior is split by responsibility. File tree, chat messages, and rich text history now have smaller render entries with row/message/history helpers separated. | Keep new UI behavior in the extracted components instead of growing entry files again. |
| CSS boundaries | proven | `src/index.css` is an ordered entrypoint. Rich text styling is split into editor base, toolbar shell, block trigger, selection toolbar, table toolbar, format controls, menus, history, content, image, and table sections. Toast visuals are isolated in `src/styles/toast.css`; shared motion semantics stay in `src/styles/motion.css`. | Future toolbar work should stay in the matching control-family section. |
| Backend import/export | proven | DOCX import/export facades delegate to style types, extraction, Word XML, and zip helpers. File and note transfer handlers now split by route responsibility without changing API behavior. | Keep HTTP decoding in handlers and conversion/filesystem behavior in helpers/services. |
| Agent kernel | proven | `docs/AGENT_KERNEL.md` records the single v3 execution model: bounded context, targeted note reads, typed items, ledgers and checkpoint recovery. | New execution paths must not reintroduce runtime mode switches or text-derived state. |
| UI system | proven | `docs/UI_DESIGN_SYSTEM.md` documents rich text editor control exceptions. `npm run test:ui` continues to block raw one-off buttons, shadows, radius drift, and broad motion. | Editor-specific controls must stay in named CSS classes, not long JSX class strings. |
| Old-code audit | partial | `docs/LEGACY_STRUCTURE_AUDIT.md` records the old-code review and decisions. Files over the 450-line review threshold are registered below with responsibility-based decisions. | Re-run the audit whenever a legacy compatibility path is removed or promoted. |

## Responsibility Review Register

These production files cross the 450-line review threshold or sit near it. A threshold crossing is allowed only when this table records the responsibility and the next correct maintenance action.

| File | Classification | Decision |
|---|---|---|
| `src/styles/motion.css` | Shared motion token mapping, keyframes, reduced-motion behavior, and compatibility motion classes. | Keep this as the single motion semantics registry. Move component-specific visual surfaces, such as toast styling, to their own CSS section instead of splitting keyframes by line count. |
| `backend/internal/ai/note_view_blocks.go` | HTML note fragment block indexing, section grouping, raw block capture, and compact metadata projection for `read_workspace_note_view`. | Keep together for now because parser/tokenizer/index metadata must evolve as one behavior. If it grows again, split by real boundary: raw-token capture, block/section indexing, and payload projection. |
| `backend/internal/ai/context_read_spans.go` | Current-read working set extraction, read-result span identity, stale/superseded read compaction, and model-facing read metadata for text and HTML note reads. | Keep together for now because the span identity rules must stay consistent across compaction, diagnostics, and kernel progress. If it grows again, split by real boundary: result decoding, supersession rules, and working-set projection. |
| `backend/internal/ai/context_assembler.go` | The only model-context assembly entry: active item selection, token estimation, old tool-result pruning, diagnostics, and compaction decision. | Keep orchestration here; keep protocol-specific projection in `context_protocol.go` and semantic compaction in `context_item_compaction.go`. |
| `backend/internal/ai/tool_manifest.go` | Canonical tool registry, provider/model visibility, transport and terminal-action metadata, model-facing action manifest, and tool diagnostic projection. | Keep one registry so visibility, execution protocol, recovery behavior, and diagnostics cannot drift across separate tables. If it grows again, split only by the real boundary between immutable registry data and manifest/diagnostic projection. |
| `backend/internal/ai/tool_schemas_workspace.go` | OpenAI and Anthropic schema projection for the same workspace tool contract, including targeted HTML-note creation and block editing. | Keep both provider projections beside the canonical workspace schema so required fields and operation enums cannot drift. Split only if a provider gains genuinely different schema semantics, not because the file crosses a line threshold. |
| `backend/internal/ai/agent_kernel.go` | Current Agent kernel state, read-target identity, typed turn item recording, and progress-ledger projection for the single v3 runtime. | Keep together because state normalization and target identity must remain one contract. Split only by a proven boundary between state normalization, item recording and read-target extraction. |
| `backend/internal/ai/agent_tool_loop.go` | Ordered handling of one model tool action: protocol validation, scope and write guards, strategy-ledger checks, execution, preview resolution, and result projection. | Keep the ordered action boundary together so early rejection and final execution cannot drift. Tool-specific parsing and execution remain in their dedicated files; extract only if a new independent lifecycle appears. |
| `backend/internal/ai/tools_exec_note_edit.go` | Atomic HTML note block mutation executor: target resolution, text-segment replacement, structural operations, sanitizer application, document-version checks, and compact mutation results. | Keep the execution contract together because preflight, operation semantics, and one-write/one-history verification must evolve atomically. If it grows again, split only by a proven boundary between operation application and post-write validation; keep the tool contract in one reviewed entry. |
| `backend/internal/ai/tools_exec_note_view.go` | HTML note index, section, block, and raw-view execution plus target coverage metadata and text-segment projection. | Keep the read-view contract together so index identity, raw coverage, block hashes, and section/batch payloads cannot drift. If it grows again, extract parser projection only after the read protocol remains a single tested boundary. |
| `backend/internal/store/agent_turn_items.go` | Canonical ordered agent history, legacy migration, item append/list, and interrupted tool-call repair. | Keep the table contract and migration together; move provider/context behavior out of the store package. |
| `backend/internal/store/conversations.go` | Conversation schema migration plus conversation, branch, summary, and selected-provider persistence operations. | Keep as the conversation aggregate store; new agent item history stays in `agent_turn_items.go`, and unrelated message/run behavior must not move back here. |
| `backend/internal/api/conversation_run_execute.go` | Conversation run lifecycle orchestration: assistant draft persistence, model event bridging, tool/progress event publishing, provider limit wait-and-resume, and final run completion. | Keep as the run coordinator for now so cancellation, persistence errors, and run status transitions stay in one reviewed path. If it grows again, split by real boundary: assistant message persistence, provider wait/retry orchestration, and event-to-run-stage mapping. |
| `backend/internal/api/agent_run_handlers.go` | Agent Run control routes plus the ordered SSE tailer: cursor parsing, durable replay, preview snapshot recovery, terminal barrier, cancel/resume and follow-up routing. | Keep the Run protocol boundary together while event persistence remains in the store and model execution remains in the run coordinator. If it grows again, extract only the reusable durable SSE tailer shared with tasks, not arbitrary route fragments. |
| `backend/internal/store/agent_runs.go` | Agent Run schema, migration, lifecycle state, terminal sequence barrier and JSON projection. | Keep the Run aggregate schema and state transition contract together. Event append/replay stays in `agent_events.go`; split only if migration normalization becomes an independent migration package. |
| `backend/internal/store/agent_events.go` | Durable Agent Run event append/replay, terminal barriers, user-resume transitions, and restart requeue transitions. | Keep the event lifecycle transactions together so sequence allocation, idempotency, and status barriers cannot drift. Split only if a separately reusable event-log package emerges for both Run and Task streams. |
| `src/hooks/chatRunStream.js` | One event-stream consumer that owns event ordering, assistant draft lifecycle, current-message presentation updates, progress projection, and terminal run settlement. | Keep event consumption in one ordered state machine. Pure payload normalization remains in `chatRunMessageState.js` and `chatRunStreamToolEvents.js`; split only if a new independent protocol responsibility appears. |
| `src/hooks/useEditorTabs.js` | Editor-tab aggregate state, autosave coordination, authoritative file snapshots, conflicts, and transient AI edit previews. | Keep the tab aggregate in one hook while preview parsing and stream transport remain outside it. If preview interactions expand, extract a dedicated preview-state reducer rather than splitting by line count. |
| `src/components/settings/ModelProviderSettings.jsx` | Model provider CRUD form and the explicit adapter, authentication, tool, vision, and context capability controls that must be saved as one coherent provider configuration. | Keep the configuration transaction in one form for now. Reusable field controls stay shared; split only if provider discovery or capability-catalog editing becomes an independent workflow. |

## Completed Structural Cleanup

- `src/components/editor/richText/RichTextEditor.jsx` remains below the review threshold and acts as the composition entry for Tiptap state and callbacks.
- Rich text editor responsibilities are separated into extension assembly, HTML decoration/sanitizing, image/table extensions, image upload, document sync, toolbar lifecycle, block trigger, and render surface.
- DOCX rich-text style logic is split into style types, HTML extraction, Word XML patch/apply logic, DOCX zip helpers, and import/export facades.
- File handlers are split into read, history, write, trash, and shared error/workspace helpers.
- Note transfer handlers are split into export, import, and HTML fragment helper responsibilities.
- File tree, chat messages, and rich text history entries now delegate heavy row/message/export/history utility work to smaller components.
- Rich text toolbar CSS is split by control family: shell, block trigger, selection toolbar, table toolbar, format controls, and menus.
- `docs/LEGACY_STRUCTURE_AUDIT.md` records the old-code mechanical-split audit.

## Old-Code Audit

| Area | Result | Next Action |
|---|---|---|
| `src/hooks` | Existing hook splits are mostly responsibility-based: chat state, layout state, editor tabs, file tree actions, task state, and settings state. | Do not merge by size. When touching a hook, keep state orchestration separate from pure helpers. |
| `src/components` | Heavy UI entries were re-split where the improvement was clear: file tree row/actions, chat message bubble details, and history utilities/export behavior. | Prefer subcomponents for repeated render regions and hooks for cross-component effects. |
| `backend/internal/ai` | Agent/provider/context/tool files follow stable responsibility families. Context ownership is now explicit: persistence in `agent_turn_items`, assembly in `ContextAssembler`, provider conversion in `context_protocol.go`. | Preserve tool registry, policy, durable ledger, context assembly, and protocol serialization as separate concepts. |
| `backend/internal/api` | Broad handler groups were split by route responsibility: file read/history/write/trash and note import/export/helpers. | Keep new handler files as HTTP layers; do not move conversion logic back into them. |
| `backend/internal/workspace` | Workspace code remains service-oriented: filesystem operations, path helpers, history, trash, and ordering are separated. | Keep workspace helpers package-local unless reused by another package. |

## Verification Standard

For this score to remain valid, structural work must pass:

```bash
npm run test:ui
npm run test:structure
npm run test:unit
npm run build
cd backend && go test ./...
```

If local Go is unavailable on Windows, use the Docker equivalent:

```bash
docker run --rm -v "<repo>\\backend:/src/backend" -w /src/backend docker.m.daocloud.io/library/golang:1.25-bookworm go test ./...
```

Before a release handoff, also run `npm run test:e2e`, build the Docker image, and check both `/api/health` and `/api/ready`.
