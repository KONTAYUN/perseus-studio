import React from 'react';
import { Paperclip, Send, Square } from 'lucide-react';
import ModelMenu from './ModelMenu';

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
          disabled={isSendDisabled}
          className={`motion-button composer-icon-button pointer-events-auto flex items-center justify-center ${
            isChatSending && !hasDraft
              ? 'composer-stop-button'
              : isSendDisabled
              ? 'composer-send-button-disabled'
              : 'composer-send-button'
          }`}
          title={isChatSending && hasDraft ? '挂起发送' : isChatSending ? '停止生成' : '发送'}
        >
          {isChatSending && !hasDraft ? (
            <Square size={13} fill="currentColor" strokeWidth={2.5} />
          ) : (
            <Send size={15} className="composer-send-icon" strokeWidth={2.5} />
          )}
        </button>
      </div>
    </div>
  );
}
