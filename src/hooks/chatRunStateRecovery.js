import {
  appendRunTerminalMessage,
  buildAssistantRunShell,
  buildProgressItem,
  nextLoadingStatus,
} from './chatStateUtils.js';

export function applyLatestRunState(messages, latestRun, events = [], options = {}) {
  const terminalMessages = appendRunTerminalMessage(messages, latestRun);
  if (!isActiveRun(latestRun)) return terminalMessages;

  const snapshot = buildRunProgressSnapshot(events, latestRun);
  const shellId = options.thinkingId || `run_active_${latestRun.id}`;
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

  let updated = false;
  const nextMessages = (terminalMessages || []).map((item) => {
    if (item.id === shellId || item.runId === latestRun.id || (item.role === 'assistant' && item.pending && item.activeMessageId)) {
      updated = true;
      return {
        ...item,
        ...shell,
        id: item.id || shell.id,
        content: item.content || shell.content,
        attachments: item.attachments || [],
        activeMessageId: item.activeMessageId || shell.activeMessageId,
      };
    }
    return item;
  });
  if (updated) return nextMessages;
  return [...nextMessages, shell];
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
