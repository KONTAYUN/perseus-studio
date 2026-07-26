import { apiPost, apiPostFormWithSignal } from '../api/client';
import { chatFailureHeading, chatFailureMetadata, formatChatError } from './chatErrorUtils';
import { buildAssistantRunShell, mapPendingAssistant, mergeCommittedRunStart } from './chatStateUtils.js';
import { pendingUserRequest } from '../components/chat/chatUtils.js';

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
  conversationMessages,
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
	clearChatAttachments,
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
	const blockedAttachment = attachmentsForSend.find(
	  (item) => item.kind === 'image' && item.upload_status !== 'uploaded'
	);
	if (blockedAttachment) {
	  setChatError(blockedAttachment.upload_status === 'failed' ? '图片上传失败，请重试或移除后再发送。' : '图片仍在上传，请稍候。');
	  return;
	}
    if (!trimmedContent && attachmentsForSend.length === 0 && fileReferencesForSend.length === 0 && skillReferencesForSend.length === 0) return;
    if (/^\/compact(?:\s|$)/i.test(trimmedContent) && attachmentsForSend.length === 0 && fileReferencesForSend.length === 0 && skillReferencesForSend.length === 0) {
      await compactCurrentConversation(trimmedContent.replace(/^\/compact\b/i, '').trim());
      return;
    }
    const userRequest = pendingUserRequest(conversationMessages);
    if (userRequest && !usingOverride) {
      if (attachmentsForSend.length > 0 || fileReferencesForSend.length > 0 || skillReferencesForSend.length > 0) {
        setChatError('回答当前问题时请先发送文字；附件可以在任务恢复后继续补充。');
        return;
      }
      await sendUserResponse(userRequest, trimmedContent);
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
	  attachments: attachmentsForSend.map((item) => ({ ...item })),
      fileReferences: fileReferencesForSend,
      skillReferences: skillReferencesForSend,
      currentFileContext: buildCurrentFileContext?.() || null,
      thinkingMode,
    };

    if (!usingOverride) {
      setChatInput('');
	  clearChatAttachments();
      setChatFileReferences([]);
      setChatSkillReferences([]);
    }

    if (isChatSending) {
      setQueuedUserMessage(payload, trimmedContent);
      return;
    }

    await sendChatPayload(payload);
  }

  async function sendUserResponse(request, answer) {
    const trimmedAnswer = String(answer || '').trim();
    if (!trimmedAnswer || !request?.runId || !request?.toolCallId || isChatSending) return;
    await waitForConversationTransition(conversationTransitionRef);
    const conversationID = currentConversationIdRef?.current || currentConversationId;
    const thinkingId = `temp_assistant_followup_${Date.now()}`;
    const answerId = globalThis.crypto?.randomUUID?.() || `answer_${Date.now()}_${Math.random().toString(16).slice(2)}`;
    const control = { conversationId: conversationID, abort: () => {} };
    activeChatAbortRef.current = control;
    setChatInput('');
    setChatError('');
    setIsChatSending(true);
    try {
      const result = await apiPost(`/api/agent-runs/${encodeURIComponent(request.runId)}/followup-answer`, {
        tool_call_id: request.toolCallId,
        answer: trimmedAnswer,
        answer_id: answerId,
      });
      setConversationMessages((current) => {
        const updated = (current || []).map((item) => {
          if (result.assistant_message?.id && item.id === result.assistant_message.id) {
            return { ...item, ...result.assistant_message };
          }
          if (item.id !== request.messageId) return item;
          return {
            ...item,
            attachments: (item.attachments || []).map((attachment) =>
              attachment?.tool_call_id === request.toolCallId
                ? { ...attachment, status: 'answered', answer_message_id: result.user_message?.id || '' }
                : attachment
            ),
          };
        });
        if (result.user_message && !updated.some((item) => item.id === result.user_message.id)) {
          updated.push(result.user_message);
        }
        if (!result.already_applied) {
          updated.push(buildAssistantRunShell({
            id: thinkingId,
            conversationId: conversationID,
            createdAt: new Date().toISOString(),
            runId: request.runId,
          }));
        }
        return updated;
      });
      if (!result.already_applied) {
        await waitForAgentRun(
          request.runId,
          conversationID,
          thinkingId,
          control,
          result.resume_event?.seq || 0,
          result.user_message?.id || ''
        );
      }
    } catch (error) {
      handleSendError(error, thinkingId, conversationID);
    } finally {
      if (activeChatAbortRef.current === control) activeChatAbortRef.current = null;
      setIsChatSending(false);
    }
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
	const attachments = payload.attachments.map(({ file, preview_url, client_id, fingerprint, upload_error, ...item }) => item);
    const metadataAttachments = [
      ...attachments,
      ...(payload.fileReferences || []),
      ...(payload.skillReferences || []),
      ...(payload.currentFileContext ? [payload.currentFileContext] : []),
    ];
	const filesToSend = payload.attachments
	  .filter((item) => item.kind !== 'image' || item.upload_status !== 'uploaded')
	  .map((item) => item.file)
	  .filter(Boolean);

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
    const failureMetadata = chatFailureMetadata(error);
    const prefix = chatFailureHeading(failureMetadata.failureClass, {
      resumable: Boolean(error?.runId),
      budgetExhausted: isBudgetExhausted,
    });
    setConversationMessages((current) =>
      mapPendingAssistant(current, thinkingId, (item) => ({
        ...item,
        pending: false,
        error: true,
        resumableRunId: error?.runId || '',
        diagnosticLogId: error?.diagnosticLogId || '',
        ...failureMetadata,
        content: isBudgetExhausted ? message : `${prefix}：${message}`,
      }))
    );
  }

  return {
    sendChatMessage,
    sendChatPayload,
    sendUserResponse,
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
	  if (attachment.kind === 'image' && attachment.upload_status === 'uploaded') {
		formData.append('attachment_ids[]', attachment.id);
	  }
  }
  formData.append('workspace_refs_json', JSON.stringify(payload.fileReferences || []));
  formData.append('selected_skills_json', JSON.stringify(payload.skillReferences || []));
  if (payload.currentFileContext) {
    formData.append('editor_context_json', JSON.stringify(payload.currentFileContext));
  }
  for (const file of filesToSend) {
    formData.append('files[]', file, file.name);
  }
  return formData;
}
