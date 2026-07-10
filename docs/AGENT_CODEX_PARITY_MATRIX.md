# Agent Codex-Parity Matrix

Reference baseline: OpenAI Codex open-source commit `d72d669`.

This matrix is the required record for claiming Codex-style behavior in Perseus Agent Kernel. A behavior is not considered parity-aligned unless it has a Codex reference, a Perseus implementation entry, and at least one regression test or explicit follow-up gap.

## Request, Stream, And Recovery

| Area | Codex reference | Codex behavior | Perseus implementation | Allowed difference | Verification |
| --- | --- | --- | --- | --- | --- |
| Provider request retry | `codex-rs/codex-client/src/retry.rs`, `codex-rs/model-provider-info/src/lib.rs` | Retry transient network and 5xx with bounded exponential backoff. Do not retry 429 as a normal transport failure. | `backend/internal/ai/provider_retry.go`, `backend/internal/ai/web_http.go`, `backend/internal/ai/provider_sdk.go` | Provider-specific body parsing maps Tencent/third-party 429 text to structured cooldown state. | `TestProviderRetryPolicyDefaultsAndEnv`, `TestGenerateReplyRetriesTransientHTTP502WithProviderPolicy`, `TestGenerateReplyDoesNotRetryHTTP429AndStoresRateLimitState` |
| Stream reconnect | `codex-rs/core/src/responses_retry.rs` | Treat stream reconnect separately from ordinary request retry. Bound reconnect attempts. | `backend/internal/ai/provider_stream_fallback.go`, `backend/internal/ai/provider_openai.go`, `backend/internal/ai/provider_anthropic.go` | Perseus supports OpenAI-compatible and Anthropic-compatible chat endpoints. | `TestGenerateReplyOpenAIFallsBackToNonStreamWhenFinalStreamBreaks`, `TestGenerateReplyAnthropicFallsBackToNonStreamWhenFinalStreamBreaks` |
| Stream fallback | `codex-rs/core/src/responses_retry.rs` | Once a stream has produced a partial provider turn, do not replay it as an empty stream-start failure. | Stream parsers now return partial ordinary text turns on parse errors; providers fallback to non-stream when appropriate. | Perseus has chat-completions style fallback because many configured providers are OpenAI-compatible gateways. | `TestGenerateReplyOpenAIFallsBackToNonStreamWhenFinalStreamBreaks`, `TestGenerateReplyAnthropicFallsBackToNonStreamWhenFinalStreamBreaks` |
| 429 / cooldown | `codex-rs/core/src/session/turn.rs` | Rate limits are usage-limit state, not normal retry or checkpoint recovery. | `backend/internal/ai/provider_rate_limit.go`, `backend/internal/api/agent_run_handlers.go`, `src/hooks/chatErrorUtils.js` | Cooldown is keyed by `user + provider + model`; reset time is inferred when providers omit headers. | `TestGenerateReplyDoesNotRetryHTTP429AndStoresRateLimitState`, `tests/unit/chatErrorUtils.test.js` |
| Checkpoint recovery | `codex-rs/core/src/session/turn.rs` | Continue from structured turn state; do not duplicate successful tools. | `backend/internal/ai/agent_recovery.go`, `backend/internal/ai/fallback.go` | Perseus checkpoint recovery only handles broken transport after verified tool progress; provider HTTP failures remain request-layer failures. | `TestGenerateReplyCheckpointResumeAfterTransientProviderError`, `TestGenerateReplyStreamFallbackHTTP5xxDoesNotCheckpointResume` |
| Context overflow | `codex-rs/core/src/session/turn.rs` | Context overflow is separate from network failure and handled by compaction. | `backend/internal/ai/context_manager.go`, `backend/internal/ai/context_pruning.go`, `backend/internal/ai/provider_rate_limit.go` | Existing Perseus context compaction remains, with explicit failure metadata. | Existing context compaction tests; follow-up: add explicit overflow-vs-transport UI test. |

## Thread, Turn, Items, And Tools

| Area | Codex reference | Codex behavior | Perseus implementation | Allowed difference | Verification |
| --- | --- | --- | --- | --- | --- |
| App Server main loop | `codex-rs/app-server/README.md`, `codex-rs/app-server/src/transport/stdio.rs`, `codex-rs/app-server/src/request_processors/turn_processor.rs` | Client initializes one app-server connection, starts/resumes a thread, starts turns, reads structured item notifications, and responds to server tool requests. | `backend/internal/ai/codex_sidecar.go` starts `codex app-server --listen stdio://`, sends `initialize` + `initialized`, calls `thread/start` and `turn/start`, streams `item/agentMessage/delta`, and answers `item/tool/call`. Enabled only with `PERSEUS_AGENT_ENGINE=codex_app_server`. | First integration starts an app-server process per Perseus run while the bridge hardens; persistent per-user thread reuse is a follow-up before claiming full session parity. | `go test ./internal/ai`; follow-up: fake app-server protocol fixture for initialize/thread/turn/tool calls. |
| Dynamic tools | `codex-rs/app-server/README.md` dynamic tool section, `codex-rs/app-server/tests/suite/v2/dynamic_tools.rs` | Dynamic tools require `initialize.capabilities.experimentalApi=true`; tools are registered on `thread/start`; server invokes them via `item/tool/call`; client responds with `contentItems` and `success`. | `codex_sidecar.go` registers a `perseus` namespace exposing bounded workspace/note tools and returns `inputText` tool outputs. | Perseus tool names and HTML note views are app-specific. Hidden/deferred tools are not used in first integration. | `go test ./internal/ai`; follow-up: protocol fixture checks tool response shape. |
| Thread / turn / item model | `codex-rs/core/src/session/turn.rs` | Model input is assembled from structured items, not raw UI bubbles. | `backend/internal/ai/agent_transcript.go`, `backend/internal/ai/agent_kernel_v2.go`, `backend/internal/ai/context_manager.go` | Perseus keeps legacy conversation messages for display and compatibility. | `context_transcript_messages_test.go`, `TestGenerateReplyOpenAIStreamsFinalTextAfterToolResult` |
| Tool call/result pairing | `codex-rs/core/src/session/turn.rs` | Tool calls and outputs are paired; missing output is explicit aborted state. | `backend/internal/ai/agent_tool_loop.go`, `backend/internal/ai/agent_runtime.go` | Tool schemas are Perseus-specific. | Existing agent tool loop tests; follow-up: add missing-result fixture to parity suite. |
| Loop progress guard | `codex-rs/core/src/session/turn.rs` | Loop guard must reason about real progress, not only repeated tool names. | `backend/internal/ai/agent_tool_loop.go`, `backend/internal/ai/tool_result_ledger.go` | HTML note block reads count as progress when target block/section changes. | `TestGenerateReplyOpenAIToolLoopStopsRepeatedInspectLoop`; follow-up: add repeated-vs-different-block regression. |
| Large tool result ledger | Codex context compaction and item history model | Large observations are summarized/compacted before model re-entry. | `backend/internal/ai/tool_result_budget.go`, `backend/internal/ai/tool_result_ledger.go`, `backend/internal/ai/context_observation_view.go` | Perseus stores HTML note block/index metadata because note storage is application-specific. | Existing tool result budget tests; follow-up: largest-result diagnostic assertion. |

## Provider Compatibility

| Area | Codex reference | Codex behavior | Perseus implementation | Allowed difference | Verification |
| --- | --- | --- | --- | --- | --- |
| Responses wire protocol | `codex-rs/model-provider-info/src/lib.rs`, `CHAT_WIRE_API_REMOVED_ERROR` | Codex App Server only accepts `wire_api = "responses"`; `wire_api = "chat"` is explicitly removed. | `Dockerfile` installs `@openai/codex@0.144.1`; `codex_sidecar.go` generates a Codex provider config with `wire_api="responses"` and `env_key=PERSEUS_CODEX_API_KEY`. | Existing Perseus Chat Completions providers are not automatically Codex-compatible. They must either expose `/v1/responses` or use a separately documented adapter. | `go test ./internal/ai`; follow-up: container smoke against a mock `/v1/responses` server. |
| Runtime packaging | `@openai/codex@0.144.1` npm package | App Server is delivered by the official Codex CLI package and platform binary package. | Runtime image installs Node/npm and pins `@openai/codex@0.144.1`; `docker-compose.yml` sets `PERSEUS_AGENT_ENGINE=codex_app_server`. | Image size increases because the official Codex binary is bundled. This is accepted for stability. | Follow-up: Docker build smoke and `codex --version` in image. |

## HTML Note Adaptations

These are not direct Codex features. They are Perseus adaptations built to follow Codex principles: bounded observations, target-specific reads, deterministic mutations, and verified writes.

| Area | Codex-style principle | Perseus implementation | Verification |
| --- | --- | --- | --- |
| `.html` read routing | Avoid unbounded file observations; read structure first, then target blocks. | `read_workspace_text` redirects `.html` to `read_workspace_note_view mode=index`; batch reads place HTML in `note_indexes`. | `TestExecuteToolReadWorkspaceTextRedirectsHTMLToNoteIndex`, `TestExecuteToolReadWorkspaceFilesIndexesHTMLItems` |
| Block identity | Tool reads should be target-addressable and invalidated by file changes. | `note_view_blocks.go` returns `doc_hash`, `block_id`, `section_id`, pagination metadata. | `TestReadWorkspaceNoteViewIndexHandlesLongHTMLLines`, `TestReadWorkspaceNoteViewRejectsStaleDocHash` |
| Deterministic copy | Do not ask the model to re-output whole documents for simple duplication. | `manage_workspace_path action=copy`, `workspace.CopyFile`, asset reference rewrite. | `TestExecuteToolManageWorkspacePathCopiesHTMLWithoutOverwrite` |
| Local asset reference stability | File movement/copying must preserve note images. | `workspace/note_asset_refs.go`, `workspace/path_ops.go` | Existing move tests plus copy test. |
| HTML mutation precondition | Mutations must be based on current, target-specific observations. | Existing `.html` updates through `apply_workspace_patch` require a same-turn raw block/section/full read with the current `doc_hash`; stale raw reads are rejected before writing. | `TestAgentRuntimeHTMLPatchRequiresCurrentRawRead`, `TestAgentRuntimeHTMLPatchRejectsStaleRawRead` |

## Frontend Events

| Area | Codex-style principle | Perseus implementation | Verification |
| --- | --- | --- | --- |
| Main chat vs progress stream | Intermediate tool activity is not ordinary assistant prose. | Main list keeps user, final assistant, and error cards; progress items are folded into run shell. | `TestGenerateReplyOpenAIStreamsFinalTextAfterToolResult`, `npm run build` |
| Structured failures | UI does not parse error strings to decide retry behavior. | `failure_class`, `retry_after_ms`, `reset_at`, `can_resume_now`, `recommended_action` flow through `client.js`, chat hooks, and message cards. | `tests/unit/chatErrorUtils.test.js`, `npm run build` |
| Cooldown resume | Clicking continue during provider cooldown must not call provider again. | Backend blocks `/resume` during active cooldown; frontend disables local continue and removes temporary shell if backend returns cooldown. | `TestGenerateReplyDoesNotRetryHTTP429AndStoresRateLimitState`, `tests/unit/chatErrorUtils.test.js`; follow-up: add API-level resume cooldown fixture. |

## Current Gaps

- Add a dedicated parity test fixture for missing tool result -> aborted item -> no orphan tool output in next context.
- Add an API-level resume cooldown fixture proving `/resume` does not call the provider before `reset_at`.
- Add explicit context-overflow UI regression so overflow cannot be displayed as a connection interruption.
- Add repeated HTML block-read guard test: same block without new info should steer, different block should count as progress.
