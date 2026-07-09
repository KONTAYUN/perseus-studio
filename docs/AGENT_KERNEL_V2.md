# Perseus Agent Kernel v2

Agent Kernel v2 is the stability layer for long workspace tasks. It is not a new
storage format and it is not a Codex CLI wrapper. The goal is to make Perseus
behave like mature agent runners: bounded context, explicit tool progress,
recoverable checkpoints, and targeted file reads.

## Runtime Switch

`PERSEUS_AGENT_KERNEL_MODE` controls the execution guard behavior:

- `kernel_v2` is the default.
- `legacy` keeps the previous loop-guard target behavior and disables v2 read
  progress tracking.

The switch is intentionally coarse so a local deployment can roll back the new
target/progress accounting without changing user data or workspace files.

## Core Ledgers

Kernel v2 records these internal ledgers on each run checkpoint:

- `AgentKernelState`: kernel version/mode, read targets, and recent progress
  events.
- `ActionProgressLedger`: semantic progress counters and the last successful
  read-progress round.
- `AgentLoopGuard`: repeated action and no-progress guard state.
- `ToolResultLedger`: bounded tool-result metadata for diagnostics and context
  compaction.

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

Every index/block/section read returns `doc_hash` and `ledger_id`. If the file
changes, stale `doc_hash` values are rejected and the model must re-index.

Line ranges are still valid for ordinary text files, but they are not the right
continuation strategy for rich-text HTML fragments because a short note can have
very long HTML lines.

## Context Policy

The model-visible context stays bounded:

- Large tool results are compacted to metadata/reference records.
- Current read working set and recent relevant blocks are preserved.
- Older exact payloads are replaced with instructions to re-read the target when
  exact text is needed.
- On checkpoint resume or overflow retry, the message builder can compact before
  the next provider call.

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

## Codex Sidecar Policy

Codex CLI / SDK / App Server behavior is the reference model for this kernel:
bounded context, tool-result ledgers, compaction, checkpoint-style recovery, and
targeted file reads.

A Codex sidecar may be used as a non-production comparison harness for real
failure cases, but Perseus production runs must not depend on `codex exec` as a
black-box backend. Any sidecar experiment should expose Perseus-style tools and
compare behavior against Kernel v2 before changing production code.

