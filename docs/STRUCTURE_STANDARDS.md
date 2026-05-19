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
- Hooks/actions own state flow, async actions, and cross-component coordination.
- Utilities contain pure functions only.
- Chat stream merging belongs in chat stream utilities/actions, not render components.
- File tree selection, drag/drop, mutation, trash, search, and download behavior stay in their existing hook/action modules.
- Layout math and FLIP/animation helpers stay in layout modules.
- Chat Composer changes stay inside `src/components/chat/composer/`; the wrapper remains a facade.

## CSS Boundaries

`src/index.css` is an ordered stylesheet entrypoint only. CSS lives in:

- `src/styles/tokens.css`: reset, global variables, radius token overrides, app shell background.
- `src/styles/surfaces.css`: reusable surfaces, cards, overlays, panel glow, non-motion visual primitives.
- `src/styles/motion.css`: animation keyframes/classes, motion primitives, shared interactive control motion.
- `src/styles/controls.css`: shared interactive control geometry and states.
- `src/styles/chat.css`: chat messages, composer, follow-up question panel, scroll-to-bottom control.
- `src/styles/editor.css`: editor tabs, editor textarea, editor drop states.
- `src/styles/file-tree.css`: file tree rows, file labels, inline rename/drop indicators.
- `src/styles/settings.css`: settings modal/card/view-specific transitions.
- `src/styles/utilities.css`: narrow global utility overrides that do not fit a domain.

Composer visual contract: fixed lower-right translucent/blur action dock, current button position preserved, text may pass underneath it, no Pretext/contenteditable/mirror layer, and no dynamic text-avoidance padding.

## Backend Boundaries

- `api`: HTTP concerns only: auth/user extraction, request decoding, response shape, SSE/run handler orchestration.
- `store`: SQLite persistence only: schema, CRUD, scans, ledger/checkpoint storage.
- `ai/provider_*`: protocol adapters and stream parsing only.
- `ai/context_*`: context building, estimation, pruning, freshness, and provider message conversion.
- `ai/agent_*`: agent loop, model turn, tool loop, completion/follow-up, recovery, runtime state.
- `ai/tool_*` and `ai/tools_exec_*`: tool registry, policy/scope, argument ledger, event emission, verifier, concrete tool implementations.
- `workspace`: filesystem operations, search, trash, ordering, command execution.

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
