# Perseus Studio Project Health Scorecard

This scorecard records the current engineering-health standard and the evidence behind the score. The target is **9.5 / 10** by responsibility clarity, maintainability, and verified checks, not by average file length.

## Current Score

Overall: **9.5 / 10**

| Area | Score | Evidence | Remaining Risk |
|---|---:|---|---|
| Structure standard | 9.6 | `docs/STRUCTURE_STANDARDS.md` now treats 450 lines as a review threshold, not a hard split rule. `npm run test:structure` requires a responsibility review entry for threshold crossings. | Future agents must not reintroduce mechanical line-count splits. |
| Frontend boundaries | 9.5 | Rich text editor behavior is split by responsibility: editor extensions, HTML utilities, image/table extensions, image upload, document sync, toolbar lifecycle, block trigger, and render surface. Chat Markdown, resize, history, and layout changes remain in their own modules. | `RichTextHistoryPanel` and `FileTree` are slightly over the threshold and should be split only when their feature areas change. |
| CSS boundaries | 9.5 | `src/index.css` is an ordered entrypoint. Rich text styling is now split into editor base, toolbar, history, content, image, and table sections. Chat Markdown has its own section. | `rich-text-toolbar.css` is dense because it owns several toolbar control families; split by control family only if toolbar work expands. |
| Backend import/export | 9.5 | DOCX import/export facades now delegate to style types, HTML extraction, Word XML patch/apply logic, and DOCX zip helpers. Public functions `HTMLToDocx` and `DocxToHTML` keep behavior stable. | API handler entry files remain broad route groups; split by route family only when changing those routes. |
| UI system | 9.5 | `docs/UI_DESIGN_SYSTEM.md` documents rich text editor control exceptions. `npm run test:ui` continues to block raw one-off buttons, shadows, radius drift, and broad motion. | Editor-specific controls must stay in named CSS classes, not long JSX class strings. |
| Old-code audit | 9.4 | Existing hooks/components/API/workspace areas were reviewed for mechanical splits. Most old splits map to stable responsibilities; the few threshold crossings are registered below instead of being force-split. | Full package-wide Go tests depend on local Go or Docker availability; keep Docker fallback documented. |

## Responsibility Review Register

These production files cross the 450-line review threshold or sit near it. A threshold crossing is allowed only when this table records the responsibility and the next correct maintenance action.

| File | Classification | Decision |
|---|---|---|
| `src/components/editor/richText/RichTextHistoryPanel.jsx` | Clear but slightly large | Keep for now. It owns the global recent-changes panel interaction: list, preview, restore, delete, and clear. Split into list/detail/action subcomponents only when history UI changes again. |
| `src/components/sidebar/FileTree.jsx` | Clear but slightly large old UI entry | Keep for now. It is a file tree render facade with selection, rename, drag/drop, and row actions already backed by hooks/actions. Split row/action rendering only when file-tree behavior changes. |
| `backend/internal/api/file_handlers.go` | Clear route group, broad by API surface | Keep as an HTTP handler family. Split into read/write/delete or asset handlers only when those routes change; do not split by line count alone. |
| `backend/internal/api/note_transfer_handlers.go` | Clear route group, import/export heavy | Keep as the note transfer HTTP layer. Conversion details already live in DOCX/HTML/Markdown helpers. Split by import/export route family only when API behavior changes. |
| `src/styles/rich-text-toolbar.css` | Clear but dense CSS section | Keep as the rich text toolbar/control section. If it grows, split by toolbar family such as selection toolbar, block toolbar, color/font controls, and table picker. |

## Completed Structural Cleanup In This Pass

- `src/components/editor/richText/RichTextEditor.jsx` is now below the review threshold and acts as the composition entry for Tiptap state and callbacks.
- `src/components/editor/richText/richTextEditorExtensions.js` owns Tiptap extension assembly.
- `src/components/editor/richText/richTextHtmlUtils.js` owns editor HTML decoration and sanitizing before save.
- `src/components/editor/richText/richTextImageExtension.js` and `src/components/editor/richText/richTextTableExtension.js` own Tiptap node extensions.
- `src/components/editor/richText/useRichTextImageUpload.js` owns image upload insertion.
- `src/components/editor/richText/useRichTextDocumentSync.js` owns external HTML and pinned-selection synchronization.
- `src/components/editor/richText/useRichTextToolbarLifecycle.js` owns floating toolbar viewport, scroll, pointer, and resize lifecycle.
- `src/components/editor/richText/RichTextEditorSurface.jsx` owns the editor render surface.
- DOCX rich-text style logic is split into `docx_style_types.go`, `docx_style_html_extract.go`, `docx_style_word_patch.go`, `docx_style_zip.go`, `docx_style_import_extract.go`, `docx_style_import_apply.go`, and `docx_style_import_zip.go`.
- `src/styles/editor.css` is back to editor shell concerns; rich text toolbar/history/content/image/table styles live in separate section files.

## Old-Code Audit

| Area | Result | Next Action |
|---|---|---|
| `src/hooks` | Existing hook splits are mostly responsibility-based: chat state, layout state, editor tabs, file tree actions, task state, and settings state. | Do not merge by size. When touching a hook, keep state orchestration separate from pure helpers. |
| `src/components` | Some older components are facade-like but acceptable. The main risk is UI entry components absorbing action detail. | Prefer subcomponents for repeated render regions and hooks for cross-component effects. |
| `backend/internal/ai` | Agent/provider/context/tool files follow stable responsibility families. No obvious mechanical over-splitting found. | Preserve tool registry, policy, ledger, execution, and result budget as separate concepts. |
| `backend/internal/api` | Handler files are grouped by route family. Some files are broad because routes are broad, not because unrelated domains are mixed. | Split only when changing route families or when helper logic leaks into handlers. |
| `backend/internal/workspace` | Workspace code remains service-oriented: filesystem operations, path helpers, history, trash, and ordering are separated. | Keep workspace helpers package-local unless reused by another package. |

## Verification Standard

For this score to remain valid, structural work must pass:

```bash
npm run test:ui
npm run test:structure
npm run test:unit
npm run build
cd backend && go test ./internal/workspace ./internal/api ./internal/ai
```

If local Go is unavailable on Windows, use the Docker equivalent for the Go target packages:

```bash
docker run --rm -v "E:\项目\_richtext-rebuild-git\backend:/src/backend" -w /src/backend docker.m.daocloud.io/library/golang:1.25-bookworm go test ./internal/workspace ./internal/api ./internal/ai
```

Before a release handoff, also run the Docker image build and check `/api/health`.
