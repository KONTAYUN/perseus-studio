import { apiPost } from '../api/client';
import { formatChatError } from './chatErrorUtils';
import {
  appendRunProgressItem,
  buildAssistantRunShell,
  buildContextCompactionNotice,
  buildProgressItem,
  mapPendingAssistant,
} from './chatStateUtils.js';

export function createChatCompactionActions({
  currentConversationId,
  isChatSending,
  loadConversations,
  setChatError,
  setChatInput,
  setContextCompactionNotice,
  setConversationMessages,
}) {
  async function compactCurrentConversation(instructions = '') {
    if (!currentConversationId) {
      setChatError('请先打开一个会话再压缩上下文');
      return;
    }
    if (isChatSending) {
      setChatError('当前回复还在生成，完成后再压缩上下文');
      return;
    }
    const thinkingId = `temp_compact_${Date.now()}`;
    setChatError('');
    setChatInput('');
    setConversationMessages((current) => [
      ...current,
      buildAssistantRunShell({
        id: thinkingId,
        conversationId: currentConversationId,
        createdAt: new Date().toISOString(),
        content: '',
        attachments: [],
      }),
    ]);
    try {
      appendCompactProgress(thinkingId, 'context_compaction_started', {
        reason: 'manual',
        message: '正在压缩上下文',
      });
      const result = await apiPost(`/api/conversations/${encodeURIComponent(currentConversationId)}/compact`, {
        instructions,
      });
      appendCompactProgress(thinkingId, 'context_compaction_completed', {
        ...result,
        reason: 'manual',
        message: '上下文已压缩',
      });
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          content: result?.updated
            ? `上下文已压缩：合并了 ${result.compacted_message_count || 0} 条较早消息。`
            : '当前上下文还不需要压缩。',
        }))
      );
      await loadConversations();
    } catch (error) {
      console.error('compactCurrentConversation failed', error);
      const message = formatChatError(error);
      setChatError(message);
      setConversationMessages((current) =>
        mapPendingAssistant(current, thinkingId, (item) => ({
          ...item,
          pending: false,
          content: `上下文压缩失败：${message}`,
        }))
      );
    }
  }

  function appendCompactProgress(thinkingId, type, payload = {}) {
    setContextCompactionNotice(buildContextCompactionNotice(type, payload));
    const nextItem = buildProgressItem(type, payload, payload.message || '');
    if (!nextItem) return;
    setConversationMessages((current) => appendRunProgressItem(current, thinkingId, nextItem));
  }

  return {
    compactCurrentConversation,
  };
}
