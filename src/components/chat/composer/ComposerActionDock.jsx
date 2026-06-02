import React from 'react';
import { Paperclip, Send, Square } from 'lucide-react';
import ModelMenu from './ModelMenu';
import { composerPrimaryButtonState } from './composerActionState';

export default function ComposerActionDock({
  attachmentInputRef,
  hasDraft,
  isChatSending,
  isSendDisabled,
  onSend,
  onStop,
  ide,
  panelId,
}) {
  const handleAttachmentClick = React.useCallback((e) => {
    e.stopPropagation();
    attachmentInputRef.current?.click();
  }, [attachmentInputRef]);

  const handlePrimaryAction = React.useCallback((e) => {
    e.stopPropagation();
    if (isChatSending && !hasDraft) {
      onStop?.();
      return;
    }
    onSend?.();
  }, [hasDraft, isChatSending, onSend, onStop]);

  const primaryState = composerPrimaryButtonState({ hasDraft, isChatSending, isSendDisabled });
  const isStopAction = primaryState.action === 'stop';

  return (
    <div className="composer-action-dock">
      <div className="composer-action-left">
        <button
          data-testid="chat-attachment-button"
          onClick={handleAttachmentClick}
          className="motion-button composer-icon-button composer-attach-button pointer-events-auto flex items-center justify-center"
          title="上传文件"
        >
          <Paperclip size={16} />
        </button>
      </div>
      <div className="composer-action-right">
        <ModelMenu ide={ide} panelId={panelId} variant="composer" />
        <button
          data-testid="chat-send-button"
          onClick={handlePrimaryAction}
          disabled={primaryState.disabled}
          className={`motion-button composer-icon-button pointer-events-auto flex items-center justify-center ${primaryState.toneClass}`}
          title={primaryState.title}
        >
          {isStopAction ? (
            <Square size={13} fill="currentColor" strokeWidth={2.5} />
          ) : (
            <Send size={15} className="composer-send-icon" strokeWidth={2.5} />
          )}
        </button>
      </div>
    </div>
  );
}
