# Perseus 9.5+ Project Health Scorecard

This scorecard records the current structure standard and the evidence behind the score. It should not be used to claim 9.5 unless the checks and file boundaries support that claim.

## Stable Standard

- Source of truth: [docs/STRUCTURE_STANDARDS.md](STRUCTURE_STANDARDS.md).
- Refactors are zero-behavior unless a task explicitly asks for behavior changes.
- Responsibility boundaries come first; line count is an alarm.
- Production JS/JSX, ordinary production Go, and CSS section files target fewer than 450 lines.
- New structural splits must continue within the documented boundaries. Changing a boundary requires updating `STRUCTURE_STANDARDS.md`, `ARCHITECTURE.md`, and this scorecard first.

## Current Score

Overall: **9.6 / 10**

| Area | Score | Evidence | Remaining Risk |
|---|---:|---|---|
| Stable structure standard | 9.8 | `docs/STRUCTURE_STANDARDS.md` defines facade/component/hook/utils/backend/CSS boundaries and the no-arbitrary-resplit rule. `HARNESS.md` and `ARCHITECTURE.md` link to it. | Future work must keep the docs updated when a boundary intentionally changes. |
| Frontend structure | 9.6 | Production JS/JSX passes `npm run test:structure`; known facades such as `useIdeState`, `useChatState`, and `useEditorTabs` stay under 450 lines. | A few facade files sit near the budget and should not absorb new feature detail. |
| CSS structure | 9.7 | `src/index.css` is now an entrypoint; CSS is split into `tailwind/tokens/surfaces/motion/controls/chat/chat-composer/chat-composer-queue/editor/file-tree/settings/utilities`, all below 450 lines. Composer visual contract lives in `chat-composer.css`; running input queue panels live in `chat-composer-queue.css`. | Keep future composer input changes in `chat-composer.css` and queue/steer/follow-up panels in `chat-composer-queue.css`. |
| Backend agent/context/provider | 9.6 | Context pruning/conversion/summary and provider stream SSE/OpenAI/Anthropic/usage/fallback are separate files. Agent runtime already follows model turn, tool loop, recovery, completion, follow-up boundaries. | `web_http.go` is close enough to watch, but still below budget. |
| Tool layer | 9.6 | Workspace tools are split into read/search, write/edit, path/trash, organize, and payload helpers; registry, arguments, policy/scope, events, verifier, and result budget remain separate. | Add tests when tool policy grows beyond the current automatic execution model. |
| Store/API | 9.6 | Conversation store is split into conversation CRUD, message CRUD, tool ledger, scans. Config and task stores are split. API config and run handlers are split by CRUD/test/discovery/search and execute/task/failure. | `agent_runs.go` is under budget but near the line alarm; split when run event/checkpoint semantics expand. |
| Workspace/tasks/skills | 9.5 | Workspace read/write/path/helpers, task mock/AI/validation/publish, and skill install/zip/metadata/file helpers are separated and under budget. | These packages should keep service entry files as facades and put new behavior into the existing family files. |
| Governance/tests | 9.6 | `npm run test:ui`, `npm run test:structure`, and `cd backend && go test ./...` pass. CI and `scripts/verify.ps1` include the structure gate. | Full E2E and Docker health still need to be run on this final workspace state. |

## Line Budget Exceptions

No production file currently needs a line-budget exception.

| File | Reason | Target |
|---|---|---|
| None | Structure gate passes without production exceptions. | Keep production files below 450 lines. |

## Largest Watch Files

These are below budget but should not become new dumping grounds:

| File | Current Role | Guidance |
|---|---|---|
| `backend/internal/store/agent_runs.go` | Agent run CRUD/events/checkpoint persistence | Split by status/events/checkpoints if it grows past the alarm. |
| `src/hooks/useIdeState.js` | Top-level frontend state facade | Compose new sub-hooks instead of adding feature logic here. |
| `src/hooks/useChatState.js` | Chat state facade | Keep send queue, stream merge, references, compaction, and resume in existing modules. |
| `src/styles/motion.css` | Shared motion classes and keyframes | Add new motion only when registered and documented. |
| `src/styles/chat-composer.css` | Composer shell/input/action bar/model status styling | Preserve composer visual contract. |
| `src/styles/chat-composer-queue.css` | Pending input/running steer/follow-up styling | Keep running-input queue panels separate from the composer input shell. |

## Mature Agent Boundary Map

Perseus follows the same broad separation used by OpenCode, Cline, Roo/Kilo, and Claude Code style agents:

- **Tool registry:** declares which tool name maps to which implementation.
- **Tool argument ledger:** stores sanitized/truncated arguments separate from model-facing results.
- **Tool runner:** executes the selected tool and normalizes result/error shape.
- **Tool event emitter:** produces started/completed/workspace-changed events for UI and diagnostics.
- **Checkpoint recovery:** handles transient provider failures after provider retry and stream fallback.
- **Workflow tools:** `attempt_completion` and `ask_followup_question` are always available workflow endpoints.

## Required Verification

For structural work, run:

```bash
npm run test:ui
npm run test:structure
npm run test:unit
npm run build
cd backend && go test ./...
```

Before handoff for app-level work, also run:

```bash
npm run test:e2e
docker compose up -d --build --force-recreate
```

Then check:

```text
http://127.0.0.1:8080/api/health
```
