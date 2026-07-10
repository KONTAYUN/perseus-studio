import { apiPost } from '../api/client';
import { chatFailureMetadata, formatRetryAfterLabel, isResumeBlockedByFailure, formatChatError } from './chatErrorUtils';
import { buildAssistantRunShell, mapPendingAssistant } from './chatStateUtils.js';
import { isWorkspaceRefreshEvent } from './ideUtils.js';

export function createChatRegenerateResumeActions({
  activeChatAbortRef,
  confirm,
  conversationMessages,
  currentConversationId,
  isChatSending,
  loadConversationMessages,
  loadConversations,
  onAssistantToolEvents,
  onTasksChanged,
  resumableRun,
  setChatError,
  setConversationMessages,
  setIsChatSending,
  setResumableRun,
  thinkingMode,
  waitForAgentRun,
}) {
  async function regenerateLastAssistantMessage(messageID) {
    if (!currentConversationId || !messageID || isChatSending) return;
    const selectedIndex = conversationMessages.findIndex((item) => item.id === messageID);
    const selectedMessage = selectedIndex >= 0 ? conversationMessages[selectedIndex] : null;
    let lastUserIndex = -1;
    conversationMessages.forEach((item, index) => {
      if (item.role === 'user') lastUserIndex = index;
    });
    if (!selectedMessage || selectedMessage.role !== 'assistant' || selectedIndex <= lastUserIndex) return;
    const assistantTail = conversationMessages.slice(lastUserIndex + 1).filter((item) => item.role === 'assistant');
    const hasMutationTool = assistantTail.some((message) => (message.attachments || []).some((item) =>
      isWorkspaceRefreshEvent(item)
    ));
    if (hasMutationTool) {
      const ok = await confirm?.({
        title: '重新生成回复',
        message: '重新生成不会回滚已经执行过的文件修改，确认继续？',
        confirmLabel: '继续重新生成',
        tone: 'red',
      });
      if (!ok) return;
    }

    setChatError('');
    setIsChatSending(true);
    const thinkingId = `temp_regenerate_${Date.now()}`;
    const control = { abort: () => {} };
    activeChatAbortRef.current = control;
    setConversationMessages((current) =>
      [
        ...current.slice(0, Math.max(0, lastUserIndex + 1)),
        buildAssistantRunShell({
          id: thinkingId,
          conversationId: currentConversationId,
          createdAt: new Date().toISOString(),
          content: '',
          attachments: [],
          runId: '',
        }),
      ]
    );
    try {
      const result = await apiPost(`/api/conversations/${encodeURIComponent(currentConversationId)}/regenerate-last`, {
        message_id: messageID,
        thinking_mode: thinkingMode,
      });
      if (result.run?.id) {
        await waitForAgentRun(result.run.id, result.conversation.id, thinkingId, control);
      } else {
        await loadConversations();
        await loadConversationMessages(result.conversation.id);
      }
      if (result.task) {
        onTasksChanged?.(result.task);
      }
      await onAssistantToolEvents?.(result.assistant_message?.attachments || []);
    } catch (error) {
      console.error('regenerateLastAssistantMessage failed', error);
      const message = formatChatError(error);
      setChatError(message);
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          error: true,
          content: `重新生成失败：${message}`,
        }))
      );
    } finally {
      if (activeChatAbortRef.current === control) {
        activeChatAbortRef.current = null;
      }
      setIsChatSending(false);
    }
  }

  async function continueAgentRun(runId = resumableRun?.runId, conversationID = resumableRun?.conversationId || currentConversationId) {
    if (!runId || !conversationID || isChatSending) return;
    if (resumableRun?.runId === runId && isResumeBlockedByFailure(resumableRun)) {
      const waitLabel = formatRetryAfterLabel(resumableRun.retryAfterMs, resumableRun.resetAt);
      setChatError(resumableRun.recommendedAction || `Provider rate limit is active; ${waitLabel}. Switch model or retry later.`);
      return;
    }
    const thinkingId = `temp_resume_${Date.now()}`;
    setChatError('');
    setResumableRun(null);
    setIsChatSending(true);
    setConversationMessages((current) => [
      ...current,
      buildAssistantRunShell({
        id: thinkingId,
        conversationId: conversationID,
        createdAt: new Date().toISOString(),
        content: '',
        attachments: [],
        runId,
      }),
    ]);
    const control = { abort: () => {} };
    activeChatAbortRef.current = control;
    try {
      const result = await apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/resume`, {});
      const afterSeq = result?.resume_event?.seq || 0;
      await waitForAgentRun(runId, conversationID, thinkingId, control, afterSeq);
    } catch (error) {
      console.error('continueAgentRun failed', error);
      const message = formatChatError(error);
      setChatError(message);
      if (error?.runId) {
        const failureMetadata = chatFailureMetadata(error);
        setResumableRun({
          runId: error.runId,
          conversationId: error.conversationId || conversationID,
          diagnosticLogId: error.diagnosticLogId || '',
          message,
          ...failureMetadata,
        });
        if (isResumeBlockedByFailure(failureMetadata)) {
          setConversationMessages((current) =>
            (current || [])
              .filter((item) => item.id !== thinkingId)
              .map((item) =>
                item.resumableRunId === error.runId
                  ? {
                      ...item,
                      content: message,
                      diagnosticLogId: error.diagnosticLogId || item.diagnosticLogId || '',
                      ...failureMetadata,
                    }
                  : item
              )
          );
          return;
        }
      }
      const isBudgetExhausted = error?.stage === 'budget_exhausted' || error?.incompleteReason === 'budget_exhausted';
      const prefix = error?.runId
        ? (isBudgetExhausted ? '达到轮次上限，已总结当前进度，可继续本轮' : '连接中断，可继续这轮任务')
        : '继续失败';
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
    } finally {
      if (activeChatAbortRef.current === control) {
        activeChatAbortRef.current = null;
      }
      setIsChatSending(false);
    }
  }

  return {
    continueAgentRun,
    regenerateLastAssistantMessage,
  };
}
