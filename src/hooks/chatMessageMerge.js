import { normalizeConversationMessages } from './chatStateUtils.js';

export function mergeConversationSnapshotPreservingActive(currentMessages, snapshotMessages) {
  const current = currentMessages || [];
  const snapshot = normalizeConversationMessages(snapshotMessages || []);
  if (current.length === 0) return snapshot;

  const snapshotById = new Map(snapshot.map((item) => [item.id, item]));
  const consumed = new Set();
  const merged = current.map((item) => {
    const snapshotItem = snapshotById.get(item.id);
    if (!snapshotItem) return item;
    consumed.add(item.id);
    if (shouldKeepActiveMessageContent(item, snapshotItem)) {
      return {
        ...snapshotItem,
        content: item.content,
        pending: item.pending,
        activeMessageId: item.activeMessageId,
        loadingState: item.loadingState || snapshotItem.loadingState,
        progressItems: item.progressItems || snapshotItem.progressItems || [],
        runId: item.runId || snapshotItem.runId || '',
      };
    }
    return snapshotItem;
  });

  for (const item of snapshot) {
    if (!consumed.has(item.id)) {
      merged.push(item);
    }
  }
  return merged.sort(compareMessagesByCreatedAt);
}

function shouldKeepActiveMessageContent(current, snapshot) {
  if (!current?.pending) return false;
  if (current.role !== 'assistant' || snapshot?.role !== 'assistant') return false;
  const currentContent = String(current.content || '');
  if (currentContent === '') return false;
  const snapshotContent = String(snapshot.content || '');
  return currentContent.length >= snapshotContent.length || !snapshotContent.startsWith(currentContent);
}

function compareMessagesByCreatedAt(a, b) {
  const aTime = Date.parse(a?.created_at || '') || 0;
  const bTime = Date.parse(b?.created_at || '') || 0;
  if (aTime !== bTime) return aTime - bTime;
  return String(a?.id || '').localeCompare(String(b?.id || ''));
}
