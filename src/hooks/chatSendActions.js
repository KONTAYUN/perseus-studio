import { apiPostFormWithSignal } from '../api/client';
import { chatFailureMetadata, formatChatError } from './chatErrorUtils';
import { buildAssistantRunShell, mapPendingAssistant, mergeCommittedRunStart } from './chatStateUtils.js';

export function createChatSendActions({
  activeAgentRunRef,
  activeChatAbortRef,
  buildCurrentFileContext,
  chatAttachments,
  chatFileReferences,
  chatInput,
  chatSkillReferences,
  compactCurrentConversation,
  conversationTransitionRef,
  createPendingRunMessage,
  currentConversationIdRef,
  currentConversationId,
  ensureConversation,
  hasUnsupportedRuntimeGuidance,
  isChatSending,
  loadConversationMessages,
  loadConversations,
  onAssistantToolEvents,
  onTasksChanged,
  selectedModel,
  setChatAttachments,
  setChatError,
  setChatFileReferences,
  setChatInput,
  setChatSkillReferences,
  setConversationMessages,
  setCurrentConversationId,
  setIsChatSending,
  setQueuedChatMessages,
  setResumableRun,
  thinkingMode,
  waitForAgentRun,
  warnUnsupportedRuntimeGuidance,
}) {
  async function sendChatMessage(overrideContent = null) {
    const usingOverride = typeof overrideContent === 'string';
    const trimmedContent = (usingOverride ? overrideContent : chatInput).trim();
    const attachmentsForSend = usingOverride ? [] : chatAttachments;
    const fileReferencesForSend = usingOverride ? [] : chatFileReferences;
    const skillReferencesForSend = usingOverride ? [] : chatSkillReferences;
    if (!trimmedContent && attachmentsForSend.length === 0 && fileReferencesForSend.length === 0 && skillReferencesForSend.length === 0) return;
    if (/^\/compact(?:\s|$)/i.test(trimmedContent) && attachmentsForSend.length === 0 && fileReferencesForSend.length === 0 && skillReferencesForSend.length === 0) {
      await compactCurrentConversation(trimmedContent.replace(/^\/compact\b/i, '').trim());
      return;
    }
    if (!selectedModel?.id) {
      setChatError('请先在设置页配置并启用模型');
      return;
    }
    await waitForConversationTransition(conversationTransitionRef);
    const targetConversationId = currentConversationIdRef?.current || currentConversationId;
    const activeRunConversationId = activeAgentRunRef?.current?.conversationId || activeChatAbortRef.current?.conversationId || '';
    const isBusyInSameConversation = isChatSending && (!activeRunConversationId || activeRunConversationId === targetConversationId);
    if (isBusyInSameConversation && !usingOverride) {
      if (hasUnsupportedRuntimeGuidance) {
        warnUnsupportedRuntimeGuidance();
        return;
      }
      createPendingRunMessage(trimmedContent);
      return;
    }
    if (isChatSending && !usingOverride) {
      if (hasUnsupportedRuntimeGuidance) {
        warnUnsupportedRuntimeGuidance();
        return;
      }
      createPendingRunMessage(trimmedContent, { conversationId: targetConversationId });
      return;
    }
    const payload = {
      content: trimmedContent,
      conversationId: currentConversationIdRef?.current || currentConversationId,
      attachments: attachmentsForSend.map((item) => ({
        id: item.id,
        name: item.name,
        size: item.size,
        type: item.type,
        file: item.file,
      })),
      fileReferences: fileReferencesForSend,
      skillReferences: skillReferencesForSend,
      currentFileContext: buildCurrentFileContext?.() || null,
      thinkingMode,
    };

    if (!usingOverride) {
      setChatInput('');
      setChatAttachments([]);
      setChatFileReferences([]);
      setChatSkillReferences([]);
    }

    if (isChatSending) {
      setQueuedUserMessage(payload, trimmedContent);
      return;
    }

    await sendChatPayload(payload);
  }

  async function sendFollowupAnswer(answer) {
    const trimmedAnswer = String(answer || '').trim();
    if (!trimmedAnswer) return;
    if (!selectedModel?.id) {
      setChatError('请先在设置页配置并启用模型');
      return;
    }
    await waitForConversationTransition(conversationTransitionRef);
    const payload = {
      content: trimmedAnswer,
      attachments: [],
      fileReferences: [],
      skillReferences: [],
      currentFileContext: null,
      conversationId: currentConversationIdRef?.current || currentConversationId,
      thinkingMode,
    };
    if (isChatSending) {
      setQueuedUserMessage(payload, trimmedAnswer);
      return;
    }
    await sendChatPayload(payload);
  }

  async function sendChatPayload(payload) {
    await waitForConversationTransition(conversationTransitionRef);
    const trimmedContent = payload.content.trim();
    const conversationIDForPayload = payload.conversationId || currentConversationIdRef?.current || currentConversationId;
    const shouldUpdateVisibleConversation = () =>
      !conversationIDForPayload || (currentConversationIdRef?.current || currentConversationId) === conversationIDForPayload;
    const setVisibleConversationMessages = (updater) => {
      setConversationMessages((current) => (shouldUpdateVisibleConversation() ? updater(current) : current));
    };
    const stamp = Date.now();
    const optimisticId = `temp_user_${stamp}`;
    const thinkingId = `temp_assistant_${stamp}`;
    const attachments = payload.attachments.map((item) => ({
      id: item.id,
      name: item.name,
      size: item.size,
      type: item.type,
    }));
    const metadataAttachments = [
      ...attachments,
      ...(payload.fileReferences || []),
      ...(payload.skillReferences || []),
      ...(payload.currentFileContext ? [payload.currentFileContext] : []),
    ];
    const filesToSend = payload.attachments.map((item) => item.file).filter(Boolean);

    if (shouldUpdateVisibleConversation()) {
      setChatError('');
      setResumableRun(null);
    }
    setVisibleConversationMessages((current) => [
      ...current.filter((item) => !(item.queued && item.content === trimmedContent)),
      {
        id: optimisticId,
        conversation_id: conversationIDForPayload || 'pending',
        role: 'user',
        content: trimmedContent,
        attachments: metadataAttachments,
        created_at: new Date().toISOString(),
      },
      buildAssistantRunShell({
        id: thinkingId,
        conversationId: conversationIDForPayload || 'pending',
        createdAt: new Date().toISOString(),
        content: '',
        attachments: [],
      }),
    ]);
    setIsChatSending(true);

    let conversationID = conversationIDForPayload;
    const abortController = new AbortController();
    const control = {
      conversationId: conversationID || conversationIDForPayload || '',
      abort: () => abortController.abort(),
    };
    activeChatAbortRef.current = control;
    try {
      if (conversationID) {
        if (shouldUpdateVisibleConversation()) {
          currentConversationIdRef.current = conversationID;
          setCurrentConversationId(conversationID);
        }
      } else {
        conversationID = await ensureConversation(selectedModel.id);
        control.conversationId = conversationID;
      }
      const formData = buildMessageFormData(payload, selectedModel.id, thinkingMode, filesToSend);
      const result = await apiPostFormWithSignal(
        `/api/conversations/${encodeURIComponent(conversationID)}/messages`,
        formData,
        abortController.signal
      );
      if (shouldUpdateVisibleConversation()) {
        setCurrentConversationId(result.conversation.id);
        currentConversationIdRef.current = result.conversation.id;
      }
      control.conversationId = result.conversation.id;
      const committedUserMessageId = result.user_message?.id || '';
      if (result.user_message) {
        setVisibleConversationMessages((current) =>
          mergeCommittedRunStart(current, {
            optimisticUserId: optimisticId,
            thinkingId,
            userMessage: result.user_message,
            assistantShell: buildAssistantRunShell({
              id: thinkingId,
              conversationId: result.conversation.id,
              createdAt: new Date().toISOString(),
              content: '',
              attachments: [],
              runId: result.run?.id || '',
            }),
          })
        );
      }
      if (result.run?.id) {
        await waitForAgentRun(result.run.id, result.conversation.id, thinkingId, control, 0, committedUserMessageId);
      } else {
        await loadConversations();
        if (shouldUpdateVisibleConversation()) {
          await loadConversationMessages(result.conversation.id);
        }
        if (result.task) onTasksChanged?.(result.task);
        await onAssistantToolEvents?.(result.assistant_message?.attachments || []);
      }
    } catch (error) {
      handleSendError(error, thinkingId, conversationID);
    } finally {
      if (activeChatAbortRef.current === control) activeChatAbortRef.current = null;
      setIsChatSending(false);
    }
  }

  function setQueuedUserMessage(payload, content) {
    const conversationIDForPayload = payload.conversationId || currentConversationIdRef?.current || currentConversationId;
    setQueuedChatMessages((current) => [...current, payload]);
    setConversationMessages((current) => {
      const visibleConversationId = currentConversationIdRef?.current || currentConversationId;
      if (conversationIDForPayload && visibleConversationId && conversationIDForPayload !== visibleConversationId) {
        return current;
      }
      return [
        ...current,
        {
          id: `queued_user_${Date.now()}`,
          conversation_id: conversationIDForPayload || 'pending',
          role: 'user',
          content,
          attachments: [
            ...payload.attachments.map(({ file, ...item }) => item),
            ...payload.fileReferences,
            ...payload.skillReferences,
          ],
          created_at: new Date().toISOString(),
          queued: true,
        },
      ];
    });
  }

  function handleSendError(error, thinkingId, conversationID) {
    console.error('sendChatMessage failed', error);
    if (conversationID && currentConversationIdRef?.current && currentConversationIdRef.current !== conversationID) {
      return;
    }
    if (error?.name === 'AbortError') {
      setChatError('');
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          canceled: true,
          content: '已停止生成。',
        }))
      );
      return;
    }
    const message = formatChatError(error);
    setChatError(message);
    if (error?.runId) {
      setResumableRun({
        runId: error.runId,
        conversationId: error.conversationId || conversationID,
        diagnosticLogId: error.diagnosticLogId || '',
        ...chatFailureMetadata(error),
      });
    }
    const isBudgetExhausted = error?.stage === 'budget_exhausted' || error?.incompleteReason === 'budget_exhausted';
    const prefix = error?.runId
      ? (isBudgetExhausted ? '达到轮次上限，已总结当前进度，可继续本轮' : '连接中断，可继续这轮任务')
      : '回复失败';
    setConversationMessages((current) =>
      mapPendingAssistant(current, thinkingId, (item) => ({
        ...item,
        pending: false,
        error: true,
        resumableRunId: error?.runId || '',
        diagnosticLogId: error?.diagnosticLogId || '',
        ...chatFailureMetadata(error),
        content: isBudgetExhausted ? message : `${prefix}：${message}`,
      }))
    );
  }

  return {
    sendChatMessage,
    sendChatPayload,
    sendFollowupAnswer,
  };
}

async function waitForConversationTransition(conversationTransitionRef) {
  const transition = conversationTransitionRef?.current;
  if (!transition) return;
  try {
    await transition;
  } catch {
    // The branch/select action owns the visible error. Sending then continues
    // against the last confirmed conversation, matching Codex-style committed state.
  }
}

function buildMessageFormData(payload, modelProviderId, defaultThinkingMode, filesToSend) {
  const formData = new FormData();
  formData.append('content', payload.content.trim());
  formData.append('model_provider_id', modelProviderId);
  formData.append('thinking_mode', payload.thinkingMode || defaultThinkingMode || 'standard');
  for (const attachment of payload.attachments) {
    formData.append('attachment_ids[]', attachment.id);
  }
  formData.append('workspace_refs_json', JSON.stringify(payload.fileReferences || []));
  formData.append('selected_skills_json', JSON.stringify(payload.skillReferences || []));
  if (payload.currentFileContext) {
    formData.append('current_file_context_json', JSON.stringify(payload.currentFileContext));
  }
  for (const file of filesToSend) {
    formData.append('files[]', file, file.name);
  }
  return formData;
}
