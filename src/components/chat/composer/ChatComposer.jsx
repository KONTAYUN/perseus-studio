import React from 'react';
import { pendingFollowupQuestion } from '../chatUtils';
import ComposerAttachments from './ComposerAttachments';
import ComposerInput from './ComposerInput';
import ComposerReferences from './ComposerReferences';
import FollowupQuestionPanel from './FollowupQuestionPanel';
import PendingRunMessagePanel from './PendingRunMessagePanel';
import RunSteerPreviewPanel from './RunSteerPreviewPanel';

export default function ChatComposer({ ide, panelId = 'chat', composerRef, submitChatMessage }) {
  const attachmentInputRef = React.useRef(null);
  const chatInputRef = React.useRef(null);
  const didMountFocusRef = React.useRef(false);
  const [isSkillMenuOpen, setIsSkillMenuOpen] = React.useState(false);
  const [answeringQuestionId, setAnsweringQuestionId] = React.useState('');
  const isCurrentConversationSending = ide.isCurrentConversationSending ?? ide.isChatSending;
  const visiblePendingRunMessage = visibleConversationItem(ide.pendingRunMessage, ide.currentConversationId);
  const visibleRunSteerItems = (ide.runSteerItems || []).filter((item) => item.conversation_id === ide.currentConversationId);
  const visibleRunSteerNotice = visibleConversationItem(ide.runSteerNotice, ide.currentConversationId);
  const followupQuestion = pendingFollowupQuestion(ide.conversationMessages);
  const isAnsweringFollowup = !!answeringQuestionId && answeringQuestionId === followupQuestion?.messageId;
  const isSendDisabled =
    !isCurrentConversationSending &&
    !ide.chatInput.trim() &&
    ide.chatAttachments.length === 0 &&
    ide.chatFileReferences.length === 0 &&
    ide.chatSkillReferences.length === 0;

  React.useEffect(() => {
    if (!didMountFocusRef.current) {
      didMountFocusRef.current = true;
      return;
    }
    if (!isCurrentConversationSending) {
      window.setTimeout(() => chatInputRef.current?.focus(), 0);
    }
  }, [isCurrentConversationSending]);

  React.useEffect(() => {
    if (!followupQuestion || answeringQuestionId !== followupQuestion.messageId) {
      setAnsweringQuestionId('');
    }
  }, [answeringQuestionId, followupQuestion]);

  const handleDragOver = React.useCallback((e) => {
    if (isComposerDrag(e)) {
      e.preventDefault();
    }
  }, []);

  const handleDrop = React.useCallback((e) => {
    handleComposerDrop(e, ide);
  }, [ide]);

  return (
    <div
      ref={composerRef}
      className="pointer-events-none absolute inset-x-0 bottom-0 z-20 bg-transparent px-5 pb-4 pt-2 max-[640px]:px-3"
      onDragOver={handleDragOver}
      onDrop={handleDrop}
      onClick={(e) => {
        e.stopPropagation();
        ide.setActivePanel('chat-input');
      }}
    >
      <input
        ref={attachmentInputRef}
        data-testid="chat-attachment-input"
        type="file"
        multiple
        className="hidden"
        onChange={(e) => {
          ide.addChatAttachments(e.target.files);
          e.target.value = '';
        }}
      />
      {followupQuestion && (
        <FollowupQuestionPanel
          question={followupQuestion}
          disabled={isCurrentConversationSending || isAnsweringFollowup}
          onChoose={async (option) => {
            setAnsweringQuestionId(followupQuestion.messageId);
            try {
              await ide.sendFollowupAnswer?.(option);
            } finally {
              setAnsweringQuestionId('');
            }
          }}
        />
      )}
      {visiblePendingRunMessage && (
        <PendingRunMessagePanel
          message={visiblePendingRunMessage}
          notice={visibleRunSteerNotice?.status === 'warn' || visibleRunSteerNotice?.status === 'error' ? visibleRunSteerNotice : null}
          disabled={!isCurrentConversationSending}
          onSteer={ide.sendPendingRunMessageAsSteer}
          onDelete={ide.discardPendingRunMessage}
        />
      )}
      {!visiblePendingRunMessage && (
        <RunSteerPreviewPanel
          items={visibleRunSteerItems}
          onDelete={ide.discardRunSteerItem}
          onQueueAfterRun={ide.queueRunSteerItemAfterRun}
          onRetry={ide.retryRunSteerItem}
        />
      )}
      {!visiblePendingRunMessage && (visibleRunSteerNotice?.status === 'warn' || visibleRunSteerNotice?.status === 'error') && visibleRunSteerNotice?.text && (
        <div className="run-steer-notice-panel motion-popover pointer-events-auto mb-3">
          {visibleRunSteerNotice.text}
        </div>
      )}
      <div
        className={`composer-shell motion-soft-focus pointer-events-auto relative overflow-visible border bg-white/88 backdrop-blur-2xl dark:bg-[#171719]/92 ${ide.getInputGlowStyle()}`}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
        onClick={(e) => {
          e.stopPropagation();
          ide.setActivePanel('chat-input');
        }}
      >
        <ComposerAttachments
          attachments={ide.chatAttachments}
          onRemove={ide.removeChatAttachment}
        />
        <ComposerReferences
          fileReferences={ide.chatFileReferences}
          skillReferences={ide.chatSkillReferences}
          onRemoveFileReference={ide.removeWorkspaceFileReference}
          onRemoveSkillReference={ide.removeSkillReference}
        />
        <ComposerInput
          ide={{ ...ide, isChatSending: isCurrentConversationSending }}
          attachmentInputRef={attachmentInputRef}
          inputRef={chatInputRef}
          isSkillMenuOpen={isSkillMenuOpen}
          panelId={panelId}
          setIsSkillMenuOpen={setIsSkillMenuOpen}
          submitChatMessage={submitChatMessage}
          isSendDisabled={isSendDisabled}
        />
      </div>
    </div>
  );
}

function visibleConversationItem(item, conversationId) {
  if (!item) return null;
  if (!conversationId) return null;
  const itemConversationId = item.conversationId || item.conversation_id || '';
  return !itemConversationId || itemConversationId === conversationId ? item : null;
}

function isComposerDrag(e) {
  return (
    e.dataTransfer?.types?.includes('Files') ||
    e.dataTransfer?.types?.includes('workspace_refs') ||
    e.dataTransfer?.types?.includes('workspace_path')
  );
}

function handleComposerDrop(e, ide) {
  const refsRaw = e.dataTransfer?.getData('workspace_refs');
  if (refsRaw) {
    e.preventDefault();
    e.stopPropagation();
    try {
      ide.addWorkspaceFileReferences(JSON.parse(refsRaw));
    } catch {
      const path = e.dataTransfer?.getData('workspace_path');
      if (path) ide.addWorkspaceFileReferences({ path });
    }
    return;
  }
  const workspacePath = e.dataTransfer?.getData('workspace_path');
  if (workspacePath && e.dataTransfer?.getData('workspace_type') === 'file') {
    e.preventDefault();
    e.stopPropagation();
    ide.addWorkspaceFileReferences({
      path: workspacePath,
      name: workspacePath.split('/').pop() || workspacePath,
      type: 'file',
      file_type: ide.detectFileType?.(workspacePath) || 'file',
    });
    return;
  }
  if (e.dataTransfer?.files?.length) {
    e.preventDefault();
    e.stopPropagation();
    ide.addChatAttachments(e.dataTransfer.files);
  }
}
