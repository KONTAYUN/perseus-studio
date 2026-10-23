import { createRunSubmission, submitRunRequest } from './chatRunSubmission.js';
import { apiPost } from '../api/client.js';
import { formatChatError } from './chatErrorUtils.js';
import { buildAssistantRunShell, mapPendingAssistant } from './chatStateUtils.js';
import { isWorkspaceRefreshEvent } from './ideUtils.js';

export function createChatRegenerateResumeActions({
  activeChatAbortRef,
  confirm,
  conversationMessages,
  currentConversationId,
  currentConversationIdRef,
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
  const isVisible = (id) => !currentConversationIdRef || currentConversationIdRef.current === id;
  const updateVisible = (id, update) => setConversationMessages((current) => isVisible(id) ? update(current) : current);

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
    const control = createRunSubmission(currentConversationId);
    activeChatAbortRef.current = control;
    updateVisible(currentConversationId, (current) =>
      [
        ...current,
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
      const result = await submitRunRequest(control, (options) => apiPost(`/api/conversations/${encodeURIComponent(currentConversationId)}/regenerate-last`, {
        message_id: messageID,
        thinking_mode: thinkingMode,
      }, options));
      if (result.run?.id) {
        updateVisible(currentConversationId, (current) => [
          ...current.slice(0, Math.max(0, lastUserIndex + 1)),
          ...current.filter((item) => item.id === thinkingId),
        ]);
        await waitForAgentRun(result.run.id, result.conversation.id, thinkingId, control);
      } else {
        await loadConversations();
        if (isVisible(result.conversation.id)) await loadConversationMessages(result.conversation.id);
      }
      if (result.task) {
        onTasksChanged?.(result.task);
      }
      await onAssistantToolEvents?.(result.assistant_message?.attachments || []);
    } catch (error) {
      if (error?.name === 'AbortError') {
        updateVisible(currentConversationId, (current) => current.filter((item) => item.id !== thinkingId));
        return;
      }
      console.error('regenerateLastAssistantMessage failed', error);
      const message = formatChatError(error);
      if (isVisible(currentConversationId)) setChatError(message);
      updateVisible(currentConversationId, (current) =>
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
        setIsChatSending(false);
      }
    }
  }

  async function continueAgentRun(runId = resumableRun?.runId, conversationID = resumableRun?.conversationId || currentConversationId) {
    if (!runId || !conversationID || isChatSending) return;
    const thinkingId = `temp_resume_${Date.now()}`;
    setChatError('');
    setResumableRun(null);
    setIsChatSending(true);
    updateVisible(conversationID, (current) => [
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
    const control = createRunSubmission(conversationID);
    activeChatAbortRef.current = control;
    try {
      const result = await submitRunRequest(control, (options) => apiPost(`/api/agent-runs/${encodeURIComponent(runId)}/resume`, {}, options), runId);
      const afterSeq = result?.resume_event?.seq || 0;
      await waitForAgentRun(runId, conversationID, thinkingId, control, afterSeq);
    } catch (error) {
      if (error?.name === 'AbortError') {
        updateVisible(conversationID, (current) => current.filter((item) => item.id !== thinkingId));
        return;
      }
      console.error('continueAgentRun failed', error);
      const message = formatChatError(error);
      if (isVisible(conversationID)) setChatError(message);
      if (isVisible(conversationID)) {
        setResumableRun({
          runId: error?.runId || runId,
          conversationId: error.conversationId || conversationID,
          diagnosticLogId: error.diagnosticLogId || '',
        });
      }
      const isBudgetExhausted = error?.stage === 'budget_exhausted' || error?.incompleteReason === 'budget_exhausted';
      const prefix = error?.runId
        ? (isBudgetExhausted ? '达到轮次上限，已总结当前进度，可继续本轮' : '连接中断，可继续这轮任务')
        : '继续失败';
      updateVisible(conversationID, (current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          error: true,
          resumableRunId: error?.runId || runId,
          diagnosticLogId: error?.diagnosticLogId || '',
          content: isBudgetExhausted ? message : `${prefix}：${message}`,
        }))
      );
    } finally {
      if (activeChatAbortRef.current === control) {
        activeChatAbortRef.current = null;
        setIsChatSending(false);
      }
    }
  }

  return {
    continueAgentRun,
    regenerateLastAssistantMessage,
  };
}
