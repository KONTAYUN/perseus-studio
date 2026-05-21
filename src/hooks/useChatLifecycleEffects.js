import { useEffect } from 'react';

export default function useChatLifecycleEffects({
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
}) {
  useEffect(() => {
    conversationMessagesRef.current = conversationMessages;
  }, [conversationMessages, conversationMessagesRef]);

  useEffect(() => {
    currentConversationIdRef.current = currentConversationId || '';
  }, [currentConversationId, currentConversationIdRef]);

  useEffect(() => {
    if (!contextCompactionNotice || contextCompactionNotice.status === 'started') return undefined;
    const timer = window.setTimeout(() => {
      setContextCompactionNotice((current) =>
        current?.id === contextCompactionNotice.id ? null : current
      );
    }, 6500);
    return () => window.clearTimeout(timer);
  }, [contextCompactionNotice, setContextCompactionNotice]);

  useEffect(() => {
    if (!agentRecoveryNotice || agentRecoveryNotice.status === 'started' || agentRecoveryNotice.status === 'retrying') return undefined;
    const timer = window.setTimeout(() => {
      setAgentRecoveryNotice((current) =>
        current?.id === agentRecoveryNotice.id ? null : current
      );
    }, 6500);
    return () => window.clearTimeout(timer);
  }, [agentRecoveryNotice, setAgentRecoveryNotice]);

  useEffect(() => {
    if (!currentConversationId) {
      contextUsageRequestRef.current += 1;
      setConversationMessages([]);
      setContextUsage(null);
      setContextCompactionNotice(null);
      setAgentRecoveryNotice(null);
      return;
    }
    if (suppressNextConversationLoadRef.current) {
      suppressNextConversationLoadRef.current = false;
      return;
    }
    loadConversationMessages(currentConversationId);
  }, [currentConversationId]);
}
