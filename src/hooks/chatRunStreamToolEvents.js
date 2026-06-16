export function toolEventFromPayload(payload = {}) {
  return {
    tool_name: payload.tool_name,
    status_kind: payload.status_kind || '',
    status: payload.status || 'success',
    path: payload.path || '',
    old_path: payload.old_path || '',
    new_path: payload.new_path || '',
    changed_paths: payload.changed_paths || [],
    operation: payload.operation || '',
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
