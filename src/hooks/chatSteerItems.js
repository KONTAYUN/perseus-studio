export const RUN_STEER_STATUS = {
  submitting: 'submitting',
  received: 'received',
  applied: 'applied',
  failed: 'failed',
};

const RUN_STEER_STATUS_ORDER = {
  [RUN_STEER_STATUS.submitting]: 0,
  [RUN_STEER_STATUS.received]: 1,
  [RUN_STEER_STATUS.applied]: 2,
  [RUN_STEER_STATUS.failed]: 3,
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
      return '正在引导';
    case RUN_STEER_STATUS.received:
      return '正在引导';
    case RUN_STEER_STATUS.applied:
      return '已引导对话';
    case RUN_STEER_STATUS.failed:
      return '引导未送达';
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

export function removeRunSteerItems(items, itemIds = []) {
  const ids = new Set((itemIds || []).filter(Boolean));
  if (ids.size === 0) return items || [];
  return (items || []).filter((item) => !ids.has(item.id) && !ids.has(item.clientSteerId) && !ids.has(item.steerId));
}

export function unappliedRunSteerItems(items) {
  return (items || []).filter(
    (item) =>
      item?.content &&
      (item.status === RUN_STEER_STATUS.submitting || item.status === RUN_STEER_STATUS.received)
  );
}

export function pendingRunSteerPreviewItems(items) {
  return (items || []).filter(
    (item) =>
      item?.content &&
      !item.messageId &&
      (item.status === RUN_STEER_STATUS.submitting ||
        item.status === RUN_STEER_STATUS.received ||
        item.status === RUN_STEER_STATUS.failed)
  );
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

export function visibleRunSteerTimelineItems(runSteerItems, conversationId) {
  const targetConversationId = String(conversationId || '').trim();
  if (!targetConversationId) return [];
  return (runSteerItems || []).filter((item) => {
    if (item?.conversation_id !== targetConversationId) return false;
    return item.status !== RUN_STEER_STATUS.applied;
  });
}

export function buildAppliedRunSteerItemsFromEvents(events = [], { conversationId = '', runId = '' } = {}) {
  const receivedBySteer = new Map();
  const appliedItems = [];
  for (const event of events || []) {
    const type = event?.type || '';
    if (type !== 'agent_steer_received' && type !== 'agent_steer_applied') continue;
    const payload = event.payload || {};
    const steerId = String(payload.steer_id || '').trim();
    const clientSteerId = String(payload.client_steer_id || '').trim();
    const key = steerId || clientSteerId || String(event.id || '').trim();
    if (!key) continue;
    if (type === 'agent_steer_received') {
      if (payload.user_message || payload.message_id) {
        continue;
      }
      receivedBySteer.set(key, {
        content: String(payload.content || payload.content_preview || '').trim(),
        createdAt: payload.created_at || event.created_at || '',
        clientSteerId,
        steerId,
      });
      continue;
    }
    const received = receivedBySteer.get(key) || {};
    const content = String(payload.content || payload.content_preview || received.content || '').trim();
    if (!content) continue;
    appliedItems.push({
      ...buildRunSteerItem({
        clientSteerId: clientSteerId || received.clientSteerId || key,
        content,
        conversationId: event.conversation_id || conversationId,
        runId: event.run_id || runId,
        createdAt: payload.created_at || event.created_at || received.createdAt,
      }),
      id: `steer_${event.id || key}`,
      steerId: steerId || received.steerId || '',
      status: RUN_STEER_STATUS.applied,
    });
  }
  return appliedItems;
}

export function reconcileRunSteerItems(items, nextItems = []) {
  let current = items || [];
  for (const nextItem of nextItems || []) {
    if (!nextItem?.content) continue;
    const merged = mergeRunSteerItem(current, nextItem);
    current = merged === current ? appendRunSteerItem(current, nextItem) : merged;
  }
  return current;
}

function mergeRunSteerEvent(items, eventPayload = {}, fallback = {}) {
  const clientSteerId = String(eventPayload.client_steer_id || fallback.clientSteerId || '').trim();
  const steerId = String(eventPayload.steer_id || fallback.steerId || '').trim();
  const content = String(eventPayload.content || eventPayload.content_preview || fallback.content || '').trim();
  const createdAt = String(eventPayload.created_at || fallback.createdAt || '').trim();
  const messageId = String(eventPayload.message_id || eventPayload.user_message?.id || fallback.messageId || '').trim();
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
      conversation_id: fallback.conversationId,
      runId: fallback.runId,
      status,
      content,
      error: fallback.error,
      created_at: createdAt,
      messageId,
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
    messageId,
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
