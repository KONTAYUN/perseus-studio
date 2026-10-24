# Perseus Project Health and Evidence

Document image upload/library and conservative resource collection were verified on 2026-10-24. Evidence and limits: [DOCUMENT_IMAGES_20261024.md](DOCUMENT_IMAGES_20261024.md). Editor uploads are hidden workspace resources; ordinary user images remain visible and outside collection. History/trash references and a persisted seven-day grace period protect recovery.

Document editing closure is in progress; see DOCUMENT_EDITING_IMPLEMENTATION.md. New adapter/session, backend document policy and constrained repair boundaries are authorized behavior changes. Previous test totals are historical evidence, not acceptance of this work.

This document records behavioral evidence and structure standards. Numeric self-ratings have been retired; they did not measure correctness, security or release readiness.

## Design audit remediation — 2026-10-21

[DESIGN_AUDIT_REMEDIATION.md](DESIGN_AUDIT_REMEDIATION.md) is the current per-finding implementation and evidence record. [PROJECT_DESIGN_AUDIT.md](PROJECT_DESIGN_AUDIT.md) retains the pre-fix counterexamples. A finding is not closed merely because a type or setting exists.

| Behavior / failure | Evidence level in this round | Remaining boundary |
|---|---|---|
| Prose examples never execute, actual structured edits work | Independent SQLite/mock provider regressions; both protocol browser editing; live Grok editing | Not every provider/model |
| Completion and error facts survive wording/compaction/resume | Runtime/typed-error/large-result/checkpoint negative tests | No general natural-language truth verifier |
| Event persistence failure stops subsequent tools | Independent SQLite trigger fault | Prior side effects cannot be undone by cancel |
| Patch process-exit recovery preserves user changes | Real child-process exits after four mutation stages, reopen/recover, conflict test | No power-loss ACID or entire-turn checkpoint |
| Exact-file scope checks real source/destination | Independent workspace positive/negative mutations, resume intersection | No complete authorization UI or historical context redaction |
| Host commands require explicit trusted administrator opt-in | Disabled policy and common-executor bypass regressions | OS sandbox remains unimplemented |
| Terminal message snapshots supersede stale drafts | Three merge sequence regressions and existing recovery browser tests | Full reducer migration and exhaustive reconnect permutations pending |
| Run retry/request/time limits | Actual mock HTTP counts, deadline/Retry-After tests | No monetary/token spend ceiling |
| Markdown/editor/save/animation baseline | Full browser suite: 26 passed, 1 opt-in skipped; final edit subset 3 passed | Rich serialization limits and build chunk size remain |
| Current source validation | Go all packages + vet; frontend 152/152; UI/structure + builds | Linux native/race/container/power-loss/load not run this round |

Logs and manual upgrade/backup evidence are linked from the remediation report. Do not treat previous screenshots, CI configuration, source line count or historical cross-compilation as current native-platform execution evidence.

## 2026-10-20 Functional Audit

Editor polish acceptance adds consistent light/dark scrollbars, a single accessible mode icon, removal of the editor download action, conservative format diagnostics and user-confirmed Agent repair. Regression coverage must verify prompt/file identity, preservation of composer drafts, no automatic submission, no submission after save conflict/failure or target switch, and warning dismissal after valid edits.

Markdown UI work adds shared safe message rendering and a dedicated Markdown document editor. Acceptance must cover code-copy fidelity, unsafe links/HTML, no write on open, rich/source round trips, selection and block formatting, undo, tab identity, mode retention across layout changes, keyboard save, autosave and unsupported-syntax preservation. Existing file conflict handling remains authoritative.

The implementation follow-up introduces a bounded run lifecycle, stronger per-path change verification, recovery checks, and shared motion ownership. These are behavioral reliability changes authorized by the user; they refine the existing boundaries rather than introduce a new application framework. Validation results are recorded in PROJECT_OPTIMIZATION.md.

For the director-level comparison with OpenHands, Cline, OpenCode, and Aider, see [PROJECT_DIRECTOR_REVIEW.md](PROJECT_DIRECTOR_REVIEW.md). It evaluates runtime recovery, change review/undo, contract consistency, state coupling, and operational evidence. Current readiness is assessed through the behavior/failure matrix above.

Previous structure self-ratings did not measure production correctness or security. A functional audit reproduced hidden editing-tool failures despite the original suite passing. See [PROJECT_DIAGNOSIS.md](PROJECT_DIAGNOSIS.md) for the repaired tool chain, save/refresh races, path checks, resource bounds, browser regressions, and outstanding command-isolation and dependency risks. Editor save actions remain within the existing hooks/actions boundary; CORS policy remains inside the API layer. These fixes do not certify complete multi-user sandbox isolation.

## Stable Standard

- Source of truth: [docs/STRUCTURE_STANDARDS.md](STRUCTURE_STANDARDS.md).
- Refactors are zero-behavior unless a task explicitly asks for behavior changes.
- Responsibility boundaries come first; line count is an alarm.
- Production JS/JSX, ordinary production Go, and CSS section files target fewer than 450 lines.
- New structural splits must continue within the documented boundaries. Changing a boundary requires updating `STRUCTURE_STANDARDS.md`, `ARCHITECTURE.md`, and this scorecard first.

## Structure evidence

The former numerical structure self-rating was not calibrated against behavioral reliability and has been removed. Structure gates remain useful maintenance checks: 135 frontend and 300 Go files (including tests) were scanned in this round, with ordinary production sources below the documented 450-line limit. This is not a correctness guarantee. Responsibility boundaries and next refactor targets remain documented below.

## Line Budget Exceptions

No production file currently needs a line-budget exception.

| File | Reason | Target |
|---|---|---|
| None | Structure gate passes without production exceptions. | Keep production files below 450 lines. |

## Largest Watch Files

These are below budget but should not become new dumping grounds:

| File | Current Role | Guidance |
|---|---|---|
| `backend/internal/store/agent_runs.go` | Agent run CRUD/events/checkpoint persistence | Split by status/events/checkpoints if it grows past the alarm. |
| `src/hooks/useIdeState.js` | Top-level frontend state facade | Compose new sub-hooks instead of adding feature logic here. |
| `src/hooks/useChatState.js` | Chat state facade | Keep send queue, stream merge, references, compaction, and resume in existing modules. |
| `src/styles/motion.css` | Shared motion classes and keyframes | Add new motion only when registered and documented. |
| `src/styles/chat-composer.css` | Composer shell/input/action bar/model status styling | Preserve composer visual contract. |
| `src/styles/chat-composer-queue.css` | Pending input/running steer/follow-up styling | Keep running-input queue panels separate from the composer input shell. |

## Mature Agent Boundary Map

Perseus follows the same broad separation used by OpenCode, Cline, Roo/Kilo, and Claude Code style agents:

- **Tool registry:** declares which tool name maps to which implementation.
- **Tool argument ledger:** stores sanitized/truncated arguments separate from model-facing results.
- **Tool runner:** executes the selected tool and normalizes result/error shape.
- **Tool event emitter:** produces started/completed/workspace-changed events for UI and diagnostics.
- **Checkpoint recovery:** handles transient provider failures after provider retry and stream fallback.
- **Workflow tools:** `attempt_completion` and `ask_followup_question` are always available workflow endpoints.

## Required Verification

Inline HTML span compatibility must cover code/bold children, nested spans, quoted `>` attributes, inert event/style attributes, unchanged source on open, and wrapper preservation after editing/saving. Balanced spans have editable child text. Other HTML/frontmatter/math use inert local preservation nodes with explicit local editing, rather than full visual editors for those languages. Existing Markdown serialization may normalize bullet markers, whitespace and entity spelling after an edit.

For structural work, run:

```bash
npm run test:ui
npm run test:structure
npm run test:unit
npm run build
cd backend && go test ./...
```

Before handoff for app-level work, also run:

```bash
npm run test:e2e
docker compose up -d --build --force-recreate
```

Then check:

```text
http://127.0.0.1:8080/api/health
```


### 2026-10-20 admission refinement

Conversation send and regeneration share `store` transactional run preparation (message, run, initial events, request identity) and the API run lifecycle facade. Regeneration truncates the assistant tail only after scheduler admission, immediately before execution. Frontend request controls own cancellation through the submission-to-stream handoff; API request deadlines remain separate from durable run lifetimes. This closes the synchronous regeneration bypass without introducing a second agent runner.


### Verified change review and reversal

Command environment hardening is independently testable: deny ambient service credentials through an explicit runtime allowlist and use a per-user command home. It does not raise host command execution to an isolation guarantee; OS worker isolation remains outstanding.

Single-server ownership is enforced before migration and stale-run recovery with OS file locks on the database and user-data root. Regression checks must cover duplicate ownership, release, and process termination. This is not a multi-replica scheduler or a distributed lease.

Optional title inference has a separate bounded server lifetime; UI title polling is cancellable and independent of run error/recovery controls. Check saturation/de-duplication and timer replacement, including a previous run finishing before the next one fails.

Conversation lifecycle regressions must include deletion racing foreground/background admission, deletion failure preserving messages, orphan resume rejection, and canceled execution cleanup retaining history. Active work returns HTTP 409 on deletion. Matched database/files backup restoration has been exercised locally; this does not replace crash-stage or sustained-load tests.

Interrupted background AI work must never automatically replay without a durable recovery contract. Check restart failure/cancellation and atomic recovery logs. Background AI checkpoint/resume is still an open capability, not implied by foreground run recovery.

`workspace` owns bounded change previews and hash-checked reversal of one persisted tool change. `api/workspace_changes` resolves the authenticated user's tool ledger entry and schedules reversals through the shared workspace queue. A durable reversal journal prevents replay after an ambiguous filesystem/database failure. Chat renders this through a dedicated change-review component; it does not trust client-supplied paths, hashes or snapshots. This is per-file tool-change recovery, not a shell-command or whole-workspace checkpoint.

### Extended Markdown acceptance — 2026-10-23

Require regressions for editable prose beside metadata, HTML, references, footnotes, math, directives and wiki links; lossless protected payload after save/reload; local editing, undo, insertion, clipboard and inert HTML. Opening and section navigation must not write. Evidence and limitations will be recorded in `EXTENDED_MARKDOWN_EDITING_20261023.md`.

Verified delivery: [EXTENDED_MARKDOWN_EDITING_20261023.md](EXTENDED_MARKDOWN_EDITING_20261023.md). 173 unit tests; full Chrome run 83 passed/1 opt-in skipped, final extension subset 11 passed after closing parser edges. Manual 8080 upgrade preserved hashes of 26 tables/7668 rows/109 files. Complex extensions remain local raw-content editors; long-document section changes reset section undo history.
