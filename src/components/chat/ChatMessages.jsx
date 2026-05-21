import React from 'react';
import { Copy, GitBranch, MessageSquare, RefreshCw, Trash2 } from 'lucide-react';
import { completedActionItems, formatDate, latestLiveProgressItem, loadingBubbleText, visibleMessageAttachments, shouldReserveMessageFooter } from './chatUtils';
import { Button, IconButton } from '../ui';
import { RUN_STEER_STATUS, runSteerStatusLabel } from '../../hooks/chatSteerItems';

const STEER_FAILURE_BUTTON_CLASS = 'border-white/20 bg-white/10 text-blue-50 hover:bg-white/15 hover:text-white dark:border-white/15 dark:bg-white/10';

export default function ChatMessages({ ide, messagesRef, bottomRef, messagesBottomInset }) {
  return (
    <div
      ref={messagesRef}
      className="chat-messages-scroll relative z-10 flex-1 space-y-1 overflow-y-auto overflow-x-hidden bg-gradient-to-b from-transparent to-gray-50/30 p-4 dark:to-black/10 custom-scrollbar scrollbar-rounded-clip"
      style={{ paddingBottom: `${messagesBottomInset}px` }}
    >
      {ide.chatError && (
        <div className="chat-error-card min-w-0 max-w-full overflow-hidden whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
          {ide.chatError}
          {ide.resumableRun?.runId && (
            <div className="mt-3">
              <Button
                type="button"
                variant="danger"
                size="xs"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.continueAgentRun?.();
                }}
                disabled={ide.isChatSending}
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
          <MessageBubble
            key={message.id}
            message={message}
            ide={ide}
            canRegenerate={message.id === lastAssistantMessageId(ide.conversationMessages)}
          />
        ))
      )}
      <div
        ref={bottomRef}
        className="h-px"
        style={{ scrollMarginBottom: `${messagesBottomInset}px` }}
      />
    </div>
  );
}

function MessageBubble({ message, ide, canRegenerate }) {
  const attachments = visibleMessageAttachments(message);
  const actionItems = completedActionItems(message);
  const showLoadingOnly = message.pending && !(message.content || '').trim();
  const shouldShowFooterGap = shouldReserveMessageFooter(message);
  const isSteerMessage = message.kind === 'steer';
  return (
    <div className={`group/message flex min-w-0 max-w-full overflow-hidden ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}>
      <div className={`flex min-w-0 max-w-[88%] overflow-hidden flex-col ${message.role === 'user' ? 'items-end' : 'items-start'}`}>
        {isSteerMessage && <SteerStatusLabel message={message} />}
        <div
          className={`message-bubble motion-message min-w-0 max-w-full overflow-hidden px-4 py-3 text-[14px] leading-relaxed ${
            message.role === 'user'
              ? 'message-bubble-user bg-blue-500 text-white'
              : message.error
              ? 'message-bubble-assistant border border-red-500/20 bg-red-500/10 text-red-300'
              : message.pending
              ? 'message-bubble-assistant border border-gray-100 bg-white text-gray-500 dark:border-white/5 dark:bg-[#2C2C2E] dark:text-gray-400'
              : 'message-bubble-assistant border border-gray-100 bg-white text-gray-800 dark:border-white/5 dark:bg-[#2C2C2E] dark:text-gray-200'
          }`}
        >
          {attachments.length > 0 && <MessageAttachmentBadges attachments={attachments} message={message} ide={ide} />}
          {showLoadingOnly ? (
            <LoadingStateBubble message={message} fallbackText={message.content} />
          ) : (
            <div className={`min-w-0 max-w-full whitespace-pre-wrap break-words [overflow-wrap:anywhere] ${message.error ? 'break-all' : ''}`}>
              {message.content}
            </div>
          )}
          {!showLoadingOnly && message.pending && <LiveProgressLine message={message} />}
          {actionItems.length > 0 && <CompletedActionItems items={actionItems} />}
          {isSteerMessage && message.status === RUN_STEER_STATUS.failed && (
            <SteerFailureActions message={message} ide={ide} />
          )}
          {!message.pending && message.error && message.resumableRunId && (
            <Button
              type="button"
              variant="danger"
              size="xs"
              onClick={(e) => {
                e.stopPropagation();
                ide.continueAgentRun?.(message.resumableRunId, message.conversation_id);
              }}
              disabled={ide.isChatSending}
              className="mt-3"
            >
              <RefreshCw size={13} />
              继续
            </Button>
          )}
        </div>
        {shouldShowFooterGap && (
          <div
            className={`message-actions pointer-events-none mt-1 flex h-5 max-w-full items-center gap-1 overflow-hidden opacity-0 transition-opacity duration-200 ease-apple group-hover/message:pointer-events-auto group-hover/message:opacity-100 ${
              message.role === 'user' ? 'mr-2' : 'ml-2'
            }`}
          >
            {!!(message.content || '').trim() && (
              <MessageActionButton
                title="复制消息"
                onClick={(e) => {
                  e.stopPropagation();
                  navigator.clipboard?.writeText(message.content).catch((error) => console.error(error));
                }}
              >
                <Copy size={13} />
              </MessageActionButton>
            )}
            {!!(message.content || '').trim() && canRegenerate && (
              <MessageActionButton
                title="重新生成"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.regenerateLastAssistantMessage?.(message.id);
                }}
              >
                <RefreshCw size={13} />
              </MessageActionButton>
            )}
            {!!(message.content || '').trim() && (
              <MessageActionButton
                title="从这里创建分支"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.branchConversation?.(message.id);
                }}
              >
                <GitBranch size={13} />
              </MessageActionButton>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function LiveProgressLine({ message }) {
  const item = latestLiveProgressItem(message);
  if (!item?.text) return null;
  return (
    <div className="mt-2 flex min-w-0 max-w-full items-center gap-1.5 text-[11px] font-semibold text-gray-400 dark:text-gray-500">
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
      <span className="min-w-0 truncate">{item.text}</span>
    </div>
  );
}

function SteerStatusLabel({ message }) {
  const label = runSteerStatusLabel(message.status);
  if (!label) return null;
  const isFailed = message.status === RUN_STEER_STATUS.failed;
  const isPending = message.status === RUN_STEER_STATUS.submitting || message.status === RUN_STEER_STATUS.received;
  const toneClass = isFailed
    ? 'text-amber-600 dark:text-amber-300'
    : isPending
    ? 'text-gray-400 dark:text-gray-500'
    : 'text-blue-500/80 dark:text-blue-200/80';
  return (
    <div
      className={`mb-1 mr-2 flex max-w-full items-center gap-1.5 truncate text-[11px] font-semibold ${toneClass}`}
    >
      <span>{label}</span>
      {isPending && (
        <span className="inline-flex gap-0.5">
          <span className="h-1 w-1 animate-bounce rounded-full bg-current [animation-delay:-0.2s]" />
          <span className="h-1 w-1 animate-bounce rounded-full bg-current [animation-delay:-0.1s]" />
          <span className="h-1 w-1 animate-bounce rounded-full bg-current" />
        </span>
      )}
    </div>
  );
}

function SteerFailureActions({ message, ide }) {
  return (
    <div className="mt-3 flex flex-wrap justify-end gap-1.5 text-blue-50">
      {message.error && (
        <div className="basis-full text-right text-[11px] font-semibold text-blue-50/80">
          {message.error}
        </div>
      )}
      <Button
        type="button"
        variant="subtle"
        size="xs"
        className={STEER_FAILURE_BUTTON_CLASS}
        onClick={(e) => {
          e.stopPropagation();
          ide.retryRunSteerItem?.(message.id);
        }}
        disabled={!ide.isChatSending}
      >
        <RefreshCw size={13} />
        重试
      </Button>
      <Button
        type="button"
        variant="subtle"
        size="xs"
        className={STEER_FAILURE_BUTTON_CLASS}
        onClick={(e) => {
          e.stopPropagation();
          ide.queueRunSteerItemAfterRun?.(message.id);
        }}
      >
        任务结束后发送
      </Button>
      <Button
        type="button"
        variant="subtle"
        size="xs"
        className={STEER_FAILURE_BUTTON_CLASS}
        onClick={(e) => {
          e.stopPropagation();
          ide.discardRunSteerItem?.(message.id);
        }}
      >
        <Trash2 size={13} />
        删除
      </Button>
    </div>
  );
}

function CompletedActionItems({ items }) {
  if ((items || []).length === 0) return null;
  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {items.map((item) => (
        <span
          key={item.id}
          className="rounded-full border border-blue-500/15 bg-blue-500/10 px-2 py-0.5 text-[11px] font-semibold text-blue-500 dark:border-blue-300/15 dark:bg-blue-300/10 dark:text-blue-200"
        >
          {item.label}
        </span>
      ))}
    </div>
  );
}

function LoadingStateBubble({ message, fallbackText }) {
  const visibleText = loadingBubbleText(message, fallbackText);
  return (
    <div className="loading-status-line">
      <span className="loading-status-text">
        <span className="animate-pulse">{visibleText || '\u00A0'}</span>
      </span>
      <span className="inline-flex gap-1">
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.2s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.1s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-current" />
      </span>
    </div>
  );
}

function MessageActionButton({ children, title, onClick }) {
  return (
    <IconButton
      type="button"
      onClick={onClick}
      size="compact"
      className="border border-transparent"
      title={title}
    >
      {children}
    </IconButton>
  );
}

function lastAssistantMessageId(messages) {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const item = messages[index];
    if (item.role === 'assistant' && !item.pending && !item.error) return item.id;
    if (item.role === 'user') return '';
  }
  return '';
}

function MessageAttachmentBadges({ attachments, message, ide }) {
  const taskAttachments = attachments.filter((item) => item.type === 'task');
  const badgeAttachments = attachments.filter((item) => !item.tool_name && item.type !== 'task');
  if (taskAttachments.length === 0 && badgeAttachments.length === 0) return null;
  return (
    <div className={`mb-2 space-y-2 ${message.role === 'user' ? 'text-blue-50' : 'text-gray-400'}`}>
      {taskAttachments.map((item, index) => (
        <div
          key={`${message.id}-task-${index}`}
          className="motion-card rounded-xl border border-blue-500/20 bg-blue-500/10 p-3 text-gray-700 dark:text-gray-200"
        >
          <div className="text-xs font-bold text-blue-500">已创建后台任务</div>
          <div className="mt-1 truncate text-sm font-semibold">{item.title || 'AI 后台任务'}</div>
          <div className="mt-1 text-[11px] text-gray-500 dark:text-gray-400">状态：{item.status || 'pending'}</div>
          <Button
            onClick={(e) => {
              e.stopPropagation();
              ide.tasks?.setSelectedTaskId(item.task_id);
              ide.tasks?.openTaskPanel();
            }}
            size="xs"
            className="mt-2"
          >
            查看任务详情
          </Button>
        </div>
      ))}
      {badgeAttachments.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {badgeAttachments.map((item, index) => (
            <span
              key={`${message.id}-attachment-${index}`}
              className={`motion-chip rounded-full px-2.5 py-1 text-[11px] ${
                message.role === 'user'
                  ? 'bg-white/15 text-blue-50'
                  : 'bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400'
              }`}
            >
              {item.name || '附件'}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

function EmptyChatState({ ide }) {
  const recentConversations = ide.conversations.slice(0, 4);

  return (
    <div className="flex min-h-[360px] h-full items-center justify-center px-2">
      <div className="empty-chat-card w-full max-w-md rounded-3xl border border-gray-100 bg-white/72 p-5 text-center backdrop-blur-xl dark:border-white/10 dark:bg-[#252527]/72">
        <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-blue-50 text-blue-500 dark:bg-blue-500/15 dark:text-blue-300">
          <MessageSquare size={22} />
        </div>
        <div className="mt-4 text-lg font-bold text-gray-900 dark:text-gray-100">Perseus Studio</div>
        <div className="mt-2 text-sm leading-7 text-gray-500 dark:text-gray-400">
          新建一轮对话，或从历史会话继续。AI 会按需调用 workspace 工具来查看或修改文件与文件夹。
        </div>
        <Button
          data-testid="empty-new-conversation-button"
          onClick={(e) => {
            e.stopPropagation();
            ide.createNewConversation();
          }}
          size="lg"
          className="mt-5"
        >
          新建对话
        </Button>

        {recentConversations.length > 0 && (
          <div className="mt-5 border-t border-gray-100 pt-4 text-left dark:border-white/10">
            <div className="mb-2 text-xs font-bold uppercase tracking-[0.18em] text-gray-400">最近会话</div>
            <div className="space-y-1.5">
              {recentConversations.map((conversation) => (
                <div
                  key={conversation.id}
                  role="button"
                  tabIndex={0}
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.selectConversation(conversation.id);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      e.stopPropagation();
                      ide.selectConversation(conversation.id);
                    }
                  }}
                  className="group flex cursor-pointer items-center gap-3 rounded-2xl px-3 py-2.5 text-gray-700 transition-colors hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5"
                >
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-semibold">{conversation.title || '新对话'}</div>
                    <div className="mt-0.5 text-[11px] text-gray-400">{formatDate(conversation.updated_at)}</div>
                  </div>
                  <IconButton
                    onClick={(e) => {
                      e.stopPropagation();
                      ide.deleteConversation(conversation.id);
                    }}
                    variant="danger"
                    size="xs"
                    className="h-7 w-7 opacity-0 group-hover:opacity-100"
                    title="删除会话"
                  >
                    <Trash2 size={13} />
                  </IconButton>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function TypingIndicator({ text }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span>{text}</span>
      <span className="inline-flex gap-1">
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.2s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.1s]" />
        <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-current" />
      </span>
    </span>
  );
}
