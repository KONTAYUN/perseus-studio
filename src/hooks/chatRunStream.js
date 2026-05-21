import { API_BASE_URL, apiGet, apiPost } from '../api/client';
import {
  agentEventText,
  appendRunProgressItem,
  buildAgentRecoveryNotice,
  buildAssistantRunShell,
  buildContextCompactionNotice,
  buildProgressItem,
  hasActiveRunShell,
  hasStreamingAssistantMessage,
  mapPendingAssistant,
  mergeCommittedRunStart,
  mergeAssistantMessageCompleted,
  mergeAssistantMessageStarted,
  nextLoadingStatus,
  normalizeContextUsageSummary,
  normalizeConversationMessages,
  updateLoadingState,
} from './chatStateUtils.js';
import { mergeConversationSnapshotPreservingActive } from './chatMessageMerge.js';
import { markRunSteerApplied, markRunSteerReceived, markRunSteerRejected } from './chatSteerItems.js';

function toolEventFromPayload(payload = {}) {
  return {
    tool_name: payload.tool_name,
    status: payload.status || 'success',
    path: payload.path || '',
    old_path: payload.old_path || '',
    new_path: payload.new_path || '',
    changed_paths: payload.changed_paths || [],
    hash: payload.hash || '',
    version_id: payload.version_id || '',
  };
}

export async function waitForAgentRunStream({
  afterSeq = 0,
  control,
  conversationID,
  currentConversationIdRef,
  loadConversations,
  onAssistantToolEvents,
  onTasksChanged,
  runId,
  selectedModel,
  setAgentRecoveryNotice,
  setChatError,
  setContextCompactionNotice,
  setContextUsage,
  setConversationMessages,
  setRunSteerItems,
  setRunSteerNotice,
  thinkingId,
}) {
  const isRunConversationVisible = () => (currentConversationIdRef?.current || '') === conversationID;
  const setVisibleConversationMessages = (updater) => {
    setConversationMessages((current) => (isRunConversationVisible() ? updater(current) : current));
  };
  const clearVisibleRunSteerNotice = () => {
    if (isRunConversationVisible()) {
      setRunSteerNotice?.(null);
    }
  };

  setVisibleConversationMessages((current) =>
    mapPendingAssistant(current, thinkingId, (item) => ({
      ...item,
      loadingState: {
        text: nextLoadingStatus(0),
        visibleText: nextLoadingStatus(0),
        stage: 'holding',
        activeSince: Date.now(),
      },
    }), runId)
  );

  let source = null;
  let settled = false;
  let fallbackStarted = false;
  let sawAssistantCompletion = false;
  let loadingTimer = null;
  const closeSource = () => {
    if (source) {
      source.close();
      source = null;
    }
    if (loadingTimer) {
      window.clearInterval(loadingTimer);
      loadingTimer = null;
    }
  };
  const refreshFromLatestAssistantMessage = async () => {
    try {
      const data = await apiGet(`/api/conversations/${encodeURIComponent(conversationID)}/messages`);
      const messages = normalizeConversationMessages(data.items || []);
      if (isRunConversationVisible()) {
        setConversationMessages((current) => mergeConversationSnapshotPreservingActive(current, messages));
      }
      const lastAssistant = [...messages].reverse().find((message) => message.role === 'assistant');
      await onAssistantToolEvents?.(lastAssistant?.attachments || []);
    } catch (error) {
      console.error('refreshFromLatestAssistantMessage failed', error);
    }
  };
  control.abort = () => {
    closeSource();
    settled = true;
    apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {}).catch((error) => {
      console.error('cancelAgentRun failed', error);
    });
  };

  const appendProgress = (type, payload = {}, fallbackText = '') => {
    const nextItem = buildProgressItem(type, payload, fallbackText);
    if (!nextItem) return;
    nextItem.runId = runId;
    if (shouldEnsureProgressShell(type)) {
      ensureThinkingShell();
    }
    setVisibleConversationMessages((current) => appendRunProgressItem(current, thinkingId, nextItem));
  };
  const applyFollowupQuestion = (payload = {}) => {
    const question = (payload.question || '').trim();
    if (!question) return;
    const attachment = {
      tool_call_id: payload.tool_call_id || '',
      tool_name: 'ask_followup_question',
      status: payload.status || 'success',
      label: payload.label || '已询问用户',
      question,
      options: payload.options || [],
      allow_freeform: payload.allow_freeform !== false,
    };
    setVisibleConversationMessages((current) =>
      current.map((messageItem) => {
        const isTarget =
          messageItem.role === 'assistant' &&
          (messageItem.id === thinkingId || messageItem.activeMessageId || messageItem.pending);
        if (!isTarget) return messageItem;
        return {
          ...messageItem,
          content: question,
          attachments: [attachment],
        };
      })
    );
  };
  const ensureThinkingShell = () => {
    setVisibleConversationMessages((current) => {
      if (current.some((item) => item.id === thinkingId)) return current;
      if (hasActiveRunShell(current, runId)) return current;
      if (hasStreamingAssistantMessage(current)) return current;
      return [
        ...current,
        buildAssistantRunShell({
          id: thinkingId,
          conversationId: conversationID,
          createdAt: new Date().toISOString(),
          content: '',
          attachments: [],
          runId,
        }),
      ];
    });
  };
  const finish = async () => {
    closeSource();
    await loadConversations();
    if (fallbackStarted || !sawAssistantCompletion) {
      await refreshFromLatestAssistantMessage();
    }
    settled = true;
  };

  const fail = (message, diagnosticLogId, metadata = {}) => {
    closeSource();
    const error = new Error(message || '执行失败');
    if (diagnosticLogId) error.diagnosticLogId = diagnosticLogId;
    error.runId = runId;
    error.conversationId = conversationID;
    error.stage = metadata.stage || '';
    error.incompleteReason = metadata.incompleteReason || '';
    settled = true;
    throw error;
  };

  const handleRun = async (run) => {
    if (!run) return;
    if (run.status === 'succeeded') {
      await finish();
    } else if (run.status === 'failed' || run.status === 'canceled') {
      await refreshFromLatestAssistantMessage();
      await fail(run.error_message || '执行失败', run.diagnostic_log_id, {
        stage: run.stage || '',
      });
    }
  };

  const handleEvent = async (item) => {
    const payload = item?.payload || {};
    const message = agentEventText(item?.type, payload) || payload.message;
    if (item?.type === 'model_request_summary') {
      if (isRunConversationVisible()) {
        setContextUsage(normalizeContextUsageSummary(payload, selectedModel));
      }
    } else if (item?.type === 'context_compaction_started' || item?.type === 'context_compaction_completed' || item?.type === 'context_compaction_failed') {
      if (isRunConversationVisible()) {
        setContextCompactionNotice(buildContextCompactionNotice(item.type, payload));
      }
    } else if (item?.type === 'agent_recovery_started' || item?.type === 'agent_recovery_retrying') {
      if (isRunConversationVisible()) {
        setAgentRecoveryNotice(buildAgentRecoveryNotice(item.type, payload));
        setChatError('');
      }
    } else if (item?.type === 'agent_recovery_completed' || item?.type === 'agent_recovery_failed') {
      if (isRunConversationVisible()) {
        setAgentRecoveryNotice(buildAgentRecoveryNotice(item.type, payload));
      }
    } else if (item?.type === 'agent_steer_received') {
      setRunSteerItems?.((current) => markRunSteerReceived(current, payload, { conversationId: conversationID, runId }));
    } else if (item?.type === 'agent_steer_applied') {
      setRunSteerItems?.((current) => markRunSteerApplied(current, payload, { conversationId: conversationID, runId }));
      clearVisibleRunSteerNotice();
    } else if (item?.type === 'agent_steer_rejected') {
      setRunSteerItems?.((current) => markRunSteerRejected(current, payload, { conversationId: conversationID, runId }));
      clearVisibleRunSteerNotice();
    }
    if (item?.type === 'user_message_committed') {
      setVisibleConversationMessages((current) =>
        mergeCommittedRunStart(current, {
          thinkingId,
          userMessage: payload.user_message,
          assistantShell: buildAssistantRunShell({
            id: thinkingId,
            conversationId: conversationID,
            createdAt: new Date().toISOString(),
            content: '',
            attachments: [],
            runId,
          }),
        })
      );
    } else if (item?.type === 'assistant_message_started') {
      setVisibleConversationMessages((current) => mergeAssistantMessageStarted(current, thinkingId, payload.assistant_message, { runId }));
    } else if (item?.type === 'assistant_message_delta') {
      const messageId = payload.message_id;
      const delta = payload.delta || '';
      if (messageId && delta) {
        setVisibleConversationMessages((current) =>
          current.map((messageItem) =>
            (messageItem.id === thinkingId && messageItem.activeMessageId === messageId) || messageItem.id === messageId
              ? {
                  ...messageItem,
                  content: `${messageItem.content || ''}${delta}`,
                  pending: true,
                }
              : messageItem
          )
        );
      }
    } else if (item?.type === 'tool_call_completed') {
      if (payload.tool_name === 'ask_followup_question') {
        applyFollowupQuestion(payload);
        return;
      }
      if (payload.status === 'success' && payload.tool_name) {
        await onAssistantToolEvents?.([toolEventFromPayload(payload)]);
      }
      appendProgress(item?.type, payload, message);
    } else if (item?.type === 'model_message_started') {
      ensureThinkingShell();
    } else if (item?.type === 'user_question_requested') {
      applyFollowupQuestion(payload);
    } else if (message && !['assistant_message_completed', 'run_completed', 'model_delta'].includes(item?.type)) {
      appendProgress(item?.type, payload, message);
    }
    if (item?.type === 'assistant_message_completed') {
      sawAssistantCompletion = true;
      if (payload.assistant_message) {
        setVisibleConversationMessages((current) => mergeAssistantMessageCompleted(current, thinkingId, payload.assistant_message, { runId }));
        await onAssistantToolEvents?.(payload.assistant_message.attachments || []);
      }
      if (payload.task) {
        onTasksChanged?.(payload.task);
      }
    } else if (item?.type === 'workspace_file_changed') {
      if (payload.status === 'success' && payload.tool_name) {
        await onAssistantToolEvents?.([toolEventFromPayload(payload)]);
      }
    } else if (item?.type === 'run_failed') {
      await refreshFromLatestAssistantMessage();
      await fail(payload.error || '执行失败', payload.diagnostic_log_id, {
        stage: payload.stage || '',
        incompleteReason: payload.incomplete_reason || '',
      });
    }
  };

  const pollUntilDone = async () => {
    fallbackStarted = true;
    for (let attempt = 0; attempt < 120 && !settled; attempt += 1) {
      const run = await apiGet(`/api/agent-runs/${encodeURIComponent(runId)}`);
      await handleRun(run);
      if (settled) return;
      await new Promise((resolve) => window.setTimeout(resolve, 2000));
    }
    throw new Error('执行仍在进行，但事件流连接中断，请刷新会话查看结果');
  };

  loadingTimer = window.setInterval(() => {
    if (settled) return;
    setVisibleConversationMessages((current) =>
      mapPendingAssistant(current, thinkingId, (item) => updateLoadingState(item), runId)
    );
  }, 80);

  await new Promise((resolve, reject) => {
    const streamPath = `${API_BASE_URL}/api/agent-runs/${encodeURIComponent(runId)}/stream${
      afterSeq ? `?after_seq=${encodeURIComponent(afterSeq)}` : ''
    }`;
    source = new EventSource(streamPath, {
      withCredentials: true,
    });
    control.abort = () => {
      closeSource();
      settled = true;
      apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {}).catch((error) => {
        console.error('cancelAgentRun failed', error);
      });
      const error = new DOMException('Aborted', 'AbortError');
      reject(error);
    };
    source.addEventListener('run', (event) => {
      Promise.resolve(handleRun(JSON.parse(event.data)))
        .then(() => {
          if (settled) resolve();
        })
        .catch(reject);
    });
    source.addEventListener('event', (event) => {
      Promise.resolve(handleEvent(JSON.parse(event.data)))
        .then(() => {
          if (settled) resolve();
        })
        .catch(reject);
    });
    source.onerror = () => {
      closeSource();
      if (!fallbackStarted && !settled) {
        pollUntilDone().then(resolve).catch(reject);
      }
    };
  });
}

function shouldEnsureProgressShell(type = '') {
  return type.startsWith('tool_call_') || type === 'tool_scope_rejected' || type === 'completion_rejected' || type === 'local_persistence_failed';
}
