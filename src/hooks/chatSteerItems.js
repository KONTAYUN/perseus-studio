export const RUN_STEER_STATUS = {
  submitting: 'submitting',
  received: 'received',
  applied: 'applied',
  failed: 'failed',
};

const RUN_STEER_STATUS_ORDER = {
  [RUN_STEER_STATUS.submitting]: 0,
  [RUN_STEER_STATUS.failed]: 1,
  [RUN_STEER_STATUS.received]: 2,
  [RUN_STEER_STATUS.applied]: 3,
};

export function buildRunSteerItem({
  clientSteerId,
  content,
  conversationId = '',
  runId = '',
  pendingId = '',
  createdAt,
} = {}) {
  const trimmedContent = String(content || '').trim();
  const id = clientSteerId || `client_steer_${Date.now()}`;
  return {
    id,
    clientSteerId: id,
    steerId: '',
    conversation_id: conversationId || 'pending',
    runId,
    role: 'user',
    kind: 'steer',
    status: RUN_STEER_STATUS.submitting,
    content: trimmedContent,
    pendingId,
    error: '',
    created_at: createdAt || new Date().toISOString(),
  };
}

export function runSteerStatusLabel(status) {
  switch (status) {
    case RUN_STEER_STATUS.submitting:
      return '待引导';
    case RUN_STEER_STATUS.received:
      return '待引导';
    case RUN_STEER_STATUS.applied:
      return '已引导对话';
    case RUN_STEER_STATUS.failed:
      return '引导失败';
    default:
      return '';
  }
}

export function appendRunSteerItem(items, nextItem) {
  if (!nextItem?.clientSteerId || !nextItem?.content) return items || [];
  if ((items || []).some((item) => item.clientSteerId === nextItem.clientSteerId)) {
    return mergeRunSteerItem(items, nextItem);
  }
  return [...(items || []), nextItem];
}

export function removeRunSteerItem(items, itemId) {
  return (items || []).filter((item) => item.id !== itemId && item.clientSteerId !== itemId && item.steerId !== itemId);
}

export function markRunSteerFailed(items, itemId, error = '') {
  return updateRunSteerItem(items, itemId, (item) => ({
    ...item,
    status: RUN_STEER_STATUS.failed,
    error: String(error || '').trim(),
  }));
}

export function markRunSteerReceived(items, eventPayload = {}, fallback = {}) {
  return mergeRunSteerEvent(items, eventPayload, {
    ...fallback,
    status: RUN_STEER_STATUS.received,
  });
}

export function markRunSteerApplied(items, eventPayload = {}, fallback = {}) {
  return mergeRunSteerEvent(items, eventPayload, {
    ...fallback,
    status: RUN_STEER_STATUS.applied,
  });
}

export function markRunSteerRejected(items, eventPayload = {}, fallback = {}) {
  return mergeRunSteerEvent(items, eventPayload, {
    ...fallback,
    status: RUN_STEER_STATUS.failed,
    error: eventPayload.message || fallback.error || '引导未应用，请稍后重试。',
  });
}

export function mergeRunSteerItemsWithMessages(messages, runSteerItems) {
  const visibleItems = (runSteerItems || []).filter((item) => item?.content);
  if (visibleItems.length === 0) return messages || [];
  const messageIds = new Set((messages || []).map((message) => message.id));
  const additions = visibleItems.filter((item) => !messageIds.has(item.id));
  return [...(messages || []), ...additions].sort((a, b) => {
    const aTime = Date.parse(a.created_at || '') || 0;
    const bTime = Date.parse(b.created_at || '') || 0;
    if (aTime !== bTime) return aTime - bTime;
    return String(a.id || '').localeCompare(String(b.id || ''));
  });
}

function mergeRunSteerEvent(items, eventPayload = {}, fallback = {}) {
  const clientSteerId = String(eventPayload.client_steer_id || fallback.clientSteerId || '').trim();
  const steerId = String(eventPayload.steer_id || fallback.steerId || '').trim();
  const content = String(eventPayload.content || eventPayload.content_preview || fallback.content || '').trim();
  const createdAt = String(eventPayload.created_at || fallback.createdAt || '').trim();
  const status = fallback.status || RUN_STEER_STATUS.received;
  const key = clientSteerId || steerId;
  if (!key && !content) return items || [];

  let matched = false;
  const nextItems = (items || []).map((item) => {
    if (!isRunSteerMatch(item, { clientSteerId, steerId, content })) return item;
    matched = true;
    return mergeRunSteerItem([item], {
      id: item.id,
      clientSteerId: item.clientSteerId,
      steerId,
      status,
      content,
      error: fallback.error,
      created_at: createdAt,
    })[0];
  });
  if (matched) return nextItems;
  if (!content) return nextItems;
  return appendRunSteerItem(nextItems, {
    ...buildRunSteerItem({
      clientSteerId: key || `client_steer_${Date.now()}`,
      content,
      conversationId: fallback.conversationId,
      runId: fallback.runId,
      createdAt,
    }),
    steerId,
    status,
    error: fallback.error || '',
  });
}

function mergeRunSteerItem(items, nextItem) {
  return updateRunSteerItem(items, nextItem.clientSteerId || nextItem.id || nextItem.steerId, (item) => {
    const nextStatus = strongerRunSteerStatus(item.status, nextItem.status || item.status);
    return {
      ...item,
      ...removeEmptyFields(nextItem),
      status: nextStatus,
      content: nextItem.content || item.content,
      error: nextStatus === RUN_STEER_STATUS.failed ? (nextItem.error || item.error || '') : '',
    };
  });
}

function updateRunSteerItem(items, itemId, updater) {
  let changed = false;
  const nextItems = (items || []).map((item) => {
    if (!isRunSteerMatch(item, { clientSteerId: itemId, steerId: itemId })) return item;
    changed = true;
    return updater(item);
  });
  return changed ? nextItems : items || [];
}

function isRunSteerMatch(item, { clientSteerId = '', steerId = '', content = '' } = {}) {
  return (
    (!!clientSteerId && item.clientSteerId === clientSteerId) ||
    (!!steerId && item.steerId === steerId) ||
    (!!content && item.content === content && item.status === RUN_STEER_STATUS.submitting)
  );
}

function strongerRunSteerStatus(currentStatus, nextStatus) {
  const currentRank = RUN_STEER_STATUS_ORDER[currentStatus] ?? 0;
  const nextRank = RUN_STEER_STATUS_ORDER[nextStatus] ?? 0;
  return nextRank >= currentRank ? nextStatus : currentStatus;
}

function removeEmptyFields(value) {
  return Object.fromEntries(
    Object.entries(value || {}).filter(([, item]) => item !== '' && item !== undefined && item !== null)
  );
}
