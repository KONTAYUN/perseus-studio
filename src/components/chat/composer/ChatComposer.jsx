import React from 'react';
import { pendingFollowupQuestion } from '../chatUtils';
import ComposerAttachments from './ComposerAttachments';
import ComposerInput from './ComposerInput';
import ComposerReferences from './ComposerReferences';
import FollowupQuestionPanel from './FollowupQuestionPanel';
import PendingRunMessagePanel from './PendingRunMessagePanel';

export default function ChatComposer({ ide, panelId = 'chat', composerRef, submitChatMessage }) {
  const attachmentInputRef = React.useRef(null);
  const chatInputRef = React.useRef(null);
  const didMountFocusRef = React.useRef(false);
  const [isSkillMenuOpen, setIsSkillMenuOpen] = React.useState(false);
  const [answeringQuestionId, setAnsweringQuestionId] = React.useState('');
  const followupQuestion = pendingFollowupQuestion(ide.conversationMessages);
  const isAnsweringFollowup = !!answeringQuestionId && answeringQuestionId === followupQuestion?.messageId;
  const isSendDisabled =
    !ide.isChatSending &&
    !ide.chatInput.trim() &&
    ide.chatAttachments.length === 0 &&
    ide.chatFileReferences.length === 0 &&
    ide.chatSkillReferences.length === 0;

  React.useEffect(() => {
    if (!didMountFocusRef.current) {
      didMountFocusRef.current = true;
      return;
    }
    if (!ide.isChatSending) {
      window.setTimeout(() => chatInputRef.current?.focus(), 0);
    }
  }, [ide.isChatSending]);

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
          disabled={ide.isChatSending || isAnsweringFollowup}
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
      {ide.pendingRunMessage && (
        <PendingRunMessagePanel
          message={ide.pendingRunMessage}
          notice={ide.runSteerNotice?.status === 'warn' || ide.runSteerNotice?.status === 'error' ? ide.runSteerNotice : null}
          disabled={!ide.isChatSending}
          onSteer={ide.sendPendingRunMessageAsSteer}
          onDelete={ide.discardPendingRunMessage}
        />
      )}
      {!ide.pendingRunMessage && (ide.runSteerNotice?.status === 'warn' || ide.runSteerNotice?.status === 'error') && ide.runSteerNotice?.text && (
        <div className="run-steer-notice-panel motion-popover pointer-events-auto mb-3">
          {ide.runSteerNotice.text}
        </div>
      )}
      <div
        className={`composer-shell motion-soft-focus pointer-events-auto relative overflow-visible border bg-white/88 px-4 py-2.5 backdrop-blur-2xl dark:bg-[#171719]/92 max-[640px]:px-3 ${ide.getInputGlowStyle()}`}
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
          ide={ide}
          attachmentInputRef={attachmentInputRef}
          inputRef={chatInputRef}
          isSkillMenuOpen={isSkillMenuOpen}
          setIsSkillMenuOpen={setIsSkillMenuOpen}
          submitChatMessage={submitChatMessage}
          isSendDisabled={isSendDisabled}
        />
      </div>
      <div className="text-center mt-2.5 text-[11px] text-gray-400 dark:text-gray-500 font-medium">
        支持按 Enter 发送。后台任务将持续为您服务。
      </div>
    </div>
  );
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
