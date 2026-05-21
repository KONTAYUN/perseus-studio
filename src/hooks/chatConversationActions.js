import { apiDelete, apiGet, apiPatch, apiPost } from '../api/client';
import {
  normalizeConversationMessages,
  normalizeStoredContextUsage,
} from './chatStateUtils.js';

export function createChatConversationActions({
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
  suppressNextConversationLoadRef,
}) {
  async function loadConversations() {
    setChatError('');
    setResumableRun(null);
    try {
      const data = await apiGet('/api/conversations');
      const items = data.items || [];
      setConversations(items);
      const activeConversationID = currentConversationIdRef?.current || currentConversationId;
      if (activeConversationID && !items.some((item) => item.id === activeConversationID)) {
        contextUsageRequestRef.current += 1;
        currentConversationIdRef.current = '';
        setCurrentConversationId('');
        setConversationMessages([]);
        setContextUsage(null);
        setContextCompactionNotice(null);
        setAgentRecoveryNotice(null);
      }
    } catch (error) {
      console.error('loadConversations failed', error);
      setChatError(error.message);
    }
  }

  async function loadConversationMessages(conversationID) {
    const requestId = contextUsageRequestRef.current + 1;
    contextUsageRequestRef.current = requestId;
    if (!conversationID) {
      setConversationMessages([]);
      setContextUsage(null);
      return;
    }
    try {
      const [data, usageData] = await Promise.all([
        apiGet(`/api/conversations/${encodeURIComponent(conversationID)}/messages`),
        apiGet(`/api/conversations/${encodeURIComponent(conversationID)}/context-usage`).catch((error) => {
          console.error('loadConversationContextUsage failed', error);
          return { usage: null };
        }),
      ]);
      if (contextUsageRequestRef.current !== requestId) return;
      setConversationMessages(normalizeConversationMessages(data.items || []));
      setContextUsage(normalizeStoredContextUsage(usageData, selectedModel));
    } catch (error) {
      if (contextUsageRequestRef.current !== requestId) return;
      console.error('loadConversationMessages failed', error);
      setChatError(error.message);
    }
  }

  async function ensureConversation(modelProviderID) {
    const existingConversationID = currentConversationIdRef?.current || currentConversationId;
    if (existingConversationID) return existingConversationID;
    const conversation = await apiPost('/api/conversations', {
      selected_model_provider_id: modelProviderID,
    });
    suppressNextConversationLoadRef.current = true;
    currentConversationIdRef.current = conversation.id;
    setCurrentConversationId(conversation.id);
    await loadConversations();
    return conversation.id;
  }

  async function createNewConversation() {
    if (!selectedModel?.id) {
      setChatError('请先在设置页配置并启用模型');
      return;
    }
    setChatError('');
    setResumableRun(null);
    try {
      const conversation = await apiPost('/api/conversations', {
        selected_model_provider_id: selectedModel.id,
      });
      currentConversationIdRef.current = conversation.id;
      setCurrentConversationId(conversation.id);
      setConversationMessages([]);
      setContextUsage(null);
      setContextCompactionNotice(null);
      setIsConversationMenuOpen(false);
      await loadConversations();
    } catch (error) {
      console.error('createNewConversation failed', error);
      setChatError(error.message);
    }
  }

  async function selectConversation(conversationID) {
    currentConversationIdRef.current = conversationID;
    setCurrentConversationId(conversationID);
    setContextUsage(null);
    setContextCompactionNotice(null);
    setIsConversationMenuOpen(false);
    await loadConversationMessages(conversationID);
    const conversation = conversations.find((item) => item.id === conversationID);
    if (conversation?.selected_model_provider_id) {
      setSelectedModelId?.(conversation.selected_model_provider_id);
    }
  }

  async function deleteConversation(conversationID) {
    setChatError('');
    setResumableRun(null);
    try {
      await apiDelete(`/api/conversations/${encodeURIComponent(conversationID)}`);
      if ((currentConversationIdRef?.current || currentConversationId) === conversationID) {
        currentConversationIdRef.current = '';
        setCurrentConversationId('');
        setConversationMessages([]);
        setContextUsage(null);
        setContextCompactionNotice(null);
      }
      await loadConversations();
    } catch (error) {
      console.error('deleteConversation failed', error);
      setChatError(error.message);
    }
  }

  async function renameConversation(conversationID, title) {
    const nextTitle = title.trim();
    if (!nextTitle) return;
    setChatError('');
    setResumableRun(null);
    try {
      const updated = await apiPatch(`/api/conversations/${encodeURIComponent(conversationID)}`, {
        title: nextTitle,
      });
      setConversations((current) =>
        current.map((item) => (item.id === conversationID ? { ...item, ...updated } : item))
      );
      await loadConversations();
    } catch (error) {
      console.error('renameConversation failed', error);
      setChatError(error.message);
    }
  }

  async function branchConversation(messageID) {
    const sourceConversationID = currentConversationIdRef?.current || currentConversationId;
    if (!sourceConversationID || !messageID) return;
    setChatError('');
    const transition = (async () => {
      const result = await apiPost(`/api/conversations/${encodeURIComponent(sourceConversationID)}/branch`, {
        message_id: messageID,
      });
      suppressNextConversationLoadRef.current = true;
      currentConversationIdRef.current = result.conversation.id;
      setCurrentConversationId(result.conversation.id);
      setConversationMessages(normalizeConversationMessages(result.messages || []));
      setContextUsage(null);
      setContextCompactionNotice(null);
      setIsConversationMenuOpen(false);
      await loadConversations();
      return result.conversation.id;
    })();
    if (conversationTransitionRef) {
      const trackedTransition = transition.finally(() => {
        if (conversationTransitionRef.current === trackedTransition) {
          conversationTransitionRef.current = null;
        }
      });
      conversationTransitionRef.current = trackedTransition;
    }
    try {
      return await transition;
    } catch (error) {
      console.error('branchConversation failed', error);
      setChatError(error.message);
      return '';
    }
  }

  function returnChatHome() {
    currentConversationIdRef.current = '';
    setCurrentConversationId('');
    setConversationMessages([]);
    setContextUsage(null);
    setContextCompactionNotice(null);
    setChatError('');
    setResumableRun(null);
    setIsConversationMenuOpen(false);
  }

  return {
    branchConversation,
    createNewConversation,
    deleteConversation,
    ensureConversation,
    loadConversationMessages,
    loadConversations,
    renameConversation,
    returnChatHome,
    selectConversation,
  };
}
