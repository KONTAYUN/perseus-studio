# Perseus Agent Kernel

Agent Kernel is the stability layer for long workspace tasks. It is not a new
storage format and it is not a Codex CLI wrapper. The goal is a lightweight
Perseus-native runner with mature-agent guardrails: bounded context, typed turn
items, explicit tool progress, recoverable checkpoints, provider compatibility,
and targeted file reads.

## Runtime Boundary

Production execution has one path: `kernel_v3` through `runTurnEngine`. Historical
kernel mode values are accepted only while old rows are decoded and are normalized
to `kernel_v3`; they never select another runtime. Legacy messages, questions and
attachments are handled by read-only migration/projection code and cannot affect
new tool selection, recovery or completion state.

## Canonical Turn Items And Ledgers

The `agent_turn_items` table is the only model-history source. Every user input
creates a turn and every model/tool transition is stored in global sequence
order. Legacy `messages` remain the UI and audit projection.

Persisted item kinds are:

- `user_message`
- `assistant_message`
- `tool_call`
- `tool_result`
- `environment_snapshot`
- `workspace_observation`
- `external_observation`
- `file_change`
- `task_plan`
- `compaction`
- `aborted`

Intermediate planning/commentary stays in the item stream and diagnostics. The
main chat should show user messages, final assistant replies, and actionable
error cards.

The kernel also records these internal ledgers on each run checkpoint:

- `AgentKernelState`: kernel version, read targets, and recent progress
  events/items.
- `ActionProgressLedger`: semantic progress counters and the last successful
  read-progress round.
- `AgentLoopGuard`: repeated action and no-progress guard state.
- `ToolResultLedger`: bounded tool-result metadata for diagnostics and context
  compaction.

Before building provider context, ordered items are normalized:

- assistant tool calls without results get an `aborted` tool result.
- orphan tool results without a matching call are dropped.
- normalization actions are written into compaction diagnostics.

Tool progress is keyed by the real target, not only by tool name. For example:

- HTML note block reads use `path + mode + view + doc_hash + block_id`.
- HTML note section reads use `path + mode + view + doc_hash + section_id`.
- Plain text reads include the requested or returned line range.
- Batch text reads include the normalized path set and range.

This prevents a long note task from being stopped just because the model reads
different blocks from the same file.

## HTML Note Read Flow

For `.html` notes, the stable read path is:

1. Use `read_workspace_note_view` with `mode=index`.
2. Pick a `block_id` or `section_id`.
3. Read the target with `mode=block` or `mode=section`.
4. Use `view=raw` only when exact HTML is needed.

`mode=index` is paged. It returns `start_block`, `returned_blocks`,
`total_blocks`, `has_more_blocks`, and `next_start_block`. This keeps the block
list structured even when the note is large. Provider-budget trimming must trim
whole index blocks from the end and preserve the continuation pointer; it must
not head/tail truncate the index into plain text.

Every index/block/section read returns `doc_hash` and `ledger_id`. Block and
section reads bind IDs to `doc_hash`; if the file changes, stale `doc_hash`
values are rejected and the model must re-index.

Line ranges are still valid for ordinary text files, but they are not the right
continuation strategy for rich-text HTML fragments because a short note can have
very long HTML lines.

## Editor Environment And Task Working Set

The editor sends paths only: active file, visible editor files, open tabs, and
panel position. File bodies, HTML fragments, previews, selections, and cached
editor content are excluded unless the user explicitly attaches content.

Successful reads and explicit file references create `workspace_observation`
items. Mutations create `file_change` items. This session working set is not
filtered by the currently active tab, so switching from a whitepaper to a misc
note does not erase the whitepaper task. A mutation marks an older exact read
stale; its path and purpose remain known, but precise editing requires a reread.

## Context Policy

`ContextAssembler` is the only provider-context entry. Its fixed order is:

1. Load canonical items and the active compaction boundary.
2. Repair tool pairs and omit orphan results.
3. Add path-only turn environment and the session task working set.
4. Estimate tokens for system prompts, tools, messages, environment, results,
   and reserved output.
5. Prune only eligible old tool-result bodies after the configured token threshold or an explicit provider context-overflow response.
6. If token pressure remains, write a semantic `compaction` item and retain up
   to 20,000 tokens of the latest complete turn suffix.
7. Serialize the same semantic items to OpenAI or Anthropic protocol shapes.

There is no message-count, single-message character, or 48KB chat-history
limit. Request bytes are diagnostic unless a provider explicitly declares a
transport limit. Original item rows remain stored after pruning or compaction.

When a note read is truncated for the model, the next action must point back to
`mode=index -> mode=block/section`, not to line-based continuation.

## Diagnostics

Provider request summaries include:

- `kernel_version`
- `kernel_mode`
- `kernel_progress_events`
- `kernel_read_targets`
- note index/block/section read counts
- largest tool results and ledger truncation state
- loop guard state and action-selection state

These fields are visible in Settings -> Diagnostics and in diagnostic exports.

## Reference Policy

Codex CLI / SDK / App Server, Roo Code, and similar mature agents are reference
projects for specific behaviors: bounded context, tool-result ledgers,
compaction, checkpoint-style recovery, provider compatibility, and targeted file
reads.

Perseus production runs must not depend on `codex exec` or a Codex App Server
sidecar as a black-box backend. Any reference behavior must be implemented in
the Perseus-native loop with an explicit test or documented gap.
