import { API_BASE_URL, apiGet, apiPost } from '../api/client.js';
import {
  agentEventText,
  appendRunProgressItem,
  buildAgentRecoveryNotice,
  buildAssistantRunShell,
  buildContextCompactionNotice,
  buildProgressItem,
  hasActiveRunShell,
  hasStreamingAssistantMessage,
  insertRunMessageAfterAnchor,
  isRunShellMessage,
  mapPendingAssistant,
  mergeCommittedRunStart,
  mergeAssistantMessageCompleted,
  mergeAssistantMessageStarted,
  nextLoadingStatus,
  normalizeContextUsageSummary,
  normalizeConversationMessages,
  mergeRunUserMessage,
  updateLoadingState,
} from './chatStateUtils.js';
import { mergeConversationSnapshotPreservingActive } from './chatMessageMerge.js';
import { markRunSteerApplied, markRunSteerReceived, markRunSteerRejected } from './chatSteerItems.js';
import { scheduleDelay, startInterval, stopInterval, wait } from './chatRunStreamTimers.js';
import { shouldEnsureProgressShell, toolEventFromPayload } from './chatRunStreamToolEvents.js';
import { chatFailureMetadata } from './chatErrorUtils.js';

export async function waitForAgentRunStream({
  afterSeq = 0,
  control,
  conversationID,
  currentConversationIdRef,
  userMessageId = '',
  loadConversations,
  onAssistantToolEvents,
  onAssistantPreviewEvent,
  onTasksChanged,
  runId,
  selectedModel,
  setAgentRecoveryNotice,
  setChatError,
  setContextCompactionNotice,
  setContextUsage,
  setConversationMessages,
  setConversations,
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
  let activeUserMessageId = userMessageId || '';

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
  let lastEventSeq = Number(afterSeq) || 0;
  let highestQueuedSeq = lastEventSeq;
  let reconnectAttempts = 0;
  let reconnectScheduled = false;
  let replayComplete = false;
  let pendingTerminalRun = null;
  let finalSeq = 0;
  let eventChain = Promise.resolve();
  const closeSource = () => {
    if (source) {
      source.close();
      source = null;
    }
    if (loadingTimer) {
      stopInterval(loadingTimer);
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
  const refreshConversationsAfterTitleGeneration = async () => {
    for (const delay of [1000, 3000, 6000, 10000, 15000, 15000]) {
      await wait(delay);
      await loadConversations();
    }
  };
  control.abort = () => {
    closeSource();
    settled = true;
    apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {
      source: 'stop_button',
      reason: 'user_requested_stop',
    }).catch((error) => {
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
  const applyUserRequest = (payload = {}) => {
    const question = (payload.question || '').trim();
    if (!question) return;
    const toolName = payload.tool_name || 'ask_followup_question';
    const kind = userRequestMessageKind(toolName);
    const attachment = {
      tool_call_id: payload.tool_call_id || '',
      tool_name: toolName,
      status: payload.status || 'waiting_for_user',
      label: payload.label || userRequestLabel(toolName),
      question,
      options: toolName === 'request_user_input' ? [] : payload.options || [],
    };
    setVisibleConversationMessages((current) =>
      current.map((messageItem) => {
        const isTarget =
          messageItem.role === 'assistant' &&
          (messageItem.id === thinkingId || messageItem.activeMessageId || messageItem.pending);
        if (!isTarget) return messageItem;
        return {
          ...messageItem,
          kind,
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
      if (hasStreamingAssistantMessage(current, runId)) return current;
      return insertRunMessageAfterAnchor(current, buildAssistantRunShell({
        id: thinkingId,
        conversationId: conversationID,
        createdAt: new Date().toISOString(),
        content: '',
        attachments: [],
        runId,
      }), activeUserMessageId, runId);
    });
  };
  const finish = async () => {
    closeSource();
    setVisibleConversationMessages((current) =>
      (current || []).filter((item) => !isRunShellMessage(item, { thinkingId, runId }))
    );
    await loadConversations();
    refreshConversationsAfterTitleGeneration().catch((error) => {
      console.error('refreshConversationsAfterTitleGeneration failed', error);
    });
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
    Object.assign(error, chatFailureMetadata(metadata));
    settled = true;
    throw error;
  };

  const handleRun = async (run) => {
    if (!run) return;
    finalSeq = Math.max(finalSeq, Number(run.final_seq) || 0);
    if (['succeeded', 'waiting_for_user', 'delegated', 'failed', 'canceled'].includes(run.status)) {
      pendingTerminalRun = run;
      if (run.final_seq === undefined) replayComplete = true;
    }
  };

  const finishIfTerminalReady = async () => {
    if (!pendingTerminalRun || !replayComplete || lastEventSeq < finalSeq) return false;
    const run = pendingTerminalRun;
    pendingTerminalRun = null;
    if (run.status === 'failed' || run.status === 'canceled') {
      await refreshFromLatestAssistantMessage();
      await fail(run.error_message || '执行失败', run.diagnostic_log_id, {
        stage: run.stage || '',
        ...chatFailureMetadata(run),
      });
    }
    await finish();
    return true;
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
      if (payload.user_message) {
        setVisibleConversationMessages((current) => mergeRunUserMessage(current, payload.user_message, activeUserMessageId, runId));
        setRunSteerItems?.((current) => markRunSteerApplied(current, payload, { conversationId: conversationID, runId }));
      }
    } else if (item?.type === 'agent_steer_applied') {
      setRunSteerItems?.((current) => markRunSteerApplied(current, payload, { conversationId: conversationID, runId }));
      if (payload.user_message) {
        setVisibleConversationMessages((current) => mergeRunUserMessage(current, payload.user_message, activeUserMessageId, runId));
      }
      clearVisibleRunSteerNotice();
    } else if (item?.type === 'agent_steer_rejected') {
      setRunSteerItems?.((current) => markRunSteerRejected(current, payload, { conversationId: conversationID, runId }));
      clearVisibleRunSteerNotice();
    } else if (item?.type === 'agent_steer_canceled') {
      const messageId = payload.message_id || '';
      if (messageId) {
        setVisibleConversationMessages((current) => (current || []).filter((messageItem) => messageItem.id !== messageId));
      }
      clearVisibleRunSteerNotice();
    } else if (item?.type === 'user_response_answered' || item?.type === 'user_question_answered') {
      if (payload.user_message) {
        setVisibleConversationMessages((current) => {
          const updated = current.map((messageItem) => ({
            ...messageItem,
            attachments: (messageItem.attachments || []).map((attachment) =>
              attachment?.tool_call_id === payload.tool_call_id
                ? { ...attachment, status: 'answered', answer_message_id: payload.answer_message_id || '' }
                : attachment
            ),
          }));
          return mergeRunUserMessage(updated, payload.user_message, activeUserMessageId, runId);
        });
      }
    }
    if (item?.type === 'user_message_committed') {
      activeUserMessageId = payload.user_message?.id || activeUserMessageId;
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
    } else if (item?.type === 'assistant_message_started' || item?.type === 'assistant_commentary_started') {
      setVisibleConversationMessages((current) => mergeAssistantMessageStarted(current, thinkingId, payload.assistant_message, { runId }));
    } else if (item?.type === 'assistant_message_delta' || item?.type === 'assistant_commentary_delta') {
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
    } else if (item?.type === 'assistant_message_discarded') {
      const messageId = payload.message_id;
      if (messageId) {
        setVisibleConversationMessages((current) => current.filter((messageItem) => messageItem.id !== messageId));
      }
    } else if (item?.type === 'assistant_commentary_interrupted' && payload.assistant_message) {
      setVisibleConversationMessages((current) =>
        mergeAssistantMessageCompleted(current, thinkingId, payload.assistant_message, { runId })
      );
    } else if (item?.type?.startsWith('workspace_edit_preview_')) {
      await onAssistantPreviewEvent?.({ ...payload, type: item.type });
    } else if (item?.type === 'tool_call_completed') {
      if (isUserRequestToolName(payload.tool_name)) {
        applyUserRequest(payload);
        return;
      }
      if (payload.status === 'success' && payload.tool_name) {
        await onAssistantToolEvents?.([toolEventFromPayload(payload)]);
      }
      appendProgress(item?.type, payload, message);
    } else if (item?.type === 'model_message_started') {
      ensureThinkingShell();
    } else if (item?.type === 'user_input_requested' || item?.type === 'user_choice_requested' || item?.type === 'user_question_requested') {
      applyUserRequest(payload);
    } else if (message && !['assistant_message_completed', 'assistant_commentary_completed', 'assistant_message_presentation_updated', 'run_completed', 'model_delta'].includes(item?.type)) {
      appendProgress(item?.type, payload, message);
    }
    if (item?.type === 'assistant_message_completed' || item?.type === 'assistant_commentary_completed') {
      sawAssistantCompletion = true;
      if (payload.assistant_message) {
        setVisibleConversationMessages((current) => mergeAssistantMessageCompleted(current, thinkingId, payload.assistant_message, { runId }));
        await onAssistantToolEvents?.(payload.assistant_message.attachments || []);
        if (!settled) ensureThinkingShell();
      }
      if (payload.task) {
        onTasksChanged?.(payload.task);
      }
    } else if (item?.type === 'assistant_message_presentation_updated') {
      if (payload.assistant_message) {
        setVisibleConversationMessages((current) =>
          mergeAssistantMessageCompleted(current, thinkingId, payload.assistant_message, { runId })
        );
        await onAssistantToolEvents?.(payload.assistant_message.attachments || []);
      }
    } else if (item?.type === 'workspace_file_changed') {
      if (payload.status === 'success' && payload.tool_name) {
        await onAssistantToolEvents?.([toolEventFromPayload(payload, item.type)]);
      }
    } else if (item?.type === 'run_failed') {
      pendingTerminalRun = {
        status: 'failed',
        error_message: payload.error || '执行失败',
        diagnostic_log_id: payload.diagnostic_log_id || '',
        stage: payload.stage || '',
        incomplete_reason: payload.incomplete_reason || '',
        ...chatFailureMetadata(payload),
      };
    } else if (item?.type === 'conversation_title_updated') {
      if (payload.conversation) {
        setConversations?.((current) =>
          current.map((conversation) =>
            conversation.id === payload.conversation.id ? { ...conversation, ...payload.conversation } : conversation
          )
        );
      } else if (payload.title) {
        setConversations?.((current) =>
          current.map((conversation) =>
            conversation.id === conversationID ? { ...conversation, title: payload.title } : conversation
          )
        );
      }
      await loadConversations();
    }
  };

  const pollUntilDone = async () => {
    fallbackStarted = true;
    for (let attempt = 0; attempt < 120 && !settled; attempt += 1) {
      const data = await apiGet(
        `/api/agent-runs/${encodeURIComponent(runId)}/events?after_seq=${encodeURIComponent(lastEventSeq)}`
      );
      for (const item of data.items || []) {
        const seq = Number(item?.seq) || 0;
        if (seq <= lastEventSeq) continue;
        if (seq !== lastEventSeq + 1) break;
        await handleEvent(item);
        lastEventSeq = seq;
        highestQueuedSeq = seq;
      }
      await handleRun(data.run);
      replayComplete = true;
      if (await finishIfTerminalReady()) return;
      await wait(2000);
    }
    throw new Error('执行仍在进行，但事件流连接中断，请刷新会话查看结果');
  };

  const runQueued = (handler, reject) => {
    eventChain = eventChain.then(handler, handler);
    eventChain.catch(reject);
  };

  loadingTimer = startInterval(() => {
    if (settled) return;
    setVisibleConversationMessages((current) =>
      mapPendingAssistant(current, thinkingId, (item) => updateLoadingState(item), runId)
    );
  }, 80);

  await new Promise((resolve, reject) => {
    const openStream = () => {
      if (settled) return;
      reconnectScheduled = false;
      replayComplete = false;
      highestQueuedSeq = lastEventSeq;
      const resumeSeq = lastEventSeq > 0 ? lastEventSeq : Number(afterSeq) || 0;
      const streamPath = `${API_BASE_URL}/api/agent-runs/${encodeURIComponent(runId)}/stream${
        resumeSeq ? `?after_seq=${encodeURIComponent(resumeSeq)}` : ''
      }`;
      source = new EventSource(streamPath, {
        withCredentials: true,
      });
      control.abort = () => {
        closeSource();
        settled = true;
        apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/cancel`, {
          source: 'stop_button',
          reason: 'user_requested_stop',
        }).catch((error) => {
          console.error('cancelAgentRun failed', error);
        });
        const error = new DOMException('Aborted', 'AbortError');
        reject(error);
      };
      source.addEventListener('run', (event) => {
        runQueued(async () => {
          await handleRun(JSON.parse(event.data));
          if (await finishIfTerminalReady()) resolve();
        }, reject);
      });
      source.addEventListener('stream_opened', (event) => {
        const payload = JSON.parse(event.data);
        runQueued(async () => {
          finalSeq = Math.max(finalSeq, Number(payload.final_seq) || 0);
          await handleRun(payload.run);
        }, reject);
      });
      source.addEventListener('replay_complete', (event) => {
        const payload = JSON.parse(event.data);
        runQueued(async () => {
          finalSeq = Math.max(finalSeq, Number(payload.final_seq) || 0);
          replayComplete = Number(payload.through_seq) <= lastEventSeq;
          if (await finishIfTerminalReady()) resolve();
        }, reject);
      });
      source.addEventListener('event', (event) => {
        const item = JSON.parse(event.data);
        if (item?.seq) {
          const seq = Number(item.seq);
          if (seq <= lastEventSeq || seq <= highestQueuedSeq) return;
          if (seq !== highestQueuedSeq + 1) {
            closeSource();
            highestQueuedSeq = lastEventSeq;
            if (!reconnectScheduled) {
              reconnectScheduled = true;
              eventChain.finally(() => scheduleDelay(openStream, 50));
            }
            return;
          }
          highestQueuedSeq = seq;
        }
        runQueued(async () => {
          await handleEvent(item);
          reconnectAttempts = 0;
          if (item?.seq) {
            lastEventSeq = Number(item.seq);
          }
          if (await finishIfTerminalReady()) resolve();
        }, reject);
      });
      source.addEventListener('preview', (event) => {
        const item = JSON.parse(event.data);
        runQueued(async () => {
          const payload = item?.payload || {};
          const applied = await onAssistantPreviewEvent?.({ ...payload, type: item.type });
          if (applied === false) {
            const state = await apiGet(`/api/agent-runs/${encodeURIComponent(runId)}/previews`);
            for (const preview of state.items || []) {
              await onAssistantPreviewEvent?.(preview);
            }
          }
        }, reject);
      });
      source.onerror = () => {
        closeSource();
        if (settled || fallbackStarted || reconnectScheduled) return;
        if (reconnectAttempts < 3) {
          reconnectAttempts += 1;
          reconnectScheduled = true;
          scheduleDelay(openStream, Math.min(500 * reconnectAttempts, 1500));
          return;
        }
        pollUntilDone().then(resolve).catch(reject);
      };
    };
    openStream();
  });
}

function isUserRequestToolName(toolName = '') {
  return toolName === 'request_user_input' || toolName === 'request_user_choice' || toolName === 'ask_followup_question';
}

function userRequestMessageKind(toolName = '') {
  if (toolName === 'request_user_input') return 'user_input_request';
  if (toolName === 'request_user_choice') return 'user_choice_request';
  return 'followup_question';
}

function userRequestLabel(toolName = '') {
  if (toolName === 'request_user_input') return '等待用户输入';
  if (toolName === 'request_user_choice') return '等待用户选择';
  return '已询问用户';
}
