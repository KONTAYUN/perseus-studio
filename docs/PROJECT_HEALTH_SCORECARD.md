# Perseus Studio Project Health Scorecard

This scorecard records the current engineering-health standard and the evidence behind the score. The target is **9.5 / 10** by responsibility clarity, maintainability, and verified checks, not by average file length.

## Current Score

Overall: **9.6 / 10**

## Optimization Baseline

This project is optimized by runtime and maintenance impact, not by deleting lines.

| Area | Baseline | Optimization Direction |
|---|---|---|
| Frontend build | Before this pass, Vite emitted one main JS chunk: `assets/index-CqgydvHg.js` at **1,283.84 kB** minified / **387.27 kB** gzip, with the Rollup large chunk warning. After this pass, the main entry is **304.27 kB** / **89.29 kB** gzip, and the largest vendor chunks are **363.35 kB** editor, **361.82 kB** general vendor, and **141.99 kB** React. | Keep editor/main layout on the first screen, lazy-load low-frequency overlays, and use stable vendor chunks for React, Tiptap, icons, and general vendor code. |
| Test maintainability | Chat run stream tests repeated EventSource/window setup and `waitForAgentRunStream` boilerplate in each case. AI integration tests repeated request payload decode setup. | Move repeated test harness work into package-local/unit-test helpers while preserving every scenario assertion. |
| Runtime rendering | Resize and active panel behavior already avoid React state updates on every drag frame. Chat Markdown, message bubbles, and code blocks already use memo boundaries. | Keep memo limited to proven repeated render boundaries; avoid broad memo wrappers that add complexity without reliable wins. |

| Area | Score | Evidence | Remaining Risk |
|---|---:|---|---|
| Structure standard | 9.6 | `docs/STRUCTURE_STANDARDS.md` treats 450 lines as a review threshold, not a hard split rule. `npm run test:structure` requires a responsibility review entry for threshold crossings. | Future agents must not reintroduce mechanical line-count splits. |
| Frontend boundaries | 9.6 | Rich text editor behavior is split by responsibility. File tree, chat messages, and rich text history now have smaller render entries with row/message/history helpers separated. | Keep new UI behavior in the extracted components instead of growing entry files again. |
| CSS boundaries | 9.6 | `src/index.css` is an ordered entrypoint. Rich text styling is split into editor base, toolbar shell, block trigger, selection toolbar, table toolbar, format controls, menus, history, content, image, and table sections. Toast visuals are isolated in `src/styles/toast.css`; shared motion semantics stay in `src/styles/motion.css`. | Future toolbar work should stay in the matching control-family section. |
| Backend import/export | 9.6 | DOCX import/export facades delegate to style types, extraction, Word XML, and zip helpers. File and note transfer handlers now split by route responsibility without changing API behavior. | Keep HTTP decoding in handlers and conversion/filesystem behavior in helpers/services. |
| Agent kernel | 9.5 | `docs/AGENT_KERNEL_V2.md` records the v2 execution model: bounded context, targeted note reads, tool/result ledgers, checkpoint restore state, and the legacy rollback switch. | Future AI work should extend the kernel ledger/read-target model instead of adding ad hoc loop-guard exceptions. |
| UI system | 9.5 | `docs/UI_DESIGN_SYSTEM.md` documents rich text editor control exceptions. `npm run test:ui` continues to block raw one-off buttons, shadows, radius drift, and broad motion. | Editor-specific controls must stay in named CSS classes, not long JSX class strings. |
| Old-code audit | 9.5 | `docs/LEGACY_STRUCTURE_AUDIT.md` records the old-code review and decisions. Files over the 450-line review threshold are registered below with responsibility-based decisions. | Full package-wide Go tests depend on local Go or Docker availability; keep Docker fallback documented. |

## Responsibility Review Register

These production files cross the 450-line review threshold or sit near it. A threshold crossing is allowed only when this table records the responsibility and the next correct maintenance action.

| File | Classification | Decision |
|---|---|---|
| `src/styles/motion.css` | Shared motion token mapping, keyframes, reduced-motion behavior, and compatibility motion classes. | Keep this as the single motion semantics registry. Move component-specific visual surfaces, such as toast styling, to their own CSS section instead of splitting keyframes by line count. |
| `backend/internal/ai/note_view_blocks.go` | HTML note fragment block indexing, section grouping, raw block capture, and compact metadata projection for `read_workspace_note_view`. | Keep together for now because parser/tokenizer/index metadata must evolve as one behavior. If it grows again, split by real boundary: raw-token capture, block/section indexing, and payload projection. |
| `backend/internal/ai/context_read_spans.go` | Current-read working set extraction, read-result span identity, stale/superseded read compaction, and model-facing read metadata for text and HTML note reads. | Keep together for now because the span identity rules must stay consistent across compaction, diagnostics, and kernel progress. If it grows again, split by real boundary: result decoding, supersession rules, and working-set projection. |
| `backend/internal/ai/tool_result_budget.go` | Model-facing tool-result budgeting, compact note-index projection, continuation hints, and stable metadata preservation for large tool results. | Keep together for now because budget trimming and metadata preservation must be reasoned about as one behavior. If it grows again, split by real boundary: generic JSON budgeting, note-index compaction, and read-continuation hints. |

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
| `backend/internal/ai` | Agent/provider/context/tool files follow stable responsibility families. No obvious mechanical over-splitting found. | Preserve tool registry, policy, ledger, execution, and result budget as separate concepts. |
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

Before a release handoff, also run the Docker image build and check `/api/health`.
