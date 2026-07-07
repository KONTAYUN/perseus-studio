# Perseus Structure Standards

This document is the stable structure contract for Perseus. Future AI agents and developers must use these boundaries before doing structural refactors. The goal is not periodic rewrites; the goal is small maintenance inside known responsibility areas.

## Core Principle

Responsibility boundaries come first. Line count is an alarm, not a personal style preference.

When a file grows too large, split it by the responsibility it already contains. Do not invent a new architecture because a different agent prefers different names.

## Change Rules

- Refactors are zero-behavior unless the user explicitly asks for behavior changes.
- Keep public APIs, tool names, HTTP routes, JSON fields, SSE event names, CSS class names, and user-visible UI behavior stable during structure work.
- Thin facade files may compose modules and expose a stable API, but must not contain business detail.
- If a boundary itself needs to change, update this document, `ARCHITECTURE.md`, and `docs/PROJECT_HEALTH_SCORECARD.md` first, with the migration reason.
- Do not do scheduled big rewrites. Split only when a feature touches an area, a file crosses the review threshold, or responsibilities are mixed.

## File Review Thresholds

- Frontend production JS/JSX: 450 lines triggers a responsibility review.
- Backend ordinary production Go: 450 lines triggers a responsibility review.
- CSS section files: 450 lines triggers a responsibility review.
- Large integration tests and fixtures may exceed 450 lines, but production structure scores do not use test exceptions to hide runtime complexity.

The threshold is not a hard split rule. A file may stay over the threshold when its responsibility is clear, a split would create worse navigation, and `docs/PROJECT_HEALTH_SCORECARD.md` records the decision. If a file is both over the threshold and mixed in responsibility, split by responsibility before adding a scorecard entry.

## Frontend Boundaries

- React components render UI and own only local interaction.
- Hooks/actions own state flow, async actions, and cross-component coordination.
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
- `src/styles/chat.css`: chat messages, assistant progress, recovery notices, and chat-only message surfaces.
- `src/styles/chat-markdown.css`: rendered assistant Markdown, code blocks, tables, and chat content scroll affordances.
- `src/styles/chat-composer.css`: composer shell, input, bottom action bar, model status, and scroll-to-bottom control.
- `src/styles/chat-composer-queue.css`: pending input queue, running steer previews, and follow-up question panels above the composer.
- `src/styles/editor.css`: editor tabs, editor textarea, editor drop states.
- `src/styles/rich-text-toolbar.css`: rich text floating toolbar shell and shared floating surface.
- `src/styles/rich-text-block-toolbar.css`: six-dot block trigger and block-menu trigger visuals.
- `src/styles/rich-text-selection-toolbar.css`: selection toolbar shell.
- `src/styles/rich-text-table-toolbar.css`: table toolbar, table trigger, and table command menu visuals.
- `src/styles/rich-text-format-controls.css`: font, color, highlight, and formatting controls.
- `src/styles/rich-text-toolbar-menus.css`: rich text submenu panels and table size picker visuals.
- `src/styles/rich-text-history.css`: global recent changes panel and history preview controls.
- `src/styles/rich-text-content.css`: rich text body typography, prose density, headings, lists, quotes, inline code, and document-width behavior.
- `src/styles/rich-text-image.css`: note image frames, resize handles, image toolbar, and alignment/width visuals.
- `src/styles/rich-text-table.css`: rich text table, table cell, selected cell, and table-editing visuals.
- `src/styles/file-tree.css`: file tree rows, file labels, inline rename/drop indicators.
- `src/styles/settings.css`: settings modal/card/view-specific transitions.
- `src/styles/utilities.css`: narrow global utility overrides that do not fit a domain.

Composer visual contract: Codex-style bottom action bar inside the composer shell, with attachments on the lower left, model/reasoning status near the lower right, and send/stop on the far lower right. Text and attachment previews sit above the action bar. No Pretext/contenteditable/mirror layer and no dynamic text-measurement avoidance logic.

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

- frontend production JS/JSX review entries for files over the threshold,
- backend production Go review entries for files over the threshold,
- CSS entrypoint order and review entries for section files over the threshold,
- composer anti-regression patterns,
- Pretext residue ban.

If the gate fails, fix the boundary or document a deliberate responsibility review before continuing. Do not split only to reduce line count.
