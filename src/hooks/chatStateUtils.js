import {
  agentEventText,
  completedToolStatus,
  nextLoadingStatus,
  pendingToolStatus,
  playfulPendingStatus,
  updateLoadingState,
} from './chatStatusTextUtils.js';
import {
  normalizeConversationMessage,
  normalizeConversationMessages as normalizeRunConversationMessages,
} from './chatRunMessageState.js';

export {
  agentEventText,
  completedToolStatus,
  nextLoadingStatus,
  pendingToolStatus,
  playfulPendingStatus,
  updateLoadingState,
};

export {
  appendRunProgressItem,
  appendRunTerminalMessage,
  appendUniqueProgressItem,
  buildAssistantRunShell,
  buildProgressItem,
  hasActiveRunShell,
  hasStreamingAssistantMessage,
  insertRunMessageAfterAnchor,
  isRunShellMessage,
  mapPendingAssistant,
  mergeAssistantMessageCompleted,
  mergeAssistantMessageStarted,
  mergeCommittedRunStart,
  mergeRunUserMessage,
  moveRunMessageAfterAnchor,
} from './chatRunMessageState.js';

export { normalizeConversationMessage };

export function normalizeConversationMessages(items) {
  return normalizeRunConversationMessages(items);
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
