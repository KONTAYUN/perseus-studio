import React from 'react';
import { Copy, GitBranch, MessageSquare, RefreshCw, Trash2 } from 'lucide-react';
import { formatDate, formatToolLabel, summarizeCommandOutput, visibleMessageAttachments } from './chatUtils';

export default function ChatMessages({ ide, messagesRef, bottomRef, messagesBottomInset }) {
  return (
    <div
      ref={messagesRef}
      className="relative z-10 flex-1 space-y-1 overflow-y-auto overflow-x-hidden bg-gradient-to-b from-transparent to-gray-50/30 p-4 dark:to-black/10 custom-scrollbar scrollbar-rounded-clip"
      style={{ paddingBottom: `${messagesBottomInset}px` }}
    >
      {ide.chatError && (
        <div className="min-w-0 max-w-full overflow-hidden whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
          {ide.chatError}
          {ide.resumableRun?.runId && (
            <div className="mt-3">
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.continueAgentRun?.();
                }}
                disabled={ide.isChatSending}
                className="inline-flex items-center gap-1.5 rounded-xl bg-red-500 px-3 py-1.5 text-xs font-bold text-white shadow-sm transition hover:bg-red-600 disabled:cursor-not-allowed disabled:opacity-60"
              >
                <RefreshCw size={13} />
                继续
              </button>
            </div>
          )}
        </div>
      )}

      {!ide.currentConversationId && ide.conversationMessages.length === 0 ? (
        <EmptyChatState ide={ide} />
      ) : ide.conversationMessages.length === 0 ? (
        <div className="flex h-full min-h-[240px] items-center justify-center">
          <div className="max-w-sm rounded-3xl border border-gray-100 bg-white/70 px-5 py-4 text-center text-sm leading-7 text-gray-500 shadow-sm dark:border-white/10 dark:bg-[#2C2C2E]/70 dark:text-gray-400">
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
  const showLoadingOnly = message.pending && !(message.content || '').trim();
  return (
    <div className={`group/message flex min-w-0 max-w-full overflow-hidden ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}>
      <div className={`flex min-w-0 max-w-[88%] overflow-hidden flex-col ${message.role === 'user' ? 'items-end' : 'items-start'}`}>
        <div
          className={`motion-message min-w-0 max-w-full overflow-hidden rounded-2xl px-4 py-3 text-[14px] leading-relaxed shadow-sm ${
            message.role === 'user'
              ? 'rounded-tr-sm bg-blue-500 text-white shadow-blue-500/20'
              : message.error
              ? 'rounded-tl-sm border border-red-500/20 bg-red-500/10 text-red-300'
              : message.pending
              ? 'rounded-tl-sm border border-gray-100 bg-white text-gray-500 dark:border-white/5 dark:bg-[#2C2C2E] dark:text-gray-400'
              : 'rounded-tl-sm border border-gray-100 bg-white text-gray-800 dark:border-white/5 dark:bg-[#2C2C2E] dark:text-gray-200'
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
          {!message.pending && message.error && message.resumableRunId && (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                ide.continueAgentRun?.(message.resumableRunId, message.conversation_id);
              }}
              disabled={ide.isChatSending}
              className="mt-3 inline-flex items-center gap-1.5 rounded-xl bg-red-500 px-3 py-1.5 text-xs font-bold text-white shadow-sm transition hover:bg-red-600 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <RefreshCw size={13} />
              继续
            </button>
          )}
        </div>
        {!message.pending && message.content && (
          <div
            className={`pointer-events-none mt-1 flex h-5 max-w-full items-center gap-1 overflow-hidden opacity-0 transition-[opacity,transform] duration-200 ease-apple group-hover/message:pointer-events-auto group-hover/message:translate-y-0 group-hover/message:opacity-100 ${
              message.role === 'user' ? 'mr-2' : 'ml-2'
            }`}
          >
            <MessageActionButton
              title="复制消息"
              onClick={(e) => {
                e.stopPropagation();
                navigator.clipboard?.writeText(message.content).catch((error) => console.error(error));
              }}
            >
              <Copy size={13} />
            </MessageActionButton>
            {canRegenerate && (
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
            <MessageActionButton
              title="从这里创建分支"
              onClick={(e) => {
                e.stopPropagation();
                ide.branchConversation?.(message.id);
              }}
            >
              <GitBranch size={13} />
            </MessageActionButton>
          </div>
        )}
      </div>
    </div>
  );
}

function LoadingStateBubble({ message, fallbackText }) {
  const visibleText = message.loadingState?.visibleText || fallbackText || '正在处理';
  return (
    <div className="flex items-center gap-2">
      <span className="inline-block animate-pulse">{visibleText}</span>
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
    <button
      type="button"
      onClick={onClick}
      className="motion-button flex h-5 w-5 items-center justify-center rounded-md border border-transparent text-gray-500 hover:border-gray-200 hover:bg-white/80 hover:text-blue-500 hover:shadow-sm dark:text-gray-400 dark:hover:border-white/10 dark:hover:bg-white/5 dark:hover:text-blue-300"
      title={title}
    >
      {children}
    </button>
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
  return (
    <div className={`mb-2 space-y-2 ${message.role === 'user' ? 'text-blue-50' : 'text-gray-400'}`}>
      {attachments
        .filter((item) => item.type === 'task')
        .map((item, index) => (
          <div
            key={`${message.id}-task-${index}`}
            className="motion-card rounded-xl border border-blue-500/20 bg-blue-500/10 p-3 text-gray-700 dark:text-gray-200"
          >
            <div className="text-xs font-bold text-blue-500">已创建后台任务</div>
            <div className="mt-1 truncate text-sm font-semibold">{item.title || 'AI 后台任务'}</div>
            <div className="mt-1 text-[11px] text-gray-500 dark:text-gray-400">状态：{item.status || 'pending'}</div>
            <button
              onClick={(e) => {
                e.stopPropagation();
                ide.tasks?.setSelectedTaskId(item.task_id);
                ide.tasks?.openTaskPanel();
              }}
              className="mt-2 rounded-lg bg-blue-500 px-3 py-1.5 text-xs font-bold text-white hover:bg-blue-600"
            >
              查看任务详情
            </button>
          </div>
        ))}
      <div className="flex flex-wrap gap-2">
        {attachments
          .filter((item) => item.tool_name !== 'run_workspace_command' && item.type !== 'task')
          .map((item, index) => (
            <span
              key={`${message.id}-attachment-${index}`}
              className={`motion-chip rounded-full px-2.5 py-1 text-[11px] ${
                message.role === 'user'
                  ? 'bg-white/15 text-blue-50'
                  : item.tool_name
                  ? item.status === 'failed'
                    ? 'border border-red-500/20 bg-red-500/10 text-red-300'
                    : 'border border-blue-500/15 bg-blue-500/10 text-blue-300'
                  : 'bg-gray-100 text-gray-500 dark:bg-white/5 dark:text-gray-400'
              }`}
            >
              {item.tool_name ? item.label || formatToolLabel(item) : item.name || '附件'}
            </span>
          ))}
      </div>
      {attachments
        .filter((item) => item.tool_name === 'run_workspace_command')
        .map((item, index) => (
          <CommandToolEvent key={`${message.id}-command-${index}`} item={item} />
        ))}
    </div>
  );
}

function EmptyChatState({ ide }) {
  const recentConversations = ide.conversations.slice(0, 4);

  return (
    <div className="flex min-h-[360px] h-full items-center justify-center px-2">
      <div className="w-full max-w-md rounded-[28px] border border-gray-100 bg-white/72 p-5 text-center shadow-[0_20px_60px_rgba(15,23,42,0.08)] backdrop-blur-xl dark:border-white/10 dark:bg-[#252527]/72 dark:shadow-[0_24px_70px_rgba(0,0,0,0.28)]">
        <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-blue-50 text-blue-500 dark:bg-blue-500/15 dark:text-blue-300">
          <MessageSquare size={22} />
        </div>
        <div className="mt-4 text-lg font-bold text-gray-900 dark:text-gray-100">Perseus Studio</div>
        <div className="mt-2 text-sm leading-7 text-gray-500 dark:text-gray-400">
          新建一轮对话，或从历史会话继续。AI 会按需调用 workspace 工具来查看或修改文件与文件夹。
        </div>
        <button
          data-testid="empty-new-conversation-button"
          onClick={(e) => {
            e.stopPropagation();
            ide.createNewConversation();
          }}
          className="mt-5 inline-flex items-center justify-center rounded-2xl bg-blue-500 px-5 py-2.5 text-sm font-bold text-white shadow-[0_12px_28px_rgba(59,130,246,0.28)] transition-all hover:-translate-y-0.5 hover:bg-blue-600 active:translate-y-0"
        >
          新建对话
        </button>

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
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      ide.deleteConversation(conversation.id);
                    }}
                    className="rounded-lg p-1.5 text-gray-400 opacity-0 transition-all hover:bg-red-50 hover:text-red-500 group-hover:opacity-100 dark:hover:bg-red-500/10"
                    title="删除会话"
                  >
                    <Trash2 size={13} />
                  </button>
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

function CommandToolEvent({ item }) {
  const exitCode = item.exit_code ?? null;
  const failed = item.status === 'failed' || item.timed_out || (exitCode !== null && exitCode !== 0);
  const title = item.timed_out ? '命令超时' : failed ? '命令失败' : '命令成功';
  const stdout = item.stdout || '';
  const stderr = item.stderr || '';
  const summary = item.error || summarizeCommandOutput(stdout, stderr);

  return (
    <div
      className={`motion-card min-w-0 max-w-full overflow-hidden rounded-xl border px-3 py-2 text-[12px] ${
        failed
          ? 'border-red-500/20 bg-red-500/10 text-red-300'
          : 'border-blue-500/15 bg-blue-500/10 text-blue-300'
      }`}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 font-semibold">
        <span>{title}</span>
        {exitCode !== null && <span>exit {exitCode}</span>}
        <span className="min-w-0 max-w-full truncate text-gray-400">cwd: {item.cwd || '.'}</span>
      </div>
      <div className="mt-1 max-w-full truncate font-mono text-[11px] text-gray-400">
        {item.command || 'command'}
      </div>
      {summary && (
        <pre className="mt-2 max-h-20 max-w-full overflow-hidden whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-lg bg-black/10 p-2 font-mono text-[11px] text-gray-300">
          {summary}
        </pre>
      )}
      {(stdout || stderr) && (
        <details className="mt-2 min-w-0 max-w-full overflow-hidden">
          <summary className="cursor-pointer text-[11px] font-semibold text-gray-400 hover:text-gray-200">
            查看完整输出
          </summary>
          {stdout && (
            <pre className="mt-2 max-h-72 max-w-full overflow-y-auto overflow-x-hidden whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-lg bg-black/20 p-2 font-mono text-[11px] text-gray-300 custom-scrollbar">
              {stdout}
            </pre>
          )}
          {stderr && (
            <pre className="mt-2 max-h-72 max-w-full overflow-y-auto overflow-x-hidden whitespace-pre-wrap break-words [overflow-wrap:anywhere] rounded-lg bg-red-950/20 p-2 font-mono text-[11px] text-red-200 custom-scrollbar">
              {stderr}
            </pre>
          )}
        </details>
      )}
    </div>
  );
}
