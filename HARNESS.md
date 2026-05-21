# Perseus Studio Agent Harness

This document defines how the Perseus Studio agent should behave. It is for developers and for future prompt/runtime changes.

## Identity

Perseus Studio is a multi-user AI workspace. The agent may chat, inspect workspace files, edit files, run bounded workspace commands, use web search, use enabled Skills, and create background tasks.

The agent must not claim it has read, written, moved, deleted, searched, or executed anything unless the corresponding tool actually succeeded.

## Workspace Rules

- Workspace paths are always user-scoped and workspace-relative.
- The agent must not access paths outside the current user's workspace.
- When the user refers to "current file", "this file", "left editor", "right editor", or "middle editor", resolve the target from `current_file_context` or `editor_panels_context`.
- File names and editor tabs are metadata only. If content is needed, call `read_workspace_text`.
- Do not guess file content from a file name.

## File Operation Rules

- Read before editing existing files unless the user provided the complete replacement content.
- Use `save_workspace_file` for new or existing text files. Set `mode=create` for new files and `mode=overwrite` for existing files.
- Use `manage_workspace_path` for creating folders, renaming, moving, or moving workspace content into `回收站`; trash is not permanent deletion.
- Use `organize_workspace_matches` for deterministic batch organization by path/name match, such as moving a group of episode files into one folder.
- After a successful write/create/delete/move/rename, final responses should name the affected path.
- If a tool fails, report the failure and do not say the operation succeeded.

## Command Rules

- Use `run_workspace_command` only for inspection, tests, builds, scripts, or batch processing that must run inside the workspace.
- Commands run with a workspace-relative cwd and must not be used to bypass workspace isolation.
- Dangerous commands are blocked by the backend safety policy.
- Command failures must be summarized honestly with exit code and key stderr/stdout.

## Web Rules

- Do not browse by default for ordinary chat.
- Use `perseus_search_web` for current or external information.
- Use `web_extract` only when a search result page needs more detail and the provider supports extraction.
- When web results influence the answer, include source URLs.

## Skill Rules

- Selected Skills are user intent. Use their instructions when available.
- If only Skill metadata is present and the content is needed, call `read_skill`.
- Do not read every installed Skill by default.
- Disabled Skills must not be read.
- `delete_skill` is permanent and should only be used for explicit Skill deletion requests.

## Frontend UI Rules

For any frontend UI work, read and follow `docs/UI_DESIGN_SYSTEM.md` before changing visual styling, controls, panel headers, file tree UI, chat UI, settings UI, dialogs, popovers, animation, color, spacing, or border radius. For any structural work, read and follow `docs/STRUCTURE_STANDARDS.md` before moving code or creating new modules.

- For structural frontend/backend work, also read `docs/STRUCTURE_STANDARDS.md` and `docs/PROJECT_HEALTH_SCORECARD.md` before editing. Keep refactors zero-behavior unless the user explicitly asks for a behavior change.
- Treat `docs/UI_DESIGN_SYSTEM.md` as the source of truth for frontend UI standards.
- Prefer shared utilities/components over one-off Tailwind class strings.
- Keep icon buttons, text buttons, inputs, menus, dialogs, and panel headers visually consistent.
- Do not add hover/active motion that shifts layout, makes icons jump, or combines parent and child transforms.
- Keep light and dark mode behavior aligned.
- If a UI change needs a new exception, document the exception in `docs/UI_DESIGN_SYSTEM.md`.
- UI changes must pass `npm run test:ui` before handoff, in addition to relevant unit/build checks.

## Project Structure Rules

- `docs/STRUCTURE_STANDARDS.md` is the stable source of truth for structure boundaries. Do not re-split by a new personal taxonomy unless that document, `ARCHITECTURE.md`, and the scorecard are updated first with a reason.
- Ordinary business files target fewer than 450 lines. If a file stays above that, the reason and next split must be listed in `docs/PROJECT_HEALTH_SCORECARD.md`.
- React components should render and handle local interaction. Put SSE merging, layout math, file-tree algorithms, and agent event processing in hooks or utilities.
- Chat composer work belongs under `src/components/chat/composer/`; do not expand the wrapper with input logic.
- Chat panel work belongs under `src/components/chat/panel/`; menus, scroll control, and dialogs should stay split.
- Main panel layout math and animation belong under `src/components/layout/mainPanels/`.
- Chat state changes should preserve the split between `useChatState`, `chatConversationActions`, `useChatReferenceActions`, `chatCompactionActions`, `chatRegenerateResumeActions`, and `chatRunStream`.
- File tree changes should preserve the split between `useFileTree`, `useFileSearchState`, `useFileSelectionState`, `useFileDragState`, `useFileTreeTrashActions`, `fileTreeMutationActions`, and `fileTreeDownloadActions`.
- Backend agent changes should preserve these boundaries: runtime loop, model turn, checkpoint recovery, tool loop, completion/follow-up, tool registry, tool arguments/ledger, and tool events.
- Prefer mature agent boundaries over local patch logic: read/search/edit tools, workflow completion, follow-up question, checkpoint recovery, and durable tool ledger remain separate concepts.

## Running Input Queue Rules

Runtime input must follow the Codex-style queue model, not ad hoc chat bubbles:

- A normal message submitted while a run is busy is a queued follow-up message. It stays above the composer and is sent after the current run reaches a terminal state, unless the run is recoverable and should be continued first.
- A steer submitted while a run is busy is a pending steer. Show it immediately as pending input above the composer; do not merge it into conversation history until the backend emits `agent_steer_applied`.
- A steer that is rejected or cannot be delivered stays in the pending-input area with retry/delete/queue-after-run actions. It is not a committed chat-history message.
- If a pending steer misses the active run before it is applied, move it into the queued follow-up path instead of leaving it stuck as pending guidance.
- Branching/selecting conversations is a committed-state transition. Sending after a branch must wait for the new conversation id before posting the message.
- Conversation timelines and composer runtime-input previews are isolated by conversation id. Events from a run in one conversation may continue to persist in the backend, but must not mutate the visible message list, pending steer UI, context-usage notices, or recovery notices for another selected conversation.

## Background Task Rules

Use `start_background_task` for long-running or multi-step work, including:

- web research plus report writing
- long documents
- batch file changes
- refactors
- build/test/fix loops
- work the user explicitly wants to run in the background

Background tasks must include a goal, suggested steps, and expected outputs. If a file is an expected output, the task must verify the file exists and is non-empty before marking the task done. Repair loops should attempt to create missing outputs before failing.

## Context Protocol

- `current_file_context`: one active editor file; path metadata only.
- `editor_panels_context`: multiple editor panels; includes panel id, position label, active path, and tabs; no file body.
- `workspace_file`: user-referenced workspace path; call `read_workspace_text` for content.
- `selected_skill`: user-selected Skill; content may be included by the server, otherwise call `read_skill`.
- uploaded text attachment: may include truncated text content.
- uploaded binary/image attachment: metadata or vision input only.
- task card: UI/status metadata, not a user file.
- tool event: record of a previous tool call, not a user attachment.

## Context And Tool Result Budgets

The model should receive the smallest useful context, while the UI and database may keep richer operational details.

- Apply provider-aware runtime profiles before building model requests.
- Keep recent conversation messages fuller, but truncate very old or oversized messages with an explicit truncation note.
- Truncate uploaded text and Skill content before injecting it into the model request.
- Limit editor panel tab lists; editor context remains path metadata only.
- Tool results returned to the model must stay valid JSON.
- Command stdout/stderr should be shortened for the model; full command event details can still be shown in the UI where available.
- Web search should return concise result summaries. Use `web_extract` for a specific page when full page content is actually needed.
- Large file reads may be truncated for model stability. If more content is needed, the agent should ask for a narrower target or use another read strategy.

## Provider Compatibility Profiles

Providers that claim the same protocol can still behave differently. Runtime code should keep lightweight profiles for common behavior classes:

- standard OpenAI-compatible providers
- Claude-like providers
- stricter Claude-code-proxy / coding gateways

Profiles may tune history size, attachment size, tool result size, command output size, web result size, and file read size. Prefer profile tuning over global tool removal when one provider class is fragile.

## Failure And Diagnostics

- Provider non-2xx, non-JSON/parse errors, transport failures, tool failures, command failures, task failures, and repair failures should write diagnostic logs.
- Diagnostic logs must not include API keys, full prompts, full file contents, or full model output.
- User-facing errors should stay short and point users to Settings -> Diagnostic Logs when a diagnostic log exists.

## Safety And Future TODO

Current minimum safety:

- per-user workspace root
- per-user Skills
- path sanitization
- command cwd restricted to workspace
- dangerous command rejection
- workspace delete goes to `回收站`

Future improvements:

- UI confirmation before AI bulk overwrite/delete operations
- diff preview before large AI edits
- stronger command allow/deny policy per deployment
- richer diagnostic correlation from frontend error to `diagnostic_log_id`
- explicit task cancellation/rollback strategy for multi-file operations
