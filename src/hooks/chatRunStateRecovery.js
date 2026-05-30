import {
  appendRunTerminalMessage,
  buildAssistantRunShell,
  buildProgressItem,
  insertRunMessageAfterAnchor,
  isRunShellMessage,
  moveRunMessageAfterAnchor,
  nextLoadingStatus,
  normalizeConversationMessage,
} from './chatStateUtils.js';

export function applyLatestRunState(messages, latestRun, events = [], options = {}) {
  const terminalMessages = appendRunTerminalMessage(messages, latestRun);
  if (!isActiveRun(latestRun)) return terminalMessages;

  const snapshot = buildRunProgressSnapshot(events, latestRun);
  const shellId = options.thinkingId || `run_active_${latestRun.id}`;
  const userMessageId = latestRun.user_message_id || '';
  const shell = {
    ...buildAssistantRunShell({
      id: shellId,
      conversationId: latestRun.conversation_id,
      createdAt: latestRun.updated_at || latestRun.started_at || latestRun.created_at,
      runId: latestRun.id,
    }),
    kind: 'run_shell',
    runId: latestRun.id,
    progressItems: snapshot.progressItems,
    activeMessageId: snapshot.activeMessageId,
    loadingState: {
      text: snapshot.loadingText,
      visibleText: snapshot.loadingText,
      stage: 'holding',
      activeSince: Date.now(),
    },
  };

  const normalized = (terminalMessages || []).map((item) => normalizeConversationMessage(item));
  const activeMessageIndex = snapshot.activeMessageId
    ? normalized.findIndex((item) => item.id === snapshot.activeMessageId)
    : -1;
  if (activeMessageIndex >= 0) {
    const activeMessageID = snapshot.activeMessageId;
    const nextMessages = normalized
      .filter((item) => !isRunShellMessage(item, { thinkingId: shellId, runId: latestRun.id }))
      .map((item) => {
      if (item.id !== activeMessageID) return item;
      return {
        ...item,
        pending: true,
        runId: latestRun.id,
        activeMessageId: activeMessageID,
        progressItems: snapshot.progressItems,
        loadingState: {
          text: snapshot.loadingText,
          visibleText: snapshot.loadingText,
          stage: 'holding',
          activeSince: Date.now(),
        },
      };
      });
    return nextMessages;
  }

  const persistedActiveIndex = findPersistedStreamingAssistantIndex(normalized, userMessageId, latestRun.id);
  if (persistedActiveIndex >= 0) {
    const activeMessageID = normalized[persistedActiveIndex].id;
    return normalized
      .filter((item, index) => index === persistedActiveIndex || !isRunShellMessage(item, { thinkingId: shellId, runId: latestRun.id }))
      .map((item) => {
        if (item.id !== activeMessageID) return item;
        return {
          ...item,
          pending: true,
          runId: latestRun.id,
          activeMessageId: activeMessageID,
          progressItems: item.progressItems || snapshot.progressItems,
          loadingState: item.loadingState || {
            text: snapshot.loadingText,
            visibleText: snapshot.loadingText,
            stage: 'holding',
            activeSince: Date.now(),
          },
        };
      });
  }

  const shellIndex = normalized.findIndex((item) => isRunShellMessage(item, { thinkingId: shellId, runId: latestRun.id }));
  if (shellIndex >= 0) {
    const nextMessages = normalized.map((item, index) => {
      if (index !== shellIndex) return item;
      return {
        ...item,
        ...shell,
      };
    });
    return moveRunMessageAfterAnchor(nextMessages, shellIndex, userMessageId, latestRun.id);
  }

  return insertRunMessageAfterAnchor(normalized, shell, userMessageId, latestRun.id);
}

function isActiveRun(run) {
  if (!run?.id || !run.user_message_id) return false;
  return run.status === 'running' || run.status === 'queued';
}

function buildRunProgressSnapshot(events = [], run = {}) {
  const activeTools = new Map();
  let activeMessageId = '';
  let loadingText = runStageLoadingText(run.stage) || nextLoadingStatus(events.length);

  for (const event of events || []) {
    const type = event?.type || '';
    const payload = event?.payload || {};
    if (type === 'assistant_message_started') {
      activeMessageId = payload.assistant_message?.id || activeMessageId;
    } else if (type === 'assistant_message_completed' && payload.assistant_message?.id === activeMessageId) {
      activeMessageId = '';
    }

    const progress = buildProgressItem(type, payload, payload.message || '');
    if (type === 'tool_call_started' && progress) {
      progress.runId = run.id;
      activeTools.set(payload.tool_call_id || progress.id, progress);
      loadingText = progress.text;
    } else if ((type === 'tool_call_completed' || type === 'tool_call_failed') && payload.tool_call_id) {
      activeTools.delete(payload.tool_call_id);
      loadingText = type === 'tool_call_failed'
        ? (progress?.text || '正在重新尝试')
        : (runStageLoadingText(run.stage) || nextLoadingStatus(events.length));
    } else if (progress && isRunSnapshotLoadingType(type)) {
      loadingText = progress.text;
    }
  }

  const progressItems = Array.from(activeTools.values()).slice(-1);
  if (progressItems[0]?.text) {
    loadingText = progressItems[0].text;
  }
  return {
    activeMessageId,
    loadingText: loadingText || nextLoadingStatus(events.length),
    progressItems: progressItems.map((item) => ({ ...item, runId: run.id })),
  };
}

function isRunSnapshotLoadingType(type = '') {
  return (
    type.startsWith('model_call_') ||
    type === 'model_message_started' ||
    type === 'model_stream_recovering' ||
    type === 'context_compaction_started' ||
    type === 'context_compaction_failed' ||
    type === 'agent_recovery_started' ||
    type === 'agent_recovery_retrying' ||
    type === 'agent_budget_exhausted' ||
    type === 'agent_budget_summary_started' ||
    type === 'agent_budget_summary_failed' ||
    type === 'completion_rejected' ||
    type === 'local_persistence_failed'
  );
}

function runStageLoadingText(stage = '') {
  switch (stage) {
    case 'queued':
      return '等待执行';
    case 'calling_model':
      return '正在等待模型响应';
    case 'executing_tool':
      return '正在执行工具';
    case 'waiting_model_after_tool':
      return '正在整理工具结果';
    case 'saving_message':
      return '正在保存回复';
    case 'retrying_model':
      return '正在重新连接模型';
    default:
      return '';
  }
}

function findPersistedStreamingAssistantIndex(messages = [], userMessageId = '', runId = '') {
  if (!userMessageId) return -1;
  const userIndex = messages.findIndex((item) => item?.id === userMessageId);
  if (userIndex < 0) return -1;
  let activeIndex = -1;
  for (let index = userIndex + 1; index < messages.length; index += 1) {
    const item = messages[index];
    if (item?.role === 'user') break;
    if (isPersistedStreamingAssistant(item, runId)) {
      activeIndex = index;
    }
  }
  return activeIndex;
}

function isPersistedStreamingAssistant(item, runId = '') {
  if (!item || item.role !== 'assistant') return false;
  if (item.kind === 'run_shell') return false;
  if (runId && item.runId && item.runId !== runId) return false;
  return item.status === 'streaming' || Boolean(item.activeMessageId);
}
