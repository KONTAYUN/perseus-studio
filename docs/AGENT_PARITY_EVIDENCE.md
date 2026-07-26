# Agent Parity Evidence Ledger

Status: active implementation ledger. Rows remain `partial` until the exact
source mapping and their regression coverage are complete.

The production agent must not claim Codex parity from a shared design idea.
Every behavior needs an exact mature-project source, a verified current
implementation, and a regression test. If any one is missing, the behavior is
either a Perseus-specific adaptation or an unverified legacy behavior.

## Evidence Rule

| Status | Meaning | May be enabled by default? |
| --- | --- | --- |
| `proven` | Exact source, matching behavior, and passing regression test exist. | Yes. |
| `partial` | A source exists, but Perseus differs or does not yet have the required test. | No new claims of parity. |
| `custom` | Required by Perseus but not supplied by the reference project. It needs a small written specification and explicit approval. | Only after approval and a dedicated test. |
| `unverified` | Existing behavior has no source-backed design or cannot be shown to match the source. | No. It must be removed or rewritten. |

Rules for every future agent change:

1. No source URL and source file path: no design decision and no code.
2. A broad phrase such as "Codex-style" or "follows the principle" is not evidence.
3. A pending test is not verification. The exact test must exist and pass.
4. A Perseus-specific requirement is allowed, but it must say `custom`; it cannot be presented as Codex behavior.
5. Implementation commits must name the ledger row IDs they satisfy.

## Fixed Reference Set

| ID | Project | Fixed source | What it may prove |
| --- | --- | --- | --- |
| `C1` | OpenAI Codex | [retry.rs](https://github.com/openai/codex/blob/d72d669/codex-rs/codex-client/src/retry.rs) | Request retry classification and backoff. |
| `C2` | OpenAI Codex | [model provider defaults](https://github.com/openai/codex/blob/d72d669/codex-rs/model-provider-info/src/lib.rs) | Default retry policy settings. |
| `C3` | OpenAI Codex | [responses retry](https://github.com/openai/codex/blob/d72d669/codex-rs/core/src/responses_retry.rs) | Stream reconnection and transport fallback. |
| `C4` | OpenAI Codex | [turn handling](https://github.com/openai/codex/blob/d72d669/codex-rs/core/src/session/turn.rs) | Turn lifecycle, tool-result flow, completion, and compaction boundaries. |
| `R1` | Roo Code | [read_file](https://roocode.com/docs/features/tools/read-file) | Bounded file reads and targeted ranges. |
| `R2` | Roo Code | [apply_diff](https://roocode.com/docs/features/tools/apply-diff) | Targeted edit application and failed-edit handling. |
| `R3` | Roo Code | [context condensing](https://roocode.com/docs/features/context-management/context-condensing) | Context reduction before an overlong conversation continues. |
| `R4` | Roo Code | [ToolRepetitionDetector](https://github.com/RooCodeInc/Roo-Code/blob/main/src/core/tools/ToolRepetitionDetector.ts) | Stop only consecutive identical canonical tool calls; reset after intervention. |
| `R5` | Roo Code | [validateToolResultIds](https://github.com/RooCodeInc/Roo-Code/blob/main/src/core/task/validateToolResultIds.ts) | Preserve tool-call/result protocol integrity and synthesize an interrupted result when one is missing. |
| `R6` | Roo Code commit `b867ec9145750d0ae1ff7f02d35406e9bf2a0b16` | [`markdown-formatting.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/prompts/sections/markdown-formatting.ts), [`attempt_completion.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/prompts/tools/native-tools/attempt_completion.ts), [`AttemptCompletionTool.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/tools/AttemptCompletionTool.ts), [`MarkdownBlock.tsx`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/webview-ui/src/components/common/MarkdownBlock.tsx) | Require complete Markdown in the completion result, pass it through unchanged, and render fenced code in the Markdown UI. It does not prove that plain commands should be heuristically wrapped. |
| `R7` | Roo Code commit `b867ec9145750d0ae1ff7f02d35406e9bf2a0b16` | [`ask_followup_question.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/prompts/tools/native-tools/ask_followup_question.ts), [`AskFollowupQuestionTool.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/tools/AskFollowupQuestionTool.ts), [`ChatRow.tsx`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/webview-ui/src/components/chat/ChatRow.tsx), [`FollowUpSuggest.tsx`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/webview-ui/src/components/chat/FollowUpSuggest.tsx) | Suspend on an explicit question tool, render the Markdown question and plain suggestion controls in one chat row, and return the user's answer as the original tool result before continuing the same task. |
| `R8` | Roo Code commit `b867ec9145750d0ae1ff7f02d35406e9bf2a0b16` | [`AskFollowupQuestionTool.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/tools/AskFollowupQuestionTool.ts), [`Task.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/task/Task.ts), [`ChatRow.tsx`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/webview-ui/src/components/chat/ChatRow.tsx) | Keep the question message, its answer/result, and later assistant messages as separate task/message events. Updating an answered question does not project that question onto the next assistant message. |
| `R9` | Roo Code commit `b867ec9145750d0ae1ff7f02d35406e9bf2a0b16` | [`rules.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/prompts/sections/rules.ts), [`objective.ts`](https://github.com/RooCodeInc/Roo-Code/blob/b867ec9145750d0ae1ff7f02d35406e9bf2a0b16/src/core/prompts/sections/objective.ts) | Use available tools to obtain discoverable facts and avoid asking the user to perform investigation the agent can perform itself. This is a prompt and tool-contract boundary, not a keyword classifier or a runtime search gate. |
| `C5` | OpenAI Codex | [`prompt_with_apply_patch_instructions.md`](https://github.com/openai/codex/blob/main/codex-rs/core/prompt_with_apply_patch_instructions.md) | Work autonomously with available tools and return to the user when blocked by information or authority the tools cannot supply. |
| `O1` | OpenClaw | [`system-prompt.ts`](https://github.com/openclaw/openclaw/blob/main/src/agents/system-prompt.ts) | Inspect available context and tools before asking when the agent can obtain the answer itself. |
| `A1` | Aider | [repo map](https://aider.chat/docs/repomap.html) | Bounded repository context selection. |
| `A2` | Aider | [lint and test](https://aider.chat/docs/usage/lint-test.html) | Verify edits and feed failures back into the task. |
| `A3` | Aider | [base coder](https://github.com/Aider-AI/aider/blob/main/aider/coders/base_coder.py) | A selected edit format is parsed, dry-run checked, applied, then optionally linted/tested. |

The fixed set is intentionally small. A source may only be used for the
specific behavior it actually demonstrates.

## Current Evidence Matrix

| Row | Behavior | Mature source | Current Perseus location | Status | Audit finding | Required decision before implementation |
| --- | --- | --- | --- | --- | --- | --- |
| `P-01` | Retry ordinary provider requests | `C1`, `C2` | `backend/internal/ai/provider_retry.go` | `partial` | The code correctly excludes 429 from ordinary retry, but currently defaults to five attempts. The fixed Codex baseline must be checked field-by-field before calling the policy equivalent. | Write one exact policy test and choose match-or-explicit-difference. |
| `P-02` | Handle provider rate limits | `C1`, `C2`, `C4` | `backend/internal/ai/provider_rate_limit.go`; `backend/internal/api/conversation_run_execute.go` | `custom` | The fixed 20-second automatic resume is Perseus behavior. It is not evidence that Codex uses this same rule for arbitrary third-party providers. | Keep only if a separate Perseus cooldown specification is approved; otherwise remove it. |
| `P-03` | Reconnect an interrupted stream | `C3` | `backend/internal/ai/provider_stream_fallback.go` | `partial` | A distinct stream path exists, but its exact retry count, partial-response handling, and fallback trigger have not been proven against `C3`. | Add a source-to-test mapping before enabling parity claims. |
| `P-04` | Separate request failure, stream failure, rate limit, and context overflow | `C1`, `C3`, `C4` | `provider_retry.go`; `provider_stream_fallback.go`; `provider_rate_limit.go` | `partial` | The files are split, but the end-to-end state transition and UI event mapping have not been tested as a single contract. | Write one failure-state matrix and integration test per class. |
| `P-05` | Build model context from structured turn items | `C4` | `backend/internal/store/agent_turn_items.go`; `backend/internal/ai/context_assembler.go`; `context_protocol.go` | `proven` | `agent_turn_items` is the canonical ordered history. UI messages are migrated/projected but never assembled directly into provider input. | Keep migration, branch/truncate, and >48KB low-token history tests passing. |
| `P-06` | Pair each tool call with a tool result or aborted result | `C4`, `R5` | `backend/internal/store/agent_turn_items.go`; `backend/internal/ai/context_items.go`; `agent_turn_engine_tools.go` | `proven` | Calls/results persist immediately; dangling calls receive `aborted`; orphan results are omitted from model context. | Keep store repair and protocol serialization tests passing. |
| `P-07` | Prevent repeated tool calls | `R4` | `backend/internal/ai/agent_turn_engine_tools.go` | `partial` | The default path now uses one canonical `tool + decoded arguments` detector. The legacy rollback path still contains older guard state and cannot be presented as parity behavior. | Keep the detector regression suite and remove legacy-only claims from default-path documentation. |
| `P-08` | Decide whether a turn is an editing task | Roo task action protocol at commit `b867ec9145750d0ae1ff7f02d35406e9bf2a0b16` | `backend/internal/ai/agent_turn_engine.go`; `agent_turn_protocol.go` | `matched` | The runtime does not classify editing intent from wording. Every response selects an explicit action; verified mutation state only validates claims made by `attempt_completion`. | `TestTurnEngineRejectsPlainTextAndCompletesThroughExplicitAction`; `TestHandleCompletionAttemptRejectsUnverifiedChangedPaths`. |
| `P-09` | Read long HTML notes safely | `R1` supports bounded targeted reads; no direct Codex HTML feature | `read_workspace_note_view`; `note_view_blocks.go` | `custom` | HTML `index -> section/block -> raw` is a valid Perseus need, but it is an application adaptation, not Codex parity. | Write an HTML read contract: pagination, identity, invalidation, and result budgets; test it separately. |
| `P-10` | Apply a targeted edit and verify it | `R2`, `A2`, `A3` | `agent_html_write_guard.go`; `tools_exec_workspace_patch.go`; `apply_workspace_patch` | `proven` | File edits have one model-visible path: the structured `apply_workspace_patch({patch})` tool. Its formal patch payload is parsed, path-checked, applied atomically, recorded as `tool_call -> tool_result -> file_change`, and verified before completion. Plain assistant text is never parsed for side effects. | Keep the cross-provider schema, plain-text non-execution, malformed patch, atomic write, and verified `changed_paths` tests passing. |
| `P-11` | Copy a whole note without model re-output | no direct fixed-source equivalent | `manage_workspace_path`; workspace copy helpers | `custom` | Deterministic file copy is a Perseus workspace operation. It is sensible, but not proof of Codex parity. | Keep as a separately specified tool with overwrite and asset tests. |
| `P-12` | Compact context before continuation | `C4`, `R3`; OpenClaw compaction/pruning docs | `context_assembler.go`; `context_item_compaction.go`; `context_items.go` | `proven` | Triggering is token-based. Old tool outputs are pruned separately; semantic compaction writes a replacement item and retains a complete recent-turn suffix while raw history remains stored. | Keep tool-pruning, compaction-order, and raw-row preservation tests passing. |
| `P-13` | Display tool progress separately from final assistant text | `C4` supports typed turn items; UI treatment is Perseus-specific | chat hooks and run event handlers | `custom` | Showing progress in a folded stream is a product decision, not a public Codex UI contract. | Keep only as a specified Perseus UI behavior with front-end tests. |
| `P-14` | Support OpenAI-compatible and Anthropic-compatible providers | provider API specifications, not agent-loop references | `provider_openai.go`; `provider_anthropic.go`; settings UI | `partial` | Protocol compatibility must be audited against the published protocol fields separately. It cannot be inferred from Codex loop sources. | Create a provider-protocol matrix with request, stream, tool-call, usage, and error fixtures. |
| `P-15` | Deliver a formatted final answer | `R6` | `backend/internal/ai/prompts.go`; `tool_schemas_completion.go`; `agent_completion.go`; `src/components/chat/markdown/*` | `proven` | `attempt_completion.result` is the only user-visible final body. Both provider schemas require complete Markdown, fenced commands retain language tags through persistence and events, and the existing renderer supplies the language header and copy action. The backend does not auto-wrap plain text. | Keep the SSH regression, schema parity, pass-through, and Markdown utility tests passing. |
| `P-16` | Request user input or a bounded choice and resume the same task | `R7` for the wait/resume lifecycle; the two-tool split is a Perseus product adaptation | `agent_followup.go`; `followup_answer.go`; `agent_run_followup_handlers.go`; `agent_followup_answers.go`; `UserRequestMessage.jsx` | `proven` | A pending request has no successful tool result. `request_user_input` renders Markdown with no generated options; `request_user_choice` renders only two or three model-supplied choices. Either answer completes the original tool call, appends a paired result, saves the checkpoint, and resumes the same run and turn. Historical `ask_followup_question` remains readable and answerable but is not provider-visible. | Keep cross-provider schemas, option validation, checkpoint pairing, transactional idempotency, answered-state, legacy compatibility, and single-render-path tests passing. |
| `P-17` | Keep run history separate from the current message presentation | `R8` | `agent_runtime.go`; `conversation_run_completion.go`; `conversation_presentation_repair.go`; `ChatMessageBubble.jsx` | `proven` | Checkpoint restoration retains complete run events, while the presentation cursor starts after restored history. Only events created in the resumed stage attach to its message. Explicit message kinds control rendering, and an idempotent migration removes duplicate question attachments by `run_id + tool_call_id`. | Keep runtime boundary, migration idempotency, stable-message update, and stale-attachment rendering tests passing. |
| `P-18` | Explore available evidence before requesting user interaction | `R9`, `C5`, `O1` | `prompts.go`; `message_attachments.go`; `tool_schemas_question.go`; `agent_turn_engine_request.go` | `proven` | One action policy tells the model to reuse valid context, use read/list/search/inspection tools for obtainable facts, continue the task, request open input only for user-only or external data, and request choices only for bounded user decisions. Editor state contains paths only and does not change task scope. The runtime does not classify intent or require a search before every request. | `TestAgentActionPolicyPrioritizesAvailableEvidenceBeforeQuestions`; `TestUserRequestToolsUseSameBoundaryAcrossProtocols`; `TestEditorContextRemainsPathOnlyWhileExplorationStaysAvailable`; `TestTurnEngineCanExploreRelevantCurrentDocumentBeforeCompleting`; `TestTurnEngineRejectsPlainTextAndCompletesThroughExplicitAction`. |

The fixed regression behind `P-15` is the observed failure where model
commentary contained a fenced `bash` command but the later
`attempt_completion.result` downgraded it to plain text. The contract now
requires the fully formatted version inside `result`; metadata cannot be
appended as a second user-visible body. As in Roo, malformed formatting remains
the model's contract violation rather than a trigger for keyword-based rewriting.

## Known Incorrect Claims To Retire

The following phrases must not be used in code, documentation, or release notes
until their row reaches `proven`:

- "fully aligned with Codex"
- "Codex-style loop guard" for `P-07`
- "Codex rate-limit behavior" for the fixed 20-second resume in `P-02`
- "Codex HTML reading strategy" for `P-09`

## Audit Order

1. Freeze `P-07` and `P-08`: they are the direct cause of the read/search loop and have no approved basis.
2. Audit `P-05`, `P-06`, `P-10`, and `P-12` together. They form one turn execution contract.
3. Audit `P-01` through `P-04` together. They form one provider-failure contract.
4. Treat `P-09`, `P-11`, and `P-13` as explicit Perseus product specifications, not copied agent behavior.
5. Audit `P-14` as a separate provider compatibility contract.

No implementation starts from this ledger until the relevant row has a source,
an exact intended behavior, and a test list. No keyword-based user-intent
classification is permitted.

## Turn Contract Audit: 2026-07-12

This audit covers the observed failure where a long note task read and searched
for many rounds without entering a stable write-and-finish path.

| Contract question | Codex evidence | Roo / Aider evidence | Current Perseus behavior | Result |
| --- | --- | --- | --- | --- |
| What starts an action loop? | A sampling response that contains a function call. Codex records that item, executes it, records output, and schedules the next sampling request. | Roo runs an explicit task and handles the tool-use blocks returned by the model. | `runTurnEngine` executes every ordinary tool and then schedules the next model request. | Matched to the explicit action loop; no user-language classifier or inferred tool-loop flag remains. |
| What ends a Perseus agent turn? | Codex permits an assistant-only final response. | Roo task mode requires `attempt_completion`; its question tool is an explicit user-input boundary. | Perseus adopts Roo's explicit-action boundary, then separates waiting into `request_user_input` and `request_user_choice`; `start_background_task` remains delegation. | Product-level allowed difference from Codex. Wait/resume mechanics match Roo; the two-tool interaction split is an explicit Perseus adaptation. |
| How are tool calls persisted? | The completed response item is recorded before tool execution. A tool future then produces a response item for the next sampling request. | Roo saves the assistant tool-use message before execution; it validates/deduplicates results and creates an interrupted result for a missing pair. | `processToolCalls` appends a tool-call item, but some early returns bypass the paired result append. | The pair is not an atomic protocol unit. |
| How is repeated behavior stopped? | The fixed source does not expose a generic user-text intent classifier. | Roo blocks only consecutive identical canonical tool calls, asks for intervention, and resets the detector. | v3 hashes one tool name plus arguments; the older guard adds unrelated semantic rules. | Keep one source-backed identical-call detector; delete the extra invented guard. |
| How is an edit executed? | Codex executes a structured tool call, then the tool result drives the next sampling step. | Roo and Aider keep edit execution in a dedicated action channel and validate its result. | Perseus exposes one structured `apply_workspace_patch({patch})` tool. Assistant text is never scanned for side effects. | One owner and one typed action chain. |
| How is context assembled? | Codex builds model input from recorded response-item history. | Roo persists API history with tool-use/result ordering and tracks task files separately. | Perseus now loads `agent_turn_items`, validates pairs, adds path-only environment and task working-set metadata, prunes old tool results, compacts by token pressure, then serializes by protocol. | One owner: `ContextAssembler`; UI messages and runtime transcript are not provider-input sources. |

## Source-Backed Rewrite Boundary

This is the default turn-engine boundary. It does not make unrelated provider
or HTML adaptations Codex behavior.

1. There is one production loop: `runTurnEngine`. The old
   `requiresCompletion`, `toolLoopActive`, `contentOnlyCompletionPolicy`, and
   duplicate no-tool loop have been deleted.
2. There is exactly one action transport: provider-native structured tool
   calls. `apply_workspace_patch({patch})` owns file edits; Roo-style
   `attempt_completion`, Perseus `request_user_input` /
   `request_user_choice`, or `start_background_task` ends or pauses the loop.
   Models without reliable tool support cannot perform Agent file operations.
3. Use Roo's canonical identical-tool detector only for structured tool calls.
   It has no user-language classifier and no guessed "editing intent" state.
4. Make every emitted tool call have one persisted result. If execution stops,
   persist a synthetic `aborted` result before the next request, following the
   Roo pairing safety net.
5. Rebuild the model context from one ordered item history. The visible chat
   transcript is a projection of that history, never an additional context
   source.
6. Plain assistant text without an action is persisted as a rejected item,
   followed by runtime-owned `protocol_feedback`. It is never shown as a final
   bubble. Three consecutive protocol mistakes fail the run; a valid tool call
   resets the counter.

This boundary deliberately does not try to infer whether the user "means to
edit" from Chinese words, English words, typos, or any other text pattern. The
model's actual protocol response is the only decision point, as in Codex; the
provider configuration states whether structured tools are supported; URL,
model-name, or error-message guessing cannot select another execution path.

## Single Structured Action Chain

| Action | Sole model transport | Persisted execution truth | User-visible projection |
| --- | --- | --- | --- |
| Continue work | Structured ordinary tool call | paired `tool_call` and `tool_result` items | typed progress only |
| Edit files | `apply_workspace_patch({patch})` | paired result plus verified `file_change` item | typed edit status; no patch body |
| Request open input | `request_user_input({question})` | waiting tool call, then paired user result | one Markdown request message |
| Request a bounded choice | `request_user_choice({question, options})` | waiting tool call, then paired user result | one choice message |
| Delegate | `start_background_task(...)` | delegated terminal item | background-task status |
| Complete | `attempt_completion({result,...})` | validated completion item | exactly one final Markdown message |

Ordinary assistant text is presentation-only process output. It cannot mutate
files, terminate a turn, or select recovery behavior. A complete formal patch
inside `attempt_completion.result` is rejected as a terminal-tool contract
error and must be resubmitted through `apply_workspace_patch`.
