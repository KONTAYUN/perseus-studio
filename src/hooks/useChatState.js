import { useEffect, useRef, useState } from 'react';
import { createChatCompactionActions } from './chatCompactionActions';
import { createChatConversationActions } from './chatConversationActions';
import { createChatRegenerateResumeActions } from './chatRegenerateResumeActions';
import { createChatSendActions } from './chatSendActions';
import { createChatSteerActions, textOnlyPayload, useActiveAgentRunRef, useMissedRunSteerAutoQueue, usePendingRunMessageAutoSend } from './chatSteerActions';
import { waitForAgentRunStream } from './chatRunStream.js';
import useChatLifecycleEffects from './useChatLifecycleEffects';
import useChatReferenceActions from './useChatReferenceActions';
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
  const [pendingRunMessage, setPendingRunMessage] = useState(null);
  const [activeAgentRun, setActiveAgentRun] = useState(null);

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
    if (isChatSending || queuedChatMessages.length === 0) return;
    const [nextMessage, ...rest] = queuedChatMessages;
    setQueuedChatMessages(rest);
    sendChatPayload(nextMessage);
  }, [isChatSending, queuedChatMessages]);

  useEffect(() => {
    setThinkingMode((current) => normalizeThinkingModeForModel(current, selectedModel));
  }, [selectedModel?.id, selectedModel?.thinking_profile]);

  useActiveAgentRunRef(activeAgentRun, activeAgentRunRef);
  usePendingRunMessageAutoSend({
    currentConversationId,
    currentConversationIdRef,
    canceledPendingRunMessageIdsRef,
    canceledRunSteerIdsRef,
    isChatSending,
    pendingRunMessage,
    resumableRun,
    sendChatPayloadRef,
    setPendingRunMessage,
    setRunSteerNotice,
    thinkingMode,
  });
  useMissedRunSteerAutoQueue({
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
  });

  const { compactCurrentConversation } = createChatCompactionActions({
    currentConversationId,
    isChatSending,
    loadConversations,
    setChatError,
    setChatInput,
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
    activeAgentRunRef,
    chatAttachments,
    chatFileReferences,
    chatInput,
    chatSkillReferences,
    currentConversationId,
    currentConversationIdRef,
    canceledPendingRunMessageIdsRef,
    isChatSending,
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
    activeAgentRunRef,
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
  });
  sendChatPayloadRef.current = sendChatPayload;

  function stopChatGeneration() {
    activeChatAbortRef.current?.abort();
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
    isChatSending,
    isCurrentConversationSending:
      isChatSending && (!activeAgentRun?.conversationId || activeAgentRun.conversationId === currentConversationId),
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
