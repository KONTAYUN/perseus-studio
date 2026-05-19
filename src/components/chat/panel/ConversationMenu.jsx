import React from 'react';
import { Archive, History, Pencil, Plus, Trash2 } from 'lucide-react';
import { formatDate } from '../chatUtils';
import { AnchoredPopover, Button } from '../../ui';

export default function ConversationMenu({ ide, onRenameConversation }) {
  const conversationButtonRef = React.useRef(null);
  const currentConversation = ide.conversations.find((item) => item.id === ide.currentConversationId);

  return (
    <div className="relative">
      <button
        ref={conversationButtonRef}
        data-testid="conversation-history-button"
        onClick={(e) => {
          e.stopPropagation();
          ide.setIsConversationMenuOpen(!ide.isConversationMenuOpen);
        }}
        className="motion-button ui-icon-button h-8 w-8"
        title="历史会话"
      >
        <History size={15} />
      </button>
      <AnchoredPopover
        anchorRef={conversationButtonRef}
        width={320}
        minWidth={260}
        maxHeight={520}
        open={ide.isConversationMenuOpen}
        onClose={() => ide.setIsConversationMenuOpen(false)}
        testId="conversation-history-popover"
      >
        {({ position }) => (
          <>
            <div className="flex items-center justify-between border-b border-gray-100 px-3 py-2.5 dark:border-white/10">
              <div>
                <div className="text-sm font-bold text-gray-800 dark:text-gray-100">历史会话</div>
                <div className="text-[11px] text-gray-400">
                  {currentConversation?.title || '当前没有选中会话'}
                </div>
              </div>
              <Button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.createNewConversation();
                }}
                size="xs"
              >
                新建
              </Button>
            </div>
            {ide.currentConversationId && (
              <div className="border-b border-gray-100 p-1.5 dark:border-white/10">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.compactCurrentConversation?.('');
                    ide.setIsConversationMenuOpen(false);
                  }}
                  disabled={ide.isChatSending}
                  className="motion-row flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm font-semibold text-gray-700 hover:bg-blue-50 hover:text-blue-600 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-200 dark:hover:bg-blue-500/15 dark:hover:text-blue-200"
                >
                  <Archive size={14} className="shrink-0" />
                  <span className="min-w-0 flex-1">压缩上下文</span>
                </button>
              </div>
            )}
            <div
              className="overflow-y-auto p-1.5 custom-scrollbar scrollbar-rounded-clip"
              style={{ maxHeight: Math.max(160, position.maxHeight - (ide.currentConversationId ? 104 : 56)) }}
            >
              {ide.conversations.length === 0 ? (
                <div className="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                  还没有历史会话。
                </div>
              ) : (
                ide.conversations.map((conversation) => (
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
                    className={`motion-row conversation-item-row group flex w-full items-center gap-2 rounded-xl px-3 py-2.5 text-left ${
                      conversation.id === ide.currentConversationId
                        ? 'bg-blue-50 text-blue-600 dark:bg-blue-500/15 dark:text-blue-300'
                        : 'text-gray-700 hover:bg-gray-50 dark:text-gray-200 dark:hover:bg-white/5'
                    }`}
                  >
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-bold">{conversation.title || '新对话'}</div>
                      <div className="mt-0.5 truncate text-[11px] text-gray-400">
                        {formatDate(conversation.updated_at)}
                      </div>
                    </div>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        onRenameConversation(conversation);
                      }}
                      className="motion-button ui-icon-button h-7 w-7 opacity-0 group-hover:opacity-100"
                      title="重命名会话"
                    >
                      <Pencil size={13} />
                    </button>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation();
                        ide.deleteConversation(conversation.id);
                      }}
                      className="motion-button ui-icon-button ui-icon-button-danger h-7 w-7 opacity-0 group-hover:opacity-100"
                      title="删除会话"
                    >
                      <Trash2 size={13} />
                    </button>
                  </div>
                ))
              )}
            </div>
          </>
        )}
      </AnchoredPopover>
    </div>
  );
}
