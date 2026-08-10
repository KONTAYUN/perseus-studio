export function mergeAIEditPreview(current, event) {
  if (!current || current.preview_id !== event.preview_id || event.snapshot_full !== false) {
    return { ...event, changes: event.changed_blocks || event.changes || [], status: 'previewing' };
  }
  const changes = new Map(
    (current.changes || []).map((change) => [String(change?.block_id || ''), change])
  );
  for (const blockID of event.removed_blocks || []) {
    changes.delete(String(blockID || ''));
  }
  for (const change of event.changed_blocks || event.changes || []) {
    changes.set(String(change?.block_id || ''), change);
  }
  return {
    ...current,
    ...event,
    changes: Array.from(changes.values()),
    status: 'previewing',
  };
}

export function canApplyAIEditPreviewDelta(current, event) {
  if (!current || current.preview_id !== event.preview_id || event.snapshot_full !== false) {
    return true;
  }
  return Number(event.base_revision || 0) === Number(current.revision || 0);
}
