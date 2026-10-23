export function toolEventFromPayload(payload = {}, eventType = '') {
  return {
    type: payload.type || eventType || '',
    tool_call_id: payload.tool_call_id || '',
    tool_name: payload.tool_name,
    status_kind: payload.status_kind || '',
    status: payload.status || 'success',
    label: payload.label || '',
    path: payload.path || '',
    old_path: payload.old_path || '',
    new_path: payload.new_path || '',
    changed_paths: Array.isArray(payload.changed_paths) ? payload.changed_paths : [],
    moved: Array.isArray(payload.moved) ? payload.moved : [],
    changes: Array.isArray(payload.changes) ? payload.changes : [],
    operation: payload.operation || '',
    operation_hash: payload.operation_hash || '',
    hash: payload.hash || '',
    version_id: payload.version_id || '',
  };
}

export function shouldEnsureProgressShell(type = '') {
  return (
    type.startsWith('tool_call_') ||
    type === 'tool_scope_rejected' ||
    type === 'completion_rejected' ||
    type === 'local_persistence_failed'
  );
}
