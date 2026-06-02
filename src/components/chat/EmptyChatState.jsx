import React from 'react';
import { MessageSquare, Trash2 } from 'lucide-react';
import { Button, IconButton } from '../ui';
import { formatDate } from './chatUtils';

export default function EmptyChatState({ ide }) {
  const recentConversations = ide.conversations.slice(0, 4);

  return (
    <div className="flex h-full items-center justify-center px-2 pb-28">
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
