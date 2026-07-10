import React from 'react';
import { RefreshCw } from 'lucide-react';
import { Button } from '../ui';
import EmptyChatState from './EmptyChatState';
import MemoizedMessageBubble, { lastAssistantMessageId } from './ChatMessageBubble';
import { formatRetryAfterLabel, isResumeBlockedByFailure } from '../../hooks/chatErrorUtils';

export default function ChatMessages({ ide, messagesRef, bottomRef, messagesBottomInset, messagesScrollbarBottomInset }) {
  const isHomeEmpty = !ide.currentConversationId && ide.conversationMessages.length === 0 && !ide.chatError && !ide.agentRecoveryNotice?.text;
  const bottomInset = isHomeEmpty ? 0 : messagesBottomInset;
  const scrollbarBottomInset = isHomeEmpty ? 10 : Math.max(10, messagesScrollbarBottomInset || 10);
  const lastAssistantId = React.useMemo(
    () => lastAssistantMessageId(ide.conversationMessages),
    [ide.conversationMessages]
  );
  const resumeBlocked = isResumeBlockedByFailure(ide.resumableRun);
  const resumeBlockedLabel = resumeBlocked
    ? formatRetryAfterLabel(ide.resumableRun?.retryAfterMs, ide.resumableRun?.resetAt)
    : '';
  return (
    <div
      ref={messagesRef}
      className="chat-messages-scroll relative z-10 flex-1 space-y-1 overflow-y-auto overflow-x-hidden bg-transparent p-4 custom-scrollbar scrollbar-rounded-clip"
      style={{
        paddingBottom: `${bottomInset}px`,
        '--scrollbar-track-margin-bottom': `${scrollbarBottomInset}px`,
      }}
    >
      {ide.chatError && (
        <div className="chat-error-card min-w-0 max-w-full overflow-hidden whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
          {ide.chatError}
          {ide.resumableRun?.runId && (
            <div className="mt-3">
              {resumeBlockedLabel && (
                <div className="mb-2 text-[11px] font-semibold text-red-300">
                  {resumeBlockedLabel}
                </div>
              )}
              <Button
                type="button"
                variant="danger"
                size="xs"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.continueAgentRun?.();
                }}
                disabled={ide.isChatSending || resumeBlocked}
              >
                <RefreshCw size={13} />
                继续
              </Button>
            </div>
          )}
        </div>
      )}
      {ide.agentRecoveryNotice?.text && !ide.chatError && (
        <div className="agent-recovery-card">
          <span className="inline-flex items-center gap-2">
            {(ide.agentRecoveryNotice.status === 'started' || ide.agentRecoveryNotice.status === 'retrying') && (
              <RefreshCw size={14} className="animate-spin" />
            )}
            {ide.agentRecoveryNotice.text}
          </span>
        </div>
      )}

      {!ide.currentConversationId && ide.conversationMessages.length === 0 ? (
        <EmptyChatState ide={ide} />
      ) : ide.conversationMessages.length === 0 ? (
        <div className="flex h-full min-h-[240px] items-center justify-center">
          <div className="empty-chat-card max-w-sm rounded-3xl border border-gray-100 bg-white/70 px-5 py-4 text-center text-sm leading-7 text-gray-500 dark:border-white/10 dark:bg-[#2C2C2E]/70 dark:text-gray-400">
            {ide.selectedModel
              ? `使用 ${ide.selectedModel.name} 开始这轮新对话。`
              : '当前还没有可用模型，请先在设置页添加并启用模型。'}
          </div>
        </div>
      ) : (
        ide.conversationMessages.map((message) => (
          <MemoizedMessageBubble
            key={message.id}
            message={message}
            ide={ide}
            canRegenerate={message.id === lastAssistantId}
          />
        ))
      )}
      {!isHomeEmpty && (
        <div
          ref={bottomRef}
          className="h-px"
          style={{ scrollMarginBottom: `${bottomInset}px` }}
        />
      )}
    </div>
  );
}
