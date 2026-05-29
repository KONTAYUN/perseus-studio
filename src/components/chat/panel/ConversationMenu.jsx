import React from 'react';
import { ChevronDown, History, Pencil, Trash2 } from 'lucide-react';
import { formatDate } from '../chatUtils';
import { AnchoredPopover, Button } from '../../ui';

export default function ConversationMenu({ ide, onRenameConversation, variant = 'icon' }) {
  const conversationButtonRef = React.useRef(null);
  const currentConversation = ide.conversations.find((item) => item.id === ide.currentConversationId);
  const isTitleVariant = variant === 'title';
  const title = currentConversation?.title?.trim() || '新对话';

  return (
    <div className={isTitleVariant ? 'relative min-w-0 max-w-full' : 'relative'}>
      <button
        type="button"
        ref={conversationButtonRef}
        data-testid="conversation-history-button"
        onClick={(e) => {
          e.stopPropagation();
          ide.setIsConversationMenuOpen(!ide.isConversationMenuOpen);
        }}
        className={isTitleVariant ? 'motion-button chat-conversation-title-anchor' : 'motion-button ui-icon-button h-8 w-8'}
        title={isTitleVariant ? '打开历史会话' : '历史会话'}
        aria-label={isTitleVariant ? '打开历史会话' : '历史会话'}
      >
        {isTitleVariant ? (
          <>
            <span className="chat-conversation-title-text">{title}</span>
            <ChevronDown size={14} strokeWidth={2} className="chat-conversation-title-chevron" aria-hidden="true" />
          </>
        ) : (
          <History size={15} />
        )}
      </button>
      <AnchoredPopover
        anchorRef={conversationButtonRef}
        align={isTitleVariant ? 'center' : 'end'}
        width={320}
        minWidth={260}
        maxHeight={520}
        open={ide.isConversationMenuOpen}
        onClose={() => ide.setIsConversationMenuOpen(false)}
        testId="conversation-history-popover"
      >
        {({ position }) => (
          <>
            <div className="flex min-w-0 items-start justify-between gap-3 border-b border-gray-100 px-3 py-2.5 dark:border-white/10">
              <div className="min-w-0">
                <div className="text-sm font-bold text-gray-800 dark:text-gray-100">历史会话</div>
                <div className="truncate text-[11px] text-gray-400">
                  {currentConversation?.title || '当前没有选中会话'}
                </div>
              </div>
              <Button
                type="button"
                data-testid="conversation-menu-create-button"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.createNewConversation();
                }}
                size="xs"
                className="shrink-0 whitespace-nowrap"
              >
                新建
              </Button>
            </div>
            <div
              className="overflow-y-auto p-1.5 custom-scrollbar scrollbar-rounded-clip"
              style={{ maxHeight: Math.max(160, position.maxHeight - 56) }}
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
