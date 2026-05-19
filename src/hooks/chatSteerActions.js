import { useEffect } from 'react';
import { apiPost } from '../api/client';
import { formatChatError } from './chatErrorUtils';

export function textOnlyPayload(content, { conversationId = '', thinkingMode = 'standard' } = {}) {
  return {
    content,
    attachments: [],
    fileReferences: [],
    skillReferences: [],
    currentFileContext: null,
    conversationId,
    thinkingMode,
  };
}

export function createChatSteerActions({
  activeAgentRunRef,
  chatAttachments,
  chatFileReferences,
  chatInput,
  chatSkillReferences,
  currentConversationId,
  isChatSending,
  pendingRunMessage,
  sendChatPayloadRef,
  setChatInput,
  setPendingRunMessage,
  setRunSteerNotice,
  thinkingMode,
}) {
  function createPendingRunMessage(content) {
    const trimmedContent = String(content || '').trim();
    if (!trimmedContent) return false;
    setPendingRunMessage({
      id: `pending_run_${Date.now()}`,
      content: trimmedContent,
      conversationId: activeAgentRunRef.current?.conversationId || currentConversationId,
      createdAt: new Date().toISOString(),
      thinkingMode,
    });
    setChatInput('');
    setRunSteerNotice({
      id: `steer_notice_${Date.now()}`,
      status: 'queued',
      text: pendingRunMessage ? '已替换挂起消息，当前任务结束后自动发送。' : '已挂起，当前任务结束后自动发送。',
    });
    return true;
  }

  function warnUnsupportedRuntimeGuidance() {
    setRunSteerNotice({
      id: `steer_notice_${Date.now()}`,
      status: 'warn',
      text: '运行中引导暂不支持附件、文件引用或 Skill，请等待当前任务结束后发送。',
    });
  }

  async function steerChatInput() {
    const trimmedContent = chatInput.trim();
    if (!trimmedContent) return;
    if (chatAttachments.length > 0 || chatFileReferences.length > 0 || chatSkillReferences.length > 0) {
      warnUnsupportedRuntimeGuidance();
      return;
    }
    const ok = await sendRunSteer(trimmedContent);
    if (ok) setChatInput('');
  }

  async function sendPendingRunMessageAsSteer() {
    if (!pendingRunMessage?.content) return;
    const ok = await sendRunSteer(pendingRunMessage.content, {
      pendingId: pendingRunMessage.id,
      conversationId: pendingRunMessage.conversationId,
      thinkingMode: pendingRunMessage.thinkingMode,
    });
    if (ok) {
      setPendingRunMessage((current) => (current?.id === pendingRunMessage.id ? null : current));
    }
  }

  function discardPendingRunMessage() {
    setPendingRunMessage(null);
    setRunSteerNotice(null);
  }

  async function sendRunSteer(content, options = {}) {
    const trimmedContent = String(content || '').trim();
    if (!trimmedContent) return false;
    const activeRun = activeAgentRunRef.current;
    if (!isChatSending || !activeRun?.runId) {
      setPendingRunMessage((current) => (options.pendingId && current?.id === options.pendingId ? null : current));
      await sendChatPayloadRef.current?.(textOnlyPayload(trimmedContent, {
        conversationId: options.conversationId || currentConversationId,
        thinkingMode: options.thinkingMode || thinkingMode,
      }));
      return true;
    }
    const clientSteerId = `client_steer_${Date.now()}`;
    try {
      await apiPost(`/api/agent-runs/${encodeURIComponent(activeRun.runId)}/steer`, {
        content: trimmedContent,
        expected_run_id: activeRun.runId,
        client_steer_id: clientSteerId,
      });
      setRunSteerNotice({
        id: clientSteerId,
        status: 'received',
        text: '已收到引导，会在下一次安全边界应用。',
      });
      return true;
    } catch (error) {
      console.error('sendRunSteer failed', error);
      setRunSteerNotice({
        id: clientSteerId,
        status: 'error',
        text: `引导暂未发送：${formatChatError(error)}`,
      });
      if (!options.pendingId) {
        createPendingRunMessage(trimmedContent);
      }
      return false;
    }
  }

  const hasUnsupportedRuntimeGuidance = chatAttachments.length > 0 || chatFileReferences.length > 0 || chatSkillReferences.length > 0;

  return {
    createPendingRunMessage,
    discardPendingRunMessage,
    hasUnsupportedRuntimeGuidance,
    sendPendingRunMessageAsSteer,
    steerChatInput,
    warnUnsupportedRuntimeGuidance,
  };
}

export function usePendingRunMessageAutoSend({
  currentConversationId,
  isChatSending,
  pendingRunMessage,
  resumableRun,
  sendChatPayloadRef,
  setPendingRunMessage,
  setRunSteerNotice,
  thinkingMode,
}) {
  useEffect(() => {
    if (isChatSending || !pendingRunMessage || resumableRun) return;
    const nextMessage = pendingRunMessage;
    setPendingRunMessage(null);
    setRunSteerNotice(null);
    sendChatPayloadRef.current?.(textOnlyPayload(nextMessage.content, {
      conversationId: nextMessage.conversationId || currentConversationId,
      thinkingMode: nextMessage.thinkingMode || thinkingMode,
    }));
  }, [currentConversationId, isChatSending, pendingRunMessage, resumableRun, sendChatPayloadRef, setPendingRunMessage, setRunSteerNotice, thinkingMode]);
}

export function useActiveAgentRunRef(activeAgentRun, activeAgentRunRef) {
  useEffect(() => {
    activeAgentRunRef.current = activeAgentRun;
  }, [activeAgentRun, activeAgentRunRef]);
}
