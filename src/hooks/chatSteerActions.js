import { useEffect } from 'react';
import { apiPost } from '../api/client';
import { formatChatError } from './chatErrorUtils';
import {
  appendRunSteerItem,
  buildRunSteerItem,
  markRunSteerFailed,
  markRunSteerReceived,
  removeRunSteerItem,
  removeRunSteerItems,
  unappliedRunSteerItems,
} from './chatSteerItems';

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
  currentConversationIdRef,
  isChatSending,
  pendingRunMessage,
  runSteerItems,
  sendChatPayloadRef,
  setChatInput,
  setPendingRunMessage,
  setRunSteerItems,
  setRunSteerNotice,
  thinkingMode,
}) {
  function createPendingRunMessage(content) {
    const trimmedContent = String(content || '').trim();
    if (!trimmedContent) return false;
    setPendingRunMessage({
      id: `pending_run_${Date.now()}`,
      content: trimmedContent,
      conversationId: activeAgentRunRef.current?.conversationId || currentConversationIdRef?.current || currentConversationId,
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

  async function retryRunSteerItem(itemId) {
    const item = findRunSteerItem(runSteerItems, itemId);
    if (!item?.content) return false;
    setRunSteerItems((current) => removeRunSteerItem(current, itemId));
    return sendRunSteer(item.content, {
      conversationId: item.conversation_id,
      thinkingMode,
    });
  }

  function queueRunSteerItemAfterRun(itemId) {
    const item = findRunSteerItem(runSteerItems, itemId);
    if (!item?.content) return false;
    setRunSteerItems((current) => removeRunSteerItem(current, itemId));
    createPendingRunMessage(item.content);
    return true;
  }

  function discardRunSteerItem(itemId) {
    setRunSteerItems((current) => removeRunSteerItem(current, itemId));
  }

  async function sendRunSteer(content, options = {}) {
    const trimmedContent = String(content || '').trim();
    if (!trimmedContent) return false;
    const activeRun = activeAgentRunRef.current;
    if (!isChatSending || !activeRun?.runId) {
      setPendingRunMessage((current) => (options.pendingId && current?.id === options.pendingId ? null : current));
      await sendChatPayloadRef.current?.(textOnlyPayload(trimmedContent, {
        conversationId: options.conversationId || currentConversationIdRef?.current || currentConversationId,
        thinkingMode: options.thinkingMode || thinkingMode,
      }));
      return true;
    }
    const clientSteerId = `client_steer_${Date.now()}`;
    const steerItem = buildRunSteerItem({
      clientSteerId,
      content: trimmedContent,
      conversationId: activeRun.conversationId || options.conversationId || currentConversationIdRef?.current || currentConversationId,
      runId: activeRun.runId,
      pendingId: options.pendingId || '',
    });
    setRunSteerItems((current) => appendRunSteerItem(current, steerItem));
    setRunSteerNotice(null);
    try {
      const result = await apiPost(`/api/agent-runs/${encodeURIComponent(activeRun.runId)}/steer`, {
        content: trimmedContent,
        expected_run_id: activeRun.runId,
        client_steer_id: clientSteerId,
      });
      setRunSteerItems((current) => markRunSteerReceived(current, result?.event?.payload || {}, {
        clientSteerId,
        content: trimmedContent,
        conversationId: activeRun.conversationId || options.conversationId || currentConversationIdRef?.current || currentConversationId,
        runId: activeRun.runId,
      }));
      return true;
    } catch (error) {
      console.error('sendRunSteer failed', error);
      setRunSteerItems((current) => markRunSteerFailed(current, clientSteerId, formatChatError(error)));
      setRunSteerNotice({
        id: clientSteerId,
        status: 'error',
        text: `引导暂未发送：${formatChatError(error)}`,
      });
      if (!options.pendingId) {
        setRunSteerNotice(null);
      } else {
        setRunSteerItems((current) => removeRunSteerItem(current, clientSteerId));
      }
      return false;
    }
  }

  const hasUnsupportedRuntimeGuidance = chatAttachments.length > 0 || chatFileReferences.length > 0 || chatSkillReferences.length > 0;

  return {
    createPendingRunMessage,
    discardPendingRunMessage,
    discardRunSteerItem,
    hasUnsupportedRuntimeGuidance,
    queueRunSteerItemAfterRun,
    retryRunSteerItem,
    sendPendingRunMessageAsSteer,
    steerChatInput,
    warnUnsupportedRuntimeGuidance,
  };
}

function findRunSteerItem(items, itemId) {
  return (items || []).find((item) => item.id === itemId || item.clientSteerId === itemId || item.steerId === itemId);
}

export function usePendingRunMessageAutoSend({
  currentConversationId,
  currentConversationIdRef,
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
      conversationId: nextMessage.conversationId || currentConversationIdRef?.current || currentConversationId,
      thinkingMode: nextMessage.thinkingMode || thinkingMode,
    }));
  }, [currentConversationId, currentConversationIdRef, isChatSending, pendingRunMessage, resumableRun, sendChatPayloadRef, setPendingRunMessage, setRunSteerNotice, thinkingMode]);
}

export function useMissedRunSteerAutoQueue({
  currentConversationId,
  currentConversationIdRef,
  isChatSending,
  resumableRun,
  runSteerItems,
  setPendingRunMessage,
  setRunSteerItems,
  setRunSteerNotice,
  thinkingMode,
}) {
  useEffect(() => {
    if (isChatSending || resumableRun) return;
    const missed = unappliedRunSteerItems(runSteerItems);
    if (missed.length === 0) return;
    const content = missed.map((item) => item.content).filter(Boolean).join('\n\n').trim();
    if (!content) return;
    setRunSteerItems((current) =>
      removeRunSteerItems(
        current,
        missed.flatMap((item) => [item.id, item.clientSteerId, item.steerId])
      )
    );
    setPendingRunMessage({
      id: `pending_run_${Date.now()}`,
      content,
      conversationId: missed[0]?.conversation_id || currentConversationIdRef?.current || currentConversationId,
      createdAt: new Date().toISOString(),
      thinkingMode,
    });
    setRunSteerNotice({
      id: `steer_missed_${Date.now()}`,
      status: 'queued',
      text: '引导没有赶上上一轮，已转为下一条消息。',
    });
  }, [currentConversationId, currentConversationIdRef, isChatSending, resumableRun, runSteerItems, setPendingRunMessage, setRunSteerItems, setRunSteerNotice, thinkingMode]);
}

export function useActiveAgentRunRef(activeAgentRun, activeAgentRunRef) {
  useEffect(() => {
    activeAgentRunRef.current = activeAgentRun;
  }, [activeAgentRun, activeAgentRunRef]);
}
