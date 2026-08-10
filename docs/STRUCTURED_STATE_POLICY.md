# Structured State Policy

## Rule

Production behavior must be driven by typed state, error codes, manifests, protocol
fields, and parsed syntax. User-visible text is presentation only. It must not
decide task completion, retries, permissions, tool side effects, provider
capabilities, or UI status.

## Required Sources

- Agent completion: typed turn, tool, mutation, verification, and task-plan state.
- Tool behavior: `ToolManifestEntry`.
- Tool failures: `ToolError`.
- Provider failures: `ProviderResponseError`, `ProviderFailure`, network error
  types, and HTTP headers.
- Model behavior: explicit adapter, authentication, tool, vision, and context
  capability fields.
- UI state: event types, status enums, HTTP status, and stable error codes.
- Database conflicts: driver error codes.
- Command safety: parsed `CommandAction` values from the POSIX shell AST.

Unknown state remains unknown. Code must not guess a stronger classification from
localized or provider-authored prose.

## Allowed Matching

String and pattern matching remains valid when the text itself is the data being
processed:

- Protocol syntax such as SSE event fields and provider wire formats.
- HTML, CSS, Markdown, MIME, patch, and file-format parsing.
- Workspace search and user-requested content search.
- Path normalization and extension checks.
- Exact executable, flag, and path policies after shell AST parsing.
- Exact provider error codes and types received in structured fields.

## Compatibility Boundary

Provider-authored text is never used to classify retries, limits, execution
state, or capabilities. A compatibility endpoint that supplies no structured
HTTP status, error code, or error type is normalized to
`provider_unknown_failure`; the original text is retained only in protected
diagnostics.

Legacy provider URL inspection is allowed only inside the idempotent database
migration that materializes explicit `adapter_mode` and `auth_mode` fields. New
requests read those persisted fields and never inspect the URL at runtime.

## Prohibited Patterns

- `strings.Contains(err.Error(), ...)` or equivalent localized error parsing.
- `error.message.includes(...)` to select frontend behavior.
- Inferring mutation or risk from a tool name.
- Inferring model capabilities or authentication from a model name or Base URL.
- Inferring success, failure, waiting, or retry state from notification text.

The structure test enforces the common forms of these patterns. A necessary
exception must be narrowly scoped, documented here, and include an explicit
removal condition.
