import { PLAYFUL_PENDING_STATUSES, agentEventText, nextLoadingStatus } from './chatStatusTextUtils.js';

export {
  agentEventText,
  completedToolStatus,
  isPendingStatusText,
  nextLoadingStatus,
  pendingToolStatus,
  playfulPendingStatus,
  updateLoadingState,
} from './chatStatusTextUtils.js';

export function buildAssistantRunShell({ id, conversationId, createdAt, content = '', attachments = [] } = {}) {
  return {
    id,
    conversation_id: conversationId || 'pending',
    role: 'assistant',
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
  const shell = normalizeRunShell(
    (messages || []).find((item) => item.id === thinkingId) || assistantShell,
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
  const insertUserIndex = userIndex >= 0 ? userIndex : nextMessages.length;
  if (userIndex >= 0) {
    nextMessages[insertUserIndex] = {
      ...nextMessages[insertUserIndex],
      ...committedUser,
    };
  } else {
    nextMessages.splice(insertUserIndex, 0, committedUser);
  }
  nextMessages.splice(insertUserIndex + 1, 0, shell);
  return nextMessages;
}

export function appendUniqueProgressItem(items, nextItem) {
  if (!nextItem?.id) return items;
  if ((items || []).some((item) => item.id === nextItem.id)) return items;
  return [...(items || []), nextItem];
}

export function appendRunProgressItem(messages, thinkingId, nextItem) {
  if (!thinkingId || !nextItem?.id) return messages;
  let changed = false;
  const nextMessages = (messages || []).map((item) => {
    const isTarget =
      item.id === thinkingId ||
      (item.role === 'assistant' && item.pending && item.activeMessageId);
    if (!isTarget) return item;
    changed = true;
    return {
      ...item,
      progressItems: appendUniqueProgressItem(item.progressItems, nextItem),
    };
  });
  return changed ? nextMessages : messages;
}

export function mergeAssistantMessageStarted(messages, thinkingId, assistantMessage) {
  const assistant = normalizeConversationMessage(assistantMessage);
  if (!assistant) return messages;
  const nextAssistant = {
    ...assistant,
    pending: true,
    activeMessageId: assistant.id,
    progressItems: [],
  };
  let inserted = false;
  const nextMessages = [];
  for (const item of messages || []) {
    if (item.id !== thinkingId && item.id !== assistant.id) {
      nextMessages.push(item);
      continue;
    }
    if (inserted) continue;
    inserted = true;
    nextMessages.push({
      ...nextAssistant,
      loadingState: item.loadingState || nextAssistant.loadingState,
      progressItems: item.progressItems || [],
    });
  }
  return inserted ? nextMessages : [...(messages || []), nextAssistant];
}

export function mergeAssistantMessageCompleted(messages, thinkingId, assistantMessage) {
  const assistant = normalizeConversationMessage(assistantMessage);
  if (!assistant) return messages;
  const nextAssistant = {
    ...assistant,
    pending: false,
    activeMessageId: '',
    progressItems: [],
  };
  let inserted = false;
  const nextMessages = [];
  for (const item of messages || []) {
    const isActiveShell = item.id === thinkingId && (!item.activeMessageId || item.activeMessageId === assistant.id);
    if (!isActiveShell && item.id !== assistant.id) {
      nextMessages.push(item);
      continue;
    }
    if (inserted) continue;
    inserted = true;
    nextMessages.push(nextAssistant);
  }
  return inserted ? nextMessages : [...(messages || []), nextAssistant];
}

export function buildProgressItem(type, payload = {}, fallbackText = '') {
  const text = agentEventText(type, payload) || payload.message || fallbackText || '';
  if (!text) return null;
  return {
    id: payload.event_id || `${type}:${payload.tool_call_id || payload.round || payload.path || payload.message || payload.reason || text}`,
    type,
    text,
    toolName: payload.tool_name || '',
    status: payload.status || '',
  };
}

export function formatContextTokenCount(value) {
  const tokens = Math.max(0, Math.round(Number(value) || 0));
  if (tokens >= 1000000) {
    const valueM = tokens / 1000000;
    return `${Number.isInteger(valueM) ? valueM : valueM.toFixed(1)}M`;
  }
  if (tokens >= 1000) {
    return `${Math.round(tokens / 1000)}K`;
  }
  return `${tokens}`;
}

function clampPercent(value) {
  const percent = Math.round(Number(value) || 0);
  if (percent < 0) return 0;
  if (percent > 100) return 100;
  return percent;
}

export function normalizeContextUsageSummary(payload = {}, selectedModel = {}) {
  const estimatedTokens = Math.max(0, Math.round(Number(
    payload.estimated_tokens ??
    payload.estimated_tokens_after ??
    payload.estimated_tokens_before ??
    0
  ) || 0));
  const contextWindowTokens = Math.max(0, Math.round(Number(
    payload.context_window_tokens ||
    selectedModel?.context_window_tokens ||
    0
  ) || 0));
  const usedPercent = contextWindowTokens > 0
    ? clampPercent((estimatedTokens * 100) / contextWindowTokens)
    : clampPercent(payload.context_watermark_percent);
  const remainingTokens = contextWindowTokens > 0 ? Math.max(0, contextWindowTokens - estimatedTokens) : 0;
  return {
    round: Number(payload.round) || 0,
    protocol: payload.protocol || selectedModel?.protocol || '',
    messageCount: Number(payload.message_count) || 0,
    transcriptEntries: Number(payload.transcript_entries) || 0,
    toolResultCount: Number(payload.tool_result_count) || 0,
    toolLoopActive: Boolean(payload.tool_loop_active),
    estimatedTokens,
    contextWindowTokens,
    usedPercent,
    remainingPercent: Math.max(0, 100 - usedPercent),
    remainingTokens,
    compactReservedTokens: Math.max(0, Math.round(Number(payload.compact_reserved_tokens) || 0)),
    compactionBudgetTokens: Math.max(0, Math.round(Number(payload.compaction_budget_tokens) || 0)),
    autoCompactEnabled: payload.auto_compact_enabled !== false,
    estimatedToolResultTokens: Math.max(0, Math.round(Number(payload.estimated_tool_result_tokens) || 0)),
    estimatedChars: Math.max(0, Math.round(Number(payload.estimated_chars) || 0)),
    estimatedToolResultChars: Math.max(0, Math.round(Number(payload.estimated_tool_result_chars) || 0)),
    compactionActions: Array.isArray(payload.compaction_actions) ? payload.compaction_actions : [],
    updatedAt: Date.now(),
    source: payload.source || 'live',
    eventId: payload.event_id || '',
    runId: payload.run_id || '',
    createdAt: payload.created_at || '',
  };
}

export function normalizeStoredContextUsage(response = {}, selectedModel = {}) {
  const usage = response?.usage;
  if (!usage?.payload) return null;
  return normalizeContextUsageSummary(
    {
      ...usage.payload,
      source: 'stored',
      event_id: usage.event_id || usage.payload.event_id || '',
      run_id: usage.run_id || usage.payload.run_id || '',
      created_at: usage.created_at || usage.payload.created_at || '',
    },
    selectedModel
  );
}

export function buildContextUsageLabel(usage, notice) {
  if (notice?.status === 'started') return '正在压缩';
  if (notice?.status === 'completed') return notice.reason === 'auto_threshold' ? '已自动压缩' : '已压缩';
  if (notice?.status === 'failed') return '压缩跳过';
  if (!usage) return '';
  return `${usage.usedPercent || 0}%`;
}

export function buildContextUsageTitle(usage) {
  if (!usage) return '上下文状态';
  return usage.source === 'stored' ? '上次请求的上下文窗口' : '背景信息窗口';
}

export function buildContextCompactionNotice(type, payload = {}) {
  const status = type === 'context_compaction_started'
    ? 'started'
    : type === 'context_compaction_completed'
      ? 'completed'
      : 'failed';
  return {
    id: `${type}:${payload.event_id || payload.reason || payload.summary_until_message_id || Date.now()}`,
    type,
    status,
    reason: payload.reason || '',
    text: agentEventText(type, payload),
    compactedMessageCount: Number(payload.compacted_message_count) || 0,
    estimatedTokensBefore: Number(payload.estimated_tokens_before) || 0,
    estimatedTokensAfter: Number(payload.estimated_tokens_after) || 0,
    createdAt: Date.now(),
  };
}

export function buildAgentRecoveryNotice(type, payload = {}) {
  const status = type === 'agent_recovery_started'
    ? 'started'
    : type === 'agent_recovery_retrying'
      ? 'retrying'
      : type === 'agent_recovery_completed'
        ? 'completed'
        : 'failed';
  return {
    id: `${type}:${payload.event_id || payload.diagnostic_id || payload.checkpoint_seq || Date.now()}`,
    type,
    status,
    reason: payload.reason || '',
    attempt: Number(payload.attempt) || 0,
    maxAttempts: Number(payload.max_attempts) || 0,
    checkpointSeq: Number(payload.checkpoint_seq) || 0,
    diagnosticId: payload.diagnostic_id || '',
    text: agentEventText(type, payload),
    createdAt: Date.now(),
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

export function hasStreamingAssistantMessage(items) {
  return (items || []).some((item) => item?.role === 'assistant' && (item?.status === 'streaming' || item?.activeMessageId));
}

export function mapPendingAssistant(current, thinkingId, transform) {
  let updated = false;
  const next = current.map((item) => {
    const isPendingTarget =
      item.id === thinkingId ||
      (item.role === 'assistant' && item.pending);
    if (!isPendingTarget || updated) return item;
    updated = true;
    return transform(item);
  });
  return next;
}

function normalizeRunShell(shell, thinkingId, conversationId) {
  const base = buildAssistantRunShell({
    id: thinkingId,
    conversationId,
    createdAt: new Date().toISOString(),
  });
  return {
    ...base,
    ...(shell || {}),
    id: thinkingId,
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
