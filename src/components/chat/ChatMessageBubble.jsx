import React from 'react';
import { Copy, GitBranch, RefreshCw, Trash2 } from 'lucide-react';
import { completedActionItems, latestLiveProgressItem, loadingStatusDisplayText, loadingStatusReserveText, visibleMessageAttachments, shouldReserveMessageFooter } from './chatUtils';
import { Button, IconButton } from '../ui';
import { RUN_STEER_STATUS, runSteerStatusLabel } from '../../hooks/chatSteerItems';
import ChatMarkdownRenderer from './markdown/ChatMarkdownRenderer';

const STEER_FAILURE_BUTTON_CLASS = 'border-white/20 bg-white/10 text-blue-50 hover:bg-white/15 hover:text-white dark:border-white/15 dark:bg-white/10';

function MessageBubble({ message, ide, canRegenerate }) {
  const attachments = visibleMessageAttachments(message);
  const actionItems = completedActionItems(message);
  const showLoadingOnly = message.pending && !(message.content || '').trim();
  const shouldShowFooterGap = shouldReserveMessageFooter(message);
  const isSteerMessage = message.kind === 'steer';
  return (
    <div className={`group/message flex min-w-0 max-w-full overflow-hidden ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}>
      <div className={`flex min-w-0 max-w-[88%] flex-col ${message.role === 'user' ? 'items-end' : 'items-start'}`}>
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
          ) : message.role === 'assistant' && !message.error ? (
            <ChatMarkdownRenderer content={message.content} />
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
            className={`message-actions motion-fade-reveal pointer-events-none mt-1 flex h-5 max-w-full items-center gap-1 overflow-hidden opacity-0 group-hover/message:pointer-events-auto group-hover/message:opacity-100 ${
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

const MemoizedMessageBubble = React.memo(MessageBubble, areMessageBubblePropsEqual);

function areMessageBubblePropsEqual(previous, next) {
  return (
    previous.canRegenerate === next.canRegenerate &&
    previous.ide?.isChatSending === next.ide?.isChatSending &&
    isSameRenderableMessage(previous.message, next.message)
  );
}

function isSameRenderableMessage(previous = {}, next = {}) {
  return (
    previous.id === next.id &&
    previous.role === next.role &&
    previous.kind === next.kind &&
    previous.status === next.status &&
    previous.content === next.content &&
    previous.pending === next.pending &&
    previous.error === next.error &&
    previous.resumableRunId === next.resumableRunId &&
    previous.attachments === next.attachments &&
    previous.progressItems === next.progressItems &&
    previous.loadingState === next.loadingState
  );
}

function LiveProgressLine({ message }) {
  const item = latestLiveProgressItem(message);
  const text = item?.text || loadingStatusDisplayText(message, '');
  const reserveText = loadingStatusReserveText(message, text);
  if (!text && !reserveText) return null;
  return (
    <div className="loading-status-line loading-status-line-compact mt-2 text-[11px] font-semibold text-gray-400 dark:text-gray-500">
      <span className="loading-status-pulse h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
      <LoadingStatusText text={text} reserveText={reserveText} dotSize="sm" />
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
  const visibleText = loadingStatusDisplayText(message, fallbackText);
  const reserveText = loadingStatusReserveText(message, visibleText, fallbackText);
  return (
    <div className="loading-status-line">
      <LoadingStatusText text={visibleText} reserveText={reserveText} pulse />
    </div>
  );
}

function LoadingStatusText({ text, reserveText, pulse = false, dotSize = 'md' }) {
  const displayText = text || '\u00A0';
  const reservedText = reserveText || text || '\u00A0';
  return (
    <span className="loading-status-text">
      <span className="loading-status-flow loading-status-visible">
        <span className={pulse ? 'animate-pulse' : undefined}>{displayText}</span>
        <LoadingStatusDots size={dotSize} />
      </span>
      <span className="loading-status-flow loading-status-reserve" aria-hidden="true">
        <span>{reservedText}</span>
        <LoadingStatusDots size={dotSize} />
      </span>
    </span>
  );
}

function LoadingStatusDots({ size = 'md' }) {
  const dotClass = size === 'sm' ? 'h-1 w-1' : 'h-1.5 w-1.5';
  const gapClass = size === 'sm' ? 'gap-0.5' : 'gap-1';
  return (
    <span className={`loading-status-dots ${gapClass}`}>
      <span className={`${dotClass} animate-bounce rounded-full bg-current [animation-delay:-0.2s]`} />
      <span className={`${dotClass} animate-bounce rounded-full bg-current [animation-delay:-0.1s]`} />
      <span className={`${dotClass} animate-bounce rounded-full bg-current`} />
    </span>
  );
}

function MessageActionButton({ children, title, onClick }) {
  const [confirmed, setConfirmed] = React.useState(false);
  const timeoutRef = React.useRef(null);

  React.useEffect(() => () => window.clearTimeout(timeoutRef.current), []);

  return (
    <IconButton
      type="button"
      onClick={(event) => {
        onClick?.(event);
        setConfirmed(true);
        window.clearTimeout(timeoutRef.current);
        timeoutRef.current = window.setTimeout(() => setConfirmed(false), 700);
      }}
      size="compact"
      className={`border border-transparent ${confirmed ? 'motion-copy-confirm' : ''}`}
      title={title}
    >
      {children}
    </IconButton>
  );
}

export function lastAssistantMessageId(messages) {
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

export default MemoizedMessageBubble;
