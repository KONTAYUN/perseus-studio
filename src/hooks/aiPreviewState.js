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

export function mergePreviewTabByIdentity(tabs, previewID, path, preview) {
  const currentTabs = tabs || [];
  const previewIndex = currentTabs.findIndex(
    (tab) => tab.aiPreview?.preview_id === previewID
  );
  if (previewIndex < 0) return null;
  const previousPath = currentTabs[previewIndex].path;
  const collisionIndex = currentTabs.findIndex(
    (tab, index) => index !== previewIndex && tab.path === path
  );
  if (collisionIndex >= 0) {
    return {
      previousPath,
      tabs: currentTabs
        .filter((_, index) => index !== previewIndex)
        .map((tab) =>
          tab.path === path
            ? { ...tab, tabId: tab.tabId || previewID, aiPreview: preview }
            : tab
        ),
    };
  }
  return {
    previousPath,
    tabs: currentTabs.map((tab, index) =>
      index === previewIndex
        ? {
            ...tab,
            tabId: tab.tabId || previewID,
            path,
            previewSourcePath: path,
            aiPreview: preview,
          }
        : tab
    ),
  };
}

export function removeOrphanedMutationPreviewTabs(tabs, event = {}) {
  const mutationID = event.mutation_id || '';
  const changedPaths = new Set(
    (event.changed_paths?.length ? event.changed_paths : [event.path]).filter(Boolean)
  );
  if (!mutationID || changedPaths.size === 0) return tabs || [];
  return (tabs || []).filter((tab) => {
    if (tab.aiPreview?.mutation_id !== mutationID || !tab.previewOnly) return true;
    const targetPath = tab.aiPreview?.path || tab.previewSourcePath || tab.path;
    return changedPaths.has(targetPath);
  });
}
