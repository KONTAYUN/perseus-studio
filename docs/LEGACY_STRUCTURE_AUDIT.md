# Legacy Structure Audit

This audit records the old-code review focused on mechanical line-count splits, thin wrappers, duplicated helpers, and modules that create too much navigation for too little responsibility.

## Review Standard

- Keep a split when it represents a stable responsibility or change reason.
- Merge only when a file is a thin pass-through with no durable concept.
- Re-split only when one file owns unrelated behaviors that change for different reasons.
- Do not optimize for file count or average line count.

## Results

| Area | Decision | Notes |
|---|---|---|
| `src/hooks` | Keep with observation | Existing splits mostly match state-flow responsibilities: chat run stream, run message state, editor tabs, layout, file tree mutations, and settings. No obvious thin wrapper worth merging in this pass. |
| `backend/internal/ai` | Keep | The many files map to real agent concerns: provider adapters, context building, tool manifests, tool execution, action boundaries, transcripts, recovery, and budget handling. Merging would reduce navigability. |
| `backend/internal/api` | Re-split broad route files | File and note transfer handlers were broad route groups. They now split by read/history/write/trash and import/export/helper responsibilities while preserving route behavior. |
| `backend/internal/workspace` | Keep | Workspace files map to filesystem concerns: read/write/path/order/trash/history/search/assets. No line-count-only split was found that clearly harms maintenance. |
| Old UI components | Re-split heavy entry components | File tree, chat messages, and rich text history had large render entry files. They now delegate row/message/history utility responsibilities to smaller components. |
| Rich text CSS | Re-split by control family | Toolbar CSS was a dense section. It now splits by toolbar shell, block trigger, selection toolbar, table toolbar, format controls, and menus. |

## Follow-Up Watchlist

These files are below the review threshold but should not absorb unrelated behavior:

| File | Why Watch |
|---|---|
| `backend/internal/ai/context_assembler.go` | The single model-context entry owns token budgeting and old tool-result pruning; protocol serialization remains in `context_protocol.go`. |
| `src/components/editor/richText/RichTextEditor.jsx` | Composition entry for Tiptap setup and synchronization; keep new behavior in existing hooks/helpers. |
| `src/hooks/chatRunMessageState.js` | Run message placement and recovery rules are subtle; add tests before refactoring. |
| `backend/internal/ai/tool_manifest.go` | Tool declaration table is large but central; avoid scattering manifest metadata unless generation becomes necessary. |
| `backend/internal/api/conversation_run_execute.go` | Conversation execution endpoint is compact enough, but new run lifecycle behavior should move into helpers. |

## Outcome

- Production files over the 450-line review threshold: 0.
- No public API, storage format, route, or database behavior changed.
- The remaining large files are kept because their responsibilities are stable and documented.
