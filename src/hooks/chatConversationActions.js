import { apiDelete, apiGet, apiPatch, apiPost } from '../api/client';
import {
  normalizeConversationMessages,
  normalizeStoredContextUsage,
} from './chatStateUtils.js';

export function createChatConversationActions({
  contextUsageRequestRef,
  conversations,
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
      if (currentConversationId && !items.some((item) => item.id === currentConversationId)) {
        contextUsageRequestRef.current += 1;
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
    if (currentConversationId) return currentConversationId;
    const conversation = await apiPost('/api/conversations', {
      selected_model_provider_id: modelProviderID,
    });
    suppressNextConversationLoadRef.current = true;
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
      if (currentConversationId === conversationID) {
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
    if (!currentConversationId || !messageID) return;
    setChatError('');
    try {
      const result = await apiPost(`/api/conversations/${encodeURIComponent(currentConversationId)}/branch`, {
        message_id: messageID,
      });
      suppressNextConversationLoadRef.current = true;
      setCurrentConversationId(result.conversation.id);
      setConversationMessages(normalizeConversationMessages(result.messages || []));
      setContextUsage(null);
      setContextCompactionNotice(null);
      setIsConversationMenuOpen(false);
      await loadConversations();
    } catch (error) {
      console.error('branchConversation failed', error);
      setChatError(error.message);
    }
  }

  function returnChatHome() {
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
