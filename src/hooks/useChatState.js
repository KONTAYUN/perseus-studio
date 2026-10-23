import { useEffect, useRef, useState } from 'react';
import { createChatCompactionActions } from './chatCompactionActions';
import { createChatConversationActions } from './chatConversationActions';
import { createChatRegenerateResumeActions } from './chatRegenerateResumeActions';
import { createChatSendActions } from './chatSendActions';
import { createChatSteerActions, useActiveAgentRunRef, useMissedRunSteerAutoQueue } from './chatSteerActions';
import usePendingRunMessages, { useDurablePendingMessageAutoSend } from './usePendingRunMessages.js';
import { apiPost } from '../api/client.js';
import { waitForAgentRunStream } from './chatRunStream.js';
import useChatLifecycleEffects from './useChatLifecycleEffects';
import useChatReferenceActions from './useChatReferenceActions';
import useConversationTitleRefresh from './useConversationTitleRefresh.js';
import { normalizeThinkingModeForModel } from './modelReasoningUtils';
import {
  visibleRunSteerTimelineItems,
} from './chatSteerItems';

export default function useChatState({
  buildCurrentFileContext,
  confirm,
  onAssistantToolEvents,
  onTasksChanged,
  selectedModel,
  setSelectedModelId,
  userId = '',
} = {}) {
  const {
    addChatAttachments,
    addClipboardAttachments,
    addSkillReference,
    addWorkspaceFileReferences,
    chatAttachments,
    chatFileReferences,
    chatSkillReferences,
    removeChatAttachment,
    removeSkillReference,
    removeWorkspaceFileReference,
    setChatAttachments,
    setChatFileReferences,
    setChatSkillReferences,
  } = useChatReferenceActions();
  const [isConversationMenuOpen, setIsConversationMenuOpen] = useState(false);
  const [conversations, setConversations] = useState([]);
  const [currentConversationId, setCurrentConversationId] = useState('');
  const [conversationMessages, setConversationMessages] = useState([]);
  const [chatInput, setChatInput] = useState('');
  const [queuedChatMessages, setQueuedChatMessages] = useState([]);
  const [isChatSending, setIsChatSending] = useState(false);
  const [chatError, setChatError] = useState('');
  const [resumableRun, setResumableRun] = useState(null);
  const [thinkingMode, setThinkingMode] = useState('standard');
  const [contextUsage, setContextUsage] = useState(null);
  const [contextCompactionNotice, setContextCompactionNotice] = useState(null);
  const [agentRecoveryNotice, setAgentRecoveryNotice] = useState(null);
  const [runSteerNotice, setRunSteerNotice] = useState(null);
  const [runSteerItems, setRunSteerItems] = useState([]);
  const [activeAgentRun, setActiveAgentRun] = useState(null);
  const [observedRun, setObservedRun] = useState(null);
  const visibleObservedRun = observedRun?.ownerId === userId && observedRun?.conversation_id === currentConversationId
    && ['queued', 'running'].includes(observedRun.status) ? observedRun : null;
  const isBusy = isChatSending || Boolean(visibleObservedRun);

  const suppressNextConversationLoadRef = useRef(false);
  const activeChatAbortRef = useRef(null);
  const activeAgentRunRef = useRef(null);
  const currentConversationIdRef = useRef('');
  const conversationTransitionRef = useRef(null);
  const sendChatPayloadRef = useRef(null);
  const conversationMessagesRef = useRef([]);
  const contextUsageRequestRef = useRef(0);
  const canceledPendingRunMessageIdsRef = useRef(new Set());
  const canceledRunSteerIdsRef = useRef(new Set());
  const guidanceRunRef = useRef(null);
  guidanceRunRef.current = visibleObservedRun
    ? { runId: visibleObservedRun.id, conversationId: visibleObservedRun.conversation_id }
    : activeAgentRunRef.current;
  const { pendingRunMessage, pendingRunMessages, setPendingRunMessage, refreshPendingMessages } = usePendingRunMessages({
    userId, currentConversationId, currentConversationIdRef, setRunSteerNotice,
  });

  const {
    branchConversation,
    createNewConversation,
    deleteConversation,
    ensureConversation,
    loadConversationMessages,
    loadConversations,
    renameConversation,
    returnChatHome,
    selectConversation,
  } = createChatConversationActions({
    onLatestRun: run => setObservedRun(run ? { ...run, ownerId: userId } : null),
    contextUsageRequestRef,
    conversations,
    conversationTransitionRef,
    currentConversationIdRef,
    currentConversationId,
    selectedModel,
    setAgentRecoveryNotice,
    setChatError,
    setContextCompactionNotice,
    setContextUsage,
    setConversationMessages,
    setConversations,
    setCurrentConversationId,
    setIsConversationMenuOpen,
    setResumableRun,
    setSelectedModelId,
    setRunSteerItems,
    suppressNextConversationLoadRef,
  });
  const scheduleTitleRefresh = useConversationTitleRefresh(loadConversations);
  const reloadMessagesRef = useRef(loadConversationMessages);
  reloadMessagesRef.current = loadConversationMessages;
  useEffect(() => {
    if (!visibleObservedRun || isChatSending) return undefined;
    let disposed = false;
    let timer;
    const poll = async () => {
      await reloadMessagesRef.current(currentConversationId);
      if (!disposed) timer = window.setTimeout(poll, 2000);
    };
    timer = window.setTimeout(poll, 2000);
    return () => { disposed = true; window.clearTimeout(timer); };
  }, [visibleObservedRun?.id, currentConversationId, isChatSending]);

  useChatLifecycleEffects({
    agentRecoveryNotice,
    contextCompactionNotice,
    contextUsageRequestRef,
    conversationMessages,
    conversationMessagesRef,
    currentConversationIdRef,
    currentConversationId,
    loadConversationMessages,
    setAgentRecoveryNotice,
    setContextCompactionNotice,
    setContextUsage,
    setConversationMessages,
    suppressNextConversationLoadRef,
  });

  useEffect(() => {
    if (isBusy || queuedChatMessages.length === 0) return;
    const [nextMessage, ...rest] = queuedChatMessages;
    setQueuedChatMessages(rest);
    sendChatPayload(nextMessage);
  }, [isBusy, queuedChatMessages]);

  useEffect(() => {
    setThinkingMode((current) => normalizeThinkingModeForModel(current, selectedModel));
  }, [selectedModel?.id, selectedModel?.thinking_profile]);

  useActiveAgentRunRef(activeAgentRun, activeAgentRunRef);
  useDurablePendingMessageAutoSend({ userId, pendingRunMessages, isChatSending: isBusy, sendChatPayloadRef, refreshPendingMessages });
  useMissedRunSteerAutoQueue({
    modelProviderId: selectedModel?.id || '',
    currentConversationId,
    currentConversationIdRef,
    canceledPendingRunMessageIdsRef,
    isChatSending: isBusy,
    resumableRun,
    runSteerItems,
    setPendingRunMessage,
    setRunSteerItems,
    setRunSteerNotice,
    thinkingMode,
  });

  const { compactCurrentConversation } = createChatCompactionActions({
    currentConversationId,
    isChatSending: isBusy,
    loadConversations,
    setChatError,
    setContextCompactionNotice,
    setConversationMessages,
  });

  const {
    continueAgentRun,
    regenerateLastAssistantMessage,
  } = createChatRegenerateResumeActions({
    activeChatAbortRef,
    confirm,
    conversationMessages,
    currentConversationId,
    currentConversationIdRef,
    isChatSending: isBusy,
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
  });

  const {
    createPendingRunMessage,
    discardPendingRunMessage,
    discardRunSteerItem,
    hasUnsupportedRuntimeGuidance,
    queueRunSteerItemAfterRun,
    retryRunSteerItem,
    sendPendingRunMessageAsSteer,
    steerChatInput,
    warnUnsupportedRuntimeGuidance,
  } = createChatSteerActions({
    refreshPendingMessages,
    modelProviderId: selectedModel?.id || '',
    activeAgentRunRef: guidanceRunRef,
    chatAttachments,
    chatFileReferences,
    chatInput,
    chatSkillReferences,
    currentConversationId,
    currentConversationIdRef,
    canceledPendingRunMessageIdsRef,
    isChatSending: isBusy,
    pendingRunMessage,
    runSteerItems,
    sendChatPayloadRef,
    setChatInput,
    setPendingRunMessage,
    setRunSteerItems,
    setRunSteerNotice,
    thinkingMode,
  });

  const { sendChatMessage, sendChatPayload, sendFollowupAnswer } = createChatSendActions({
    activeAgentRunRef: guidanceRunRef,
    activeChatAbortRef,
    buildCurrentFileContext,
    chatAttachments,
    chatFileReferences,
    chatInput,
    chatSkillReferences,
    compactCurrentConversation,
    createPendingRunMessage,
    conversationTransitionRef,
    currentConversationIdRef,
    currentConversationId,
    ensureConversation,
    hasUnsupportedRuntimeGuidance,
    isChatSending: isBusy,
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
  });
  sendChatPayloadRef.current = sendChatPayload;

  async function stopChatGeneration() {
    if (activeChatAbortRef.current && activeChatAbortRef.current.conversationId === currentConversationIdRef.current) {
      activeChatAbortRef.current.abort();
    } else if (visibleObservedRun) {
      try {
        await apiPost(`/api/agent-runs/${encodeURIComponent(visibleObservedRun.id)}/cancel`, { source: 'stop_button', reason: 'user_requested_stop' });
        await loadConversationMessages(visibleObservedRun.conversation_id);
      } catch (error) { setChatError(error.message); }
    }
  }

  async function waitForAgentRun(runId, conversationID, thinkingId, control, afterSeq = 0, userMessageId = '') {
    const activeRun = { runId, conversationId: conversationID };
    activeAgentRunRef.current = activeRun;
    setActiveAgentRun(activeRun);
    try {
      await waitForAgentRunStream({
        afterSeq,
        control,
        conversationID,
        currentConversationIdRef,
        userMessageId,
        loadConversations,
        scheduleTitleRefresh,
        onAssistantToolEvents,
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
      });
    } finally {
      if (activeAgentRunRef.current?.runId === runId) {
        activeAgentRunRef.current = null;
        setActiveAgentRun(null);
      }
    }
  }

  return {
    addChatAttachments,
    addClipboardAttachments,
    addSkillReference,
    addWorkspaceFileReferences,
    agentRecoveryNotice,
    chatAttachments,
    chatError,
    chatFileReferences,
    chatInput,
    chatSkillReferences,
    continueAgentRun,
    branchConversation,
    conversationMessages,
    conversations,
    contextCompactionNotice,
    contextUsage,
    createNewConversation,
    currentConversationId,
    deleteConversation,
    isChatSending: isBusy,
    isCurrentConversationSending:
      Boolean(visibleObservedRun) || (isChatSending && (!activeAgentRun?.conversationId || activeAgentRun.conversationId === currentConversationId)),
    isConversationMenuOpen,
    loadConversationMessages,
    loadConversations,
    pendingRunMessage,
    removeChatAttachment,
    removeSkillReference,
    removeWorkspaceFileReference,
    resumableRun,
    regenerateLastAssistantMessage,
    renameConversation,
    returnChatHome,
    selectConversation,
    sendChatMessage,
    steerChatInput,
    sendPendingRunMessageAsSteer,
    discardPendingRunMessage,
    retryRunSteerItem,
    queueRunSteerItemAfterRun,
    discardRunSteerItem,
    sendFollowupAnswer,
    compactCurrentConversation,
    stopChatGeneration,
    setChatInput,
    setCurrentConversationId,
    setIsConversationMenuOpen,
    setThinkingMode,
    thinkingMode,
    runSteerNotice,
    runSteerItems: visibleRunSteerTimelineItems(runSteerItems, currentConversationId),
  };
}
