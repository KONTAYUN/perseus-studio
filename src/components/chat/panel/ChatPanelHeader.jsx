import React from 'react';
import { Home, PanelTop, Plus } from 'lucide-react';
import ContextUsageBadge from './ContextUsageBadge';
import ConversationMenu from './ConversationMenu';
import ModelMenu from './ModelMenu';

export default function ChatPanelHeader({ ide, onRenameConversation, panelId }) {
  return (
    <div className="chat-panel-header relative z-20 grid h-12 flex-none grid-cols-[104px_minmax(0,1fr)_104px] items-center gap-2 border-b border-gray-100 bg-transparent px-3 dark:border-white/5 group max-[420px]:grid-cols-[82px_minmax(0,1fr)_96px]">
      <div className="flex min-w-0 items-center gap-1 text-gray-400">
        <span
          draggable
          onDragStart={(e) => ide.handlePanelDragStart(e, panelId)}
          onDragEnd={ide.handlePanelDragEnd}
          className="motion-button ui-icon-button ui-panel-drag-handle h-7 w-7"
          title="拖动面板"
        >
          <PanelTop size={15} strokeWidth={1.9} className="shrink-0" />
        </span>
        <ContextUsageBadge
          usage={ide.contextUsage}
          notice={ide.contextCompactionNotice}
        />
      </div>

      <div className="flex min-w-0 justify-center">
        <ModelMenu ide={ide} panelId={panelId} />
      </div>

      <div className="flex min-w-0 items-center justify-end gap-1">
        {ide.currentConversationId && (
          <button
            data-testid="chat-home-button"
            onClick={(e) => {
              e.stopPropagation();
              ide.returnChatHome();
            }}
            className="motion-button ui-icon-button h-8 w-8"
            title="返回首页"
          >
            <Home size={15} />
          </button>
        )}
        <button
          data-testid="new-conversation-button"
          onClick={(e) => {
            e.stopPropagation();
            ide.createNewConversation();
          }}
          className="motion-button ui-icon-button h-8 w-8"
          title="新建会话"
        >
          <Plus size={15} />
        </button>
        <ConversationMenu ide={ide} onRenameConversation={onRenameConversation} />
      </div>
    </div>
  );
}
