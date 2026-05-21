import { PLAYFUL_PENDING_STATUSES, agentEventText, nextLoadingStatus } from './chatStatusTextUtils.js';

export function buildAssistantRunShell({ id, conversationId, createdAt, content = '', attachments = [], runId = '' } = {}) {
  return {
    id,
    runId,
    conversation_id: conversationId || 'pending',
    role: 'assistant',
    kind: 'run_shell',
    content,
    attachments,
    pending: true,
    created_at: createdAt || new Date().toISOString(),
    progressItems: [],
    loadingState: {
      text: PLAYFUL_PENDING_STATUSES[0],
      visibleText: PLAYFUL_PENDING_STATUSES[0],
      stage: 'holding',
      activeSince: Date.now(),
    },
    activeMessageId: '',
  };
}

export function mergeCommittedRunStart(
  messages,
  {
    optimisticUserId = '',
    thinkingId = '',
    userMessage,
    assistantShell,
  } = {}
) {
  const committedUser = normalizeConversationMessage(userMessage);
  if (!committedUser?.id || !thinkingId) return messages || [];
  const shellRunId = assistantShell?.runId || '';
  const existingShell = (messages || []).find((item) => isPendingRunShell(item, thinkingId, shellRunId));
  const shell = normalizeRunShell(
    existingShell
      ? { ...(assistantShell || {}), ...existingShell, runId: existingShell.runId || shellRunId }
      : assistantShell,
    thinkingId,
    committedUser.conversation_id
  );
  const withoutPlaceholders = removeRunStartPlaceholders(messages, {
    optimisticUserId,
    thinkingId,
    committedUser,
  });
  const userIndex = findCommittedUserIndex(withoutPlaceholders, committedUser);
  const nextMessages = [...withoutPlaceholders];
  const existingShellIndex = nextMessages.findIndex(
    (item) => isPendingRunShell(item, thinkingId, shell.runId)
  );
  const insertUserIndex = userIndex >= 0 ? userIndex : nextMessages.length;
  if (userIndex >= 0) {
    nextMessages[insertUserIndex] = {
      ...nextMessages[insertUserIndex],
      ...committedUser,
    };
  } else {
    nextMessages.splice(insertUserIndex, 0, committedUser);
  }
  if (existingShellIndex >= 0) {
    nextMessages[existingShellIndex] = {
      ...nextMessages[existingShellIndex],
      ...shell,
    };
    if (existingShellIndex !== nextMessages.length - 1) {
      const [existingShellMessage] = nextMessages.splice(existingShellIndex, 1);
      nextMessages.push(existingShellMessage);
    }
  } else {
    nextMessages.push(shell);
  }
  return nextMessages;
}

export function appendUniqueProgressItem(items, nextItem) {
  if (!nextItem?.id) return items;
  if ((items || []).some((item) => item.id === nextItem.id)) return items;
  return [...(items || []), nextItem];
}

export function appendRunProgressItem(messages, thinkingId, nextItem) {
  if (!thinkingId || !nextItem?.id) return messages;
  let targetIndex = findPendingAssistantIndex(messages, nextItem.runId);
  if (targetIndex < 0) {
    targetIndex = findPendingRunShellIndex(messages, thinkingId, nextItem.runId);
  }
  if (targetIndex < 0) return messages;
  const nextMessages = [...(messages || [])];
  nextMessages[targetIndex] = {
    ...nextMessages[targetIndex],
    progressItems: appendUniqueProgressItem(nextMessages[targetIndex].progressItems, nextItem),
  };
  return nextMessages;
}

export function mergeAssistantMessageStarted(messages, thinkingId, assistantMessage, { runId = '' } = {}) {
  const assistant = normalizeConversationMessage(assistantMessage);
  if (!assistant) return messages;
  const normalizedMessages = messages || [];
  const existingAssistant = normalizedMessages.find((item) => item?.id === assistant.id);
  if (existingAssistant && isFinalAssistantMessage(existingAssistant)) {
    return messages || [];
  }
  const nextAssistant = {
    ...assistant,
    runId,
    pending: true,
    activeMessageId: assistant.id,
    progressItems: [],
  };
  if (existingAssistant) {
    return normalizedMessages.map((item) =>
      item?.id === assistant.id
        ? {
            ...item,
            ...nextAssistant,
            loadingState: item.loadingState || nextAssistant.loadingState,
            progressItems: item.progressItems || [],
          }
        : item
    );
  }
  const shellIndex = findPendingRunShellIndex(normalizedMessages, thinkingId, runId);
  if (shellIndex >= 0) {
    const nextMessages = [...normalizedMessages];
    nextMessages[shellIndex] = {
      ...nextMessages[shellIndex],
      ...nextAssistant,
      loadingState: nextMessages[shellIndex].loadingState || nextAssistant.loadingState,
      progressItems: nextMessages[shellIndex].progressItems || [],
    };
    return nextMessages;
  }
  return [...normalizedMessages, nextAssistant];
}

export function mergeAssistantMessageCompleted(messages, thinkingId, assistantMessage, { runId = '' } = {}) {
  const assistant = normalizeConversationMessage(assistantMessage);
  if (!assistant) return messages;
  const normalizedMessages = messages || [];
  const nextAssistant = {
    ...assistant,
    runId,
    pending: false,
    activeMessageId: '',
    progressItems: [],
  };
  if (normalizedMessages.some((item) => item?.id === assistant.id)) {
    return normalizedMessages.map((item) => (item?.id === assistant.id ? { ...item, ...nextAssistant } : item));
  }
  const shellIndex = findPendingRunShellIndex(normalizedMessages, thinkingId, runId);
  if (shellIndex >= 0) {
    const nextMessages = [...normalizedMessages];
    nextMessages[shellIndex] = {
      ...nextMessages[shellIndex],
      ...nextAssistant,
    };
    return nextMessages;
  }
  return [...normalizedMessages, nextAssistant];
}

export function buildProgressItem(type, payload = {}, fallbackText = '') {
  const text = agentEventText(type, payload) || payload.message || fallbackText || '';
  if (!text) return null;
  return {
    id: payload.event_id || `${type}:${payload.tool_call_id || payload.round || payload.path || payload.message || payload.reason || text}`,
    runId: payload.run_id || payload.runId || '',
    type,
    text,
    toolName: payload.tool_name || '',
    status: payload.status || '',
  };
}

export function normalizeConversationMessage(message) {
  if (!message) return message;
  const next = {
    attachments: [],
    ...message,
  };
  if (next.meta?.kind && !next.kind) {
    next.kind = next.meta.kind;
  }
  if (next.meta?.status && !next.status) {
    next.status = next.meta.status;
  }
  if (next.role === 'assistant' && next.status === 'streaming') {
    next.pending = true;
    next.activeMessageId = next.id;
    next.loadingState = next.loadingState || {
      text: nextLoadingStatus(0),
      visibleText: nextLoadingStatus(0),
      stage: 'holding',
      activeSince: Date.now(),
    };
  }
  return next;
}

export function normalizeConversationMessages(items) {
  return (items || []).map((item) => normalizeConversationMessage(item));
}

export function appendRunTerminalMessage(messages, latestRun) {
  if (!isRecoverableTerminalRun(latestRun)) return messages || [];
  if ((messages || []).some((item) => item.resumableRunId === latestRun.id)) return messages || [];
  const message = String(latestRun.error_message || '').trim() || '上一轮任务没有正常完成，可继续本轮。';
  return [
    ...(messages || []),
    {
      id: `run_error_${latestRun.id}`,
      conversation_id: latestRun.conversation_id,
      role: 'assistant',
      content: latestRun.status === 'canceled' ? '这轮任务已停止，可继续本轮。' : message,
      attachments: [],
      created_at: latestRun.completed_at || latestRun.updated_at || new Date().toISOString(),
      pending: false,
      error: true,
      resumableRunId: latestRun.id,
      diagnosticLogId: latestRun.diagnostic_log_id || '',
    },
  ];
}

export function hasStreamingAssistantMessage(items) {
  return (items || []).some((item) => item?.role === 'assistant' && (item?.status === 'streaming' || item?.activeMessageId));
}

export function mapPendingAssistant(current, thinkingId, transform, runId = '') {
  let targetIndex = findPendingAssistantIndex(current, runId);
  if (targetIndex < 0) {
    targetIndex = findPendingRunShellIndex(current, thinkingId, runId);
  }
  if (targetIndex < 0) return current;
  const next = [...current];
  next[targetIndex] = transform(next[targetIndex]);
  return next;
}

export function hasActiveRunShell(items, runId) {
  if (!runId) return false;
  return (items || []).some((item) => isPendingRunShell(item, '', runId));
}

export function isRunShellMessage(item, { thinkingId = '', runId = '' } = {}) {
  return isPendingRunShell(item, thinkingId, runId);
}

function normalizeRunShell(shell, thinkingId, conversationId) {
  const runId = shell?.runId || '';
  const base = buildAssistantRunShell({
    id: runId || thinkingId,
    conversationId,
    createdAt: new Date().toISOString(),
    runId,
  });
  return {
    ...base,
    ...(shell || {}),
    id: runId || thinkingId,
    runId,
    conversation_id: conversationId || shell?.conversation_id || base.conversation_id,
    role: 'assistant',
    pending: true,
  };
}

function removeRunStartPlaceholders(messages, { optimisticUserId, thinkingId, committedUser }) {
  return (messages || []).filter((item) => {
    if (!item) return false;
    if (item.id === thinkingId) return false;
    if (item.id === optimisticUserId) return false;
    if (item.id === committedUser.id) return true;
    if (
      item.role === 'user' &&
      item.id?.startsWith?.('temp_user_') &&
      item.conversation_id === committedUser.conversation_id &&
      item.content === committedUser.content
    ) {
      return false;
    }
    return true;
  });
}

function findCommittedUserIndex(messages, committedUser) {
  return (messages || []).findIndex((item) => item.id === committedUser.id);
}

function isRecoverableTerminalRun(run) {
  if (!run?.id) return false;
  if (run.status !== 'failed' && run.status !== 'canceled') return false;
  if (!run.user_message_id) return false;
  return true;
}

function isPendingRunShell(item, thinkingId = '', runId = '') {
  if (!item || item.role !== 'assistant' || !item.pending) return false;
  if (thinkingId && item.id === thinkingId) return true;
  if (!runId || item.runId !== runId) return false;
  return item.kind === 'run_shell' || !item.activeMessageId;
}

function findPendingRunShellIndex(messages, thinkingId = '', runId = '') {
  const items = messages || [];
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (isPendingRunShell(items[index], thinkingId, runId)) return index;
  }
  return -1;
}

function findPendingAssistantIndex(messages, runId = '') {
  const items = messages || [];
  for (let index = items.length - 1; index >= 0; index -= 1) {
    if (isPendingAssistantForRun(items[index], runId)) return index;
  }
  return -1;
}

function isPendingAssistantForRun(item, runId = '') {
  if (!item || item.role !== 'assistant' || !item.pending) return false;
  if (!runId) return true;
  return item.runId === runId && item.activeMessageId;
}

function isFinalAssistantMessage(item) {
  if (!item || item.role !== 'assistant') return false;
  if (item.pending || item.activeMessageId) return false;
  if (item.kind === 'run_shell') return false;
  return item.status === 'final' || item.status === undefined || item.status === '';
}
