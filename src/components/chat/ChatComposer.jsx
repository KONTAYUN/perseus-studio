import React from 'react';
import { File, MessageSquare, Paperclip, Send, Square, X } from 'lucide-react';
import { formatFileSize, getSlashSkillQuery, removeSlashSkillQuery } from './chatUtils';

const CHAT_INPUT_MIN_HEIGHT = 56;
const CHAT_INPUT_MAX_HEIGHT = 260;
const CHAT_INPUT_BUTTON_SAFE_INSET = 50;
const CHAT_INPUT_BUTTON_SAFE_TRIGGER = 84;

export default function ChatComposer({ ide, panelId = 'chat', composerRef, submitChatMessage }) {
  const attachmentInputRef = React.useRef(null);
  const chatInputRef = React.useRef(null);
  const didMountFocusRef = React.useRef(false);
  const [isSkillMenuOpen, setIsSkillMenuOpen] = React.useState(false);

  const enabledSkills = ide.skills.filter((skill) => skill.enabled);
  const skillQuery = getSlashSkillQuery(ide.chatInput);
  const filteredSkills = enabledSkills.filter((skill) => {
    if (skillQuery === null) return false;
    const query = skillQuery.toLowerCase();
    if (!query) return true;
    return `${skill.name} ${skill.description || ''}`.toLowerCase().includes(query);
  });
  const isSendDisabled =
    !ide.isChatSending &&
    !ide.chatInput.trim() &&
    ide.chatAttachments.length === 0 &&
    ide.chatFileReferences.length === 0 &&
    ide.chatSkillReferences.length === 0;

  const resizeChatInput = React.useCallback(() => {
    const element = chatInputRef.current;
    if (!element) return;
    const value = element.value || '';
    const isCaretAtEnd = element.selectionStart >= value.length && element.selectionEnd >= value.length;

    element.style.height = 'auto';
    element.style.paddingBottom = '';

    if (!value.trim()) {
      element.style.height = `${CHAT_INPUT_MIN_HEIGHT}px`;
      element.style.overflowY = 'hidden';
      return;
    }

    if (element.scrollHeight > CHAT_INPUT_BUTTON_SAFE_TRIGGER) {
      element.style.paddingBottom = `${CHAT_INPUT_BUTTON_SAFE_INSET}px`;
    }

    const nextHeight = Math.min(Math.max(element.scrollHeight, CHAT_INPUT_MIN_HEIGHT), CHAT_INPUT_MAX_HEIGHT);
    element.style.height = `${nextHeight}px`;
    const isScrollable = element.scrollHeight > CHAT_INPUT_MAX_HEIGHT;
    element.style.overflowY = isScrollable ? 'auto' : 'hidden';

    if (isScrollable && isCaretAtEnd) {
      window.requestAnimationFrame(() => {
        element.scrollTop = element.scrollHeight;
      });
    }
  }, []);

  React.useLayoutEffect(() => {
    resizeChatInput();
  }, [ide.chatInput, resizeChatInput]);

  React.useEffect(() => {
    const timer = window.setTimeout(() => resizeChatInput(), 780);
    return () => window.clearTimeout(timer);
  }, [panelId, resizeChatInput]);

  React.useEffect(() => {
    if (!didMountFocusRef.current) {
      didMountFocusRef.current = true;
      return;
    }
    if (!ide.isChatSending) {
      window.setTimeout(() => chatInputRef.current?.focus(), 0);
    }
  }, [ide.isChatSending]);

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
      <div
        className={`motion-soft-focus pointer-events-auto relative overflow-visible rounded-[28px] border bg-white/88 px-4 py-2.5 shadow-[0_18px_44px_rgba(15,23,42,0.10),inset_0_1px_0_rgba(255,255,255,0.72)] backdrop-blur-2xl dark:bg-[#171719]/92 dark:shadow-[0_18px_46px_rgba(0,0,0,0.34),inset_0_1px_0_rgba(255,255,255,0.07)] max-[640px]:px-3 ${ide.getInputGlowStyle()}`}
        onDragOver={handleDragOver}
        onDrop={handleDrop}
        onClick={(e) => {
          e.stopPropagation();
          ide.setActivePanel('chat-input');
        }}
      >
        {ide.chatAttachments.length > 0 && (
          <div className="mb-2 flex flex-wrap gap-2 px-1.5 pt-0.5">
            {ide.chatAttachments.map((item) => (
              <div
                key={item.id}
                className="motion-chip flex max-w-full items-center gap-2 rounded-2xl border border-gray-200/70 bg-gray-50/70 px-3 py-1.5 text-xs text-gray-600 dark:border-white/10 dark:bg-white/[0.05] dark:text-gray-300"
              >
                <File size={13} className="shrink-0 text-blue-500" />
                <span className="max-w-[180px] truncate">{item.name}</span>
                <span className="shrink-0 text-[10px] text-gray-400">{formatFileSize(item.size)}</span>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.removeChatAttachment(item.id);
                  }}
                  className="rounded-md p-0.5 text-gray-400 hover:bg-gray-100 hover:text-red-500 dark:hover:bg-white/10"
                  title="移除附件"
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
        )}
        {(ide.chatFileReferences.length > 0 || ide.chatSkillReferences.length > 0) && (
          <div className="mb-2 flex flex-wrap gap-2 px-1.5 pt-0.5">
            {ide.chatFileReferences.map((item) => (
              <ReferenceChip
                key={item.path}
                icon={<File size={13} />}
                title={item.name}
                subtitle={item.path}
                onRemove={() => ide.removeWorkspaceFileReference(item.path)}
              />
            ))}
            {ide.chatSkillReferences.map((item) => (
              <ReferenceChip
                key={item.skill_id}
                icon={<MessageSquare size={13} />}
                title={item.name}
                subtitle="Skill"
                onRemove={() => ide.removeSkillReference(item.skill_id)}
              />
            ))}
          </div>
        )}
        <div className="relative flex items-end">
          <textarea
            ref={chatInputRef}
            data-testid="chat-input"
            value={ide.chatInput}
            onChange={(e) => {
              ide.setChatInput(e.target.value);
              setIsSkillMenuOpen(getSlashSkillQuery(e.target.value) !== null);
              requestAnimationFrame(resizeChatInput);
            }}
            rows="1"
            placeholder={ide.isChatSending ? '继续输入，消息会排队发送...' : '发送指令或拖拽文件...'}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                submitChatMessage();
                setIsSkillMenuOpen(false);
              }
            }}
            onPaste={(e) => {
              const pastedFiles = ide.addClipboardAttachments(e.clipboardData);
              if (pastedFiles > 0) {
                e.preventDefault();
                return;
              }
              requestAnimationFrame(() => {
                resizeChatInput();
                requestAnimationFrame(resizeChatInput);
              });
            }}
            className="w-full min-h-[56px] bg-transparent border-none focus:ring-0 resize-none pb-3.5 pt-3.5 pl-4 pr-[88px] text-sm leading-[24px] text-gray-800 outline-none placeholder-gray-400 dark:text-gray-100 dark:placeholder-gray-500 font-medium custom-scrollbar scrollbar-rounded-clip transition-[height] duration-200 ease-apple max-[640px]:pl-3 max-[640px]:pr-[84px]"
          />
          {isSkillMenuOpen && filteredSkills.length > 0 && (
            <div className="motion-popover absolute bottom-[64px] left-2 z-30 max-h-56 w-[min(360px,calc(100%-24px))] overflow-auto rounded-2xl border border-gray-200 bg-white p-2 shadow-2xl dark:border-white/10 dark:bg-[#252527] custom-scrollbar">
              {filteredSkills.map((skill) => (
                <button
                  key={skill.id}
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.addSkillReference(skill);
                    ide.setChatInput(removeSlashSkillQuery(ide.chatInput));
                    setIsSkillMenuOpen(false);
                    window.setTimeout(() => chatInputRef.current?.focus(), 0);
                  }}
                  className="motion-row block w-full rounded-xl px-3 py-2 text-left hover:bg-gray-100 dark:hover:bg-white/5"
                >
                  <div className="truncate text-sm font-semibold text-gray-800 dark:text-gray-100">{skill.name}</div>
                  <div className="mt-0.5 line-clamp-2 text-xs text-gray-500 dark:text-gray-400">{skill.description || '无描述'}</div>
                </button>
              ))}
            </div>
          )}
          <div className="composer-tool-cluster pointer-events-none absolute right-[-4px] top-1/2 z-30 flex -translate-y-1/2 items-center backdrop-blur-xl max-[640px]:right-0">
            <button
              data-testid="chat-attachment-button"
              onClick={(e) => {
                e.stopPropagation();
                attachmentInputRef.current?.click();
              }}
              className="motion-button composer-icon-button composer-attach-button pointer-events-auto flex items-center justify-center"
              title="上传文件"
            >
              <Paperclip size={16} />
            </button>
            <button
              data-testid="chat-send-button"
              onClick={(e) => {
                e.stopPropagation();
                if (ide.isChatSending) {
                  ide.stopChatGeneration?.();
                  return;
                }
                submitChatMessage();
              }}
              disabled={isSendDisabled}
              className={`motion-button composer-icon-button pointer-events-auto flex items-center justify-center ${
                ide.isChatSending
                  ? 'composer-stop-button'
                  : isSendDisabled
                  ? 'composer-send-button-disabled'
                  : 'composer-send-button'
              }`}
              title={ide.isChatSending ? '停止生成' : '发送'}
            >
              {ide.isChatSending ? (
                <Square size={13} fill="currentColor" strokeWidth={2.5} />
              ) : (
                <Send size={15} className="composer-send-icon" strokeWidth={2.5} />
              )}
            </button>
          </div>
        </div>
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

function ReferenceChip({ icon, title, subtitle, onRemove }) {
  return (
    <div className="motion-chip flex max-w-full items-center gap-2 rounded-2xl border border-blue-200/70 bg-blue-50/80 px-3 py-1.5 text-xs text-blue-700 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-200">
      <span className="shrink-0 text-blue-500">{icon}</span>
      <span className="min-w-0">
        <span className="block max-w-[180px] truncate font-semibold">{title}</span>
        <span className="block max-w-[180px] truncate text-[10px] opacity-70">{subtitle}</span>
      </span>
      <button
        onClick={(e) => {
          e.stopPropagation();
          onRemove();
        }}
        className="rounded-md p-0.5 text-blue-400 hover:bg-blue-100 hover:text-red-500 dark:hover:bg-white/10"
        title="移除引用"
      >
        <X size={12} />
      </button>
    </div>
  );
}
