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
    operation: payload.operation || '',
    mutation_id: payload.mutation_id || '',
    origin: payload.origin || '',
    authoritative: payload.authoritative === true,
    operation_hash: payload.operation_hash || '',
    hash: payload.hash || '',
    file_hash: payload.file_hash || payload.hash || '',
    doc_hash: payload.doc_hash || '',
    document_version: payload.document_version || payload.doc_hash || '',
    version_id: payload.version_id || '',
    changed_blocks: Array.isArray(payload.changed_blocks) ? payload.changed_blocks : [],
    validation_status: payload.validation_status || '',
    structure_delta: payload.structure_delta || null,
    history_nodes_created: Number(payload.history_nodes_created || 0),
  };
}

export function shouldEnsureProgressShell(type = '') {
  return (
    type.startsWith('tool_call_') ||
    type === 'tool_scope_rejected' ||
    type === 'completion_rejected' ||
	type === 'agent_protocol_retry' ||
    type === 'local_persistence_failed'
  );
}
