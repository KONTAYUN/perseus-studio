import { useEffect, useRef } from 'react';
import { apiDeleteWithBody, apiPost } from '../api/client';
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
  refreshPendingMessages,
  modelProviderId,
  activeAgentRunRef,
  chatAttachments,
  chatFileReferences,
  chatInput,
  chatSkillReferences,
  currentConversationId,
  currentConversationIdRef,
  canceledPendingRunMessageIdsRef,
  canceledRunSteerIdsRef,
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
  async function createPendingRunMessage(content, options = {}) {
    const trimmedContent = String(content || '').trim();
    if (!trimmedContent) return false;
    const activeRunConversationId = activeAgentRunRef.current?.conversationId || '';
    const targetConversationId = currentConversationIdRef?.current || currentConversationId;
    const conversationId = options.conversationId || activeRunConversationId || targetConversationId;
    const id = globalThis.crypto.randomUUID();
    canceledPendingRunMessageIdsRef?.current?.delete?.(id);
    const saved = await setPendingRunMessage({
      id,
      content: trimmedContent,
      conversationId,
      createdAt: new Date().toISOString(),
      thinkingMode,
      modelProviderId,
    });
    if (!saved) return false;
    if ((currentConversationIdRef?.current || currentConversationId) === targetConversationId) {
      setChatInput(current => current.trim() === trimmedContent ? '' : current);
    }
    setRunSteerNotice({
      id: `steer_notice_${Date.now()}`,
      conversationId,
      status: 'queued',
      text: pendingRunMessage ? '已替换挂起消息，当前任务结束后自动发送。' : '已挂起，当前任务结束后自动发送。',
    });
    return true;
  }

  function warnUnsupportedRuntimeGuidance() {
    setRunSteerNotice({
      id: `steer_notice_${Date.now()}`,
      conversationId: currentConversationIdRef?.current || currentConversationId,
      status: 'warn',
      text: '运行中引导暂不支持附件、文件引用或 Skill，请等待当前任务结束后发送。',
    });
  }

  async function steerChatInput() {
    const trimmedContent = chatInput.trim();
    if (!trimmedContent) return;
    const activeRun = activeAgentRunRef.current;
    const targetConversationId = currentConversationIdRef?.current || currentConversationId;
    if (!activeRun?.runId || activeRun.conversationId !== targetConversationId) {
      await createPendingRunMessage(trimmedContent, { conversationId: targetConversationId });
      return;
    }
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
      await refreshPendingMessages?.();
    }
  }

  function discardPendingRunMessage() {
    const targetConversationId = currentConversationIdRef?.current || currentConversationId;
    setPendingRunMessage((current) => {
      if (!targetConversationId || current?.conversationId === targetConversationId) {
        if (current?.id) {
          canceledPendingRunMessageIdsRef?.current?.add?.(current.id);
        }
        return null;
      }
      return current;
    });
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

  async function queueRunSteerItemAfterRun(itemId) {
    const item = findRunSteerItem(runSteerItems, itemId);
    if (!item?.content) return false;
    const saved = await createPendingRunMessage(item.content, { conversationId: item.conversation_id });
    if (saved) setRunSteerItems((current) => removeRunSteerItem(current, itemId));
    return saved;
  }

  function discardRunSteerItem(itemId) {
    const item = findRunSteerItem(runSteerItems, itemId);
    for (const id of [itemId, item?.id, item?.clientSteerId, item?.steerId].filter(Boolean)) {
      canceledRunSteerIdsRef?.current?.add?.(id);
    }
    setRunSteerItems((current) => removeRunSteerItem(current, itemId));
    cancelRunSteerOnServer(item);
  }

  async function sendRunSteer(content, options = {}) {
    const trimmedContent = String(content || '').trim();
    if (!trimmedContent) return false;
    const activeRun = activeAgentRunRef.current;
    const targetConversationId = options.conversationId || currentConversationIdRef?.current || currentConversationId;
    if (!isChatSending || !activeRun?.runId || activeRun.conversationId !== targetConversationId) {
      if (isChatSending) {
        return createPendingRunMessage(trimmedContent, { conversationId: targetConversationId });
      }
      return sendChatPayloadRef.current?.({ ...textOnlyPayload(trimmedContent, {
        conversationId: targetConversationId,
        thinkingMode: options.thinkingMode || thinkingMode,
      }), ...(options.pendingId ? { pendingMessageId: options.pendingId } : {}) });
    }
    const clientSteerId = options.pendingId ? `pending:${options.pendingId}` : globalThis.crypto.randomUUID();
    canceledRunSteerIdsRef?.current?.delete?.(clientSteerId);
    const steerItem = buildRunSteerItem({
      clientSteerId,
      content: trimmedContent,
      conversationId: activeRun.conversationId || targetConversationId,
      runId: activeRun.runId,
      pendingId: options.pendingId || '',
    });
    setRunSteerItems((current) => appendRunSteerItem(current, steerItem));
    setRunSteerNotice(null);
    try {
      const result = await apiPost(`/api/agent-runs/${encodeURIComponent(activeRun.runId)}/steer`, {
        pending_message_id: options.pendingId || '',
        content: trimmedContent,
        expected_run_id: activeRun.runId,
        client_steer_id: clientSteerId,
      });
      const payload = result?.event?.payload || {};
      const serverSteerId = payload.steer_id || '';
      if (canceledRunSteerIdsRef?.current?.has?.(clientSteerId) || (serverSteerId && canceledRunSteerIdsRef?.current?.has?.(serverSteerId))) {
        cancelRunSteerOnServer({
          runId: activeRun.runId,
          clientSteerId,
          steerId: serverSteerId,
        });
        return false;
      }
      setRunSteerItems((current) => markRunSteerReceived(current, result?.event?.payload || {}, {
        clientSteerId,
        content: trimmedContent,
        conversationId: activeRun.conversationId || targetConversationId,
        runId: activeRun.runId,
      }));
      return true;
    } catch (error) {
      console.error('sendRunSteer failed', error);
      setRunSteerItems((current) => markRunSteerFailed(current, clientSteerId, formatChatError(error)));
      setRunSteerNotice({
        id: clientSteerId,
        conversationId: targetConversationId,
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

function cancelRunSteerOnServer(item) {
  if (!item?.runId || (!item.clientSteerId && !item.steerId)) return;
  apiDeleteWithBody(`/api/agent-runs/${encodeURIComponent(item.runId)}/steers`, {
    expected_run_id: item.runId,
    client_steer_id: item.clientSteerId || '',
    steer_id: item.steerId || '',
    reason: 'user_deleted_guidance',
  }).catch((error) => {
    console.error('cancelRunSteer failed', error);
  });
}

export function useMissedRunSteerAutoQueue({
  modelProviderId,
  currentConversationId,
  currentConversationIdRef,
  canceledPendingRunMessageIdsRef,
  isChatSending,
  resumableRun,
  runSteerItems,
  setPendingRunMessage,
  setRunSteerItems,
  setRunSteerNotice,
  thinkingMode,
}) {
  const persisting = useRef(false);
  useEffect(() => {
    if (isChatSending || resumableRun || persisting.current) return;
    const missed = unappliedRunSteerItems(runSteerItems);
    if (missed.length === 0) return;
    const content = missed.map((item) => item.content).filter(Boolean).join('\n\n').trim();
    if (!content) return;
    const pendingId = globalThis.crypto.randomUUID();
    canceledPendingRunMessageIdsRef?.current?.delete?.(pendingId);
    persisting.current = true;
    Promise.resolve(setPendingRunMessage({
      id: pendingId,
      content,
      conversationId: missed[0]?.conversation_id || currentConversationIdRef?.current || currentConversationId,
      createdAt: new Date().toISOString(),
      thinkingMode,
      modelProviderId,
    })).then(saved => {
      if (!saved) return;
      setRunSteerItems(current => removeRunSteerItems(current, missed.flatMap(item => [item.id, item.clientSteerId, item.steerId])));
      setRunSteerNotice({
      id: `steer_missed_${Date.now()}`,
      conversationId: missed[0]?.conversation_id || currentConversationIdRef?.current || currentConversationId,
      status: 'queued',
      text: '引导没有赶上上一轮，已转为下一条消息。',
      });
    }).finally(() => { persisting.current = false; });
  }, [modelProviderId, canceledPendingRunMessageIdsRef, currentConversationId, currentConversationIdRef, isChatSending, resumableRun, runSteerItems, setPendingRunMessage, setRunSteerItems, setRunSteerNotice, thinkingMode]);
}

export function useActiveAgentRunRef(activeAgentRun, activeAgentRunRef) {
  useEffect(() => {
    activeAgentRunRef.current = activeAgentRun;
  }, [activeAgentRun, activeAgentRunRef]);
}
