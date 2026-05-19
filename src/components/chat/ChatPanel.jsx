import React from 'react';
import { ChevronDown } from 'lucide-react';
import ChatComposer from './ChatComposer';
import ChatMessages from './ChatMessages';
import ChatPanelHeader from './panel/ChatPanelHeader';
import RenameConversationDialog from './panel/RenameConversationDialog';
import { useChatScrollController } from './panel/useChatScrollController';

export default function ChatPanel({ ide, panelId = 'chat' }) {
  const [renamingConversation, setRenamingConversation] = React.useState(null);
  const [renameDraft, setRenameDraft] = React.useState('');
  const {
    bottomRef,
    composerHeight,
    composerRef,
    messagesBottomInset,
    messagesRef,
    scrollToBottom,
    showScrollToBottom,
    submitChatMessage,
  } = useChatScrollController(ide);

  const openRenameConversation = (conversation) => {
    setRenamingConversation(conversation);
    setRenameDraft(conversation.title || '新对话');
  };

  const closeRenameConversation = () => {
    setRenamingConversation(null);
    setRenameDraft('');
  };

  const submitRenameConversation = (e) => {
    e.preventDefault();
    if (!renamingConversation) return;
    const title = renameDraft.trim();
    if (title && title !== renamingConversation.title) {
      ide.renameConversation(renamingConversation.id, title);
    }
    closeRenameConversation();
  };

  return (
    <section
      id="panel-content-chat"
      data-testid="chat-panel"
      className={`ui-panel-surface relative h-full flex flex-col bg-white/85 dark:bg-[#1C1C1E]/85 backdrop-blur-2xl overflow-visible transition-[border-color,box-shadow,background-color,transform] duration-300 ease-[cubic-bezier(0.19,1,0.22,1)] border ${ide.getPanelGlowStyle(
        panelId
      )}`}
      onClick={() => ide.setActivePanel(panelId)}
    >
      <ChatPanelHeader
        ide={ide}
        panelId={panelId}
        onRenameConversation={openRenameConversation}
      />

      <ChatMessages
        ide={ide}
        messagesRef={messagesRef}
        bottomRef={bottomRef}
        messagesBottomInset={messagesBottomInset}
      />

      {showScrollToBottom && ide.conversationMessages.length > 0 && (
        <button
          type="button"
          data-testid="scroll-to-bottom-button"
          onClick={(e) => {
            e.stopPropagation();
            scrollToBottom('smooth', { force: true });
          }}
          className="scroll-to-bottom-button motion-button motion-soft-focus absolute left-1/2 z-30 flex h-8 w-11 items-center justify-center rounded-2xl border border-transparent backdrop-blur-2xl"
          style={{ bottom: `${composerHeight + 18}px`, transform: 'translateX(-50%)' }}
          title="回到底部"
          aria-label="回到底部"
        >
          <ChevronDown size={17} />
        </button>
      )}

      <ChatComposer
        ide={ide}
        panelId={panelId}
        composerRef={composerRef}
        submitChatMessage={submitChatMessage}
      />

      <RenameConversationDialog
        draft={renameDraft}
        onClose={closeRenameConversation}
        onDraftChange={setRenameDraft}
        onSubmit={submitRenameConversation}
        open={Boolean(renamingConversation)}
      />
    </section>
  );
}
