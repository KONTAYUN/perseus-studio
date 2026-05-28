import React from 'react';
import TextareaAutosize from 'react-textarea-autosize';
import { getSlashSkillQuery, removeSlashSkillQuery } from '../chatUtils';
import ComposerActionDock from './ComposerActionDock';
import SkillMenu from './SkillMenu';

const CHAT_INPUT_MAX_ROWS = 10;

export default function ComposerInput({
  ide,
  attachmentInputRef,
  inputRef,
  isSkillMenuOpen,
  panelId,
  setIsSkillMenuOpen,
  submitChatMessage,
  isSendDisabled,
}) {
  const enabledSkills = ide.skills.filter((skill) => skill.enabled);
  const skillQuery = getSlashSkillQuery(ide.chatInput);
  const filteredSkills = enabledSkills.filter((skill) => {
    if (skillQuery === null) return false;
    const query = skillQuery.toLowerCase();
    if (!query) return true;
    return `${skill.name} ${skill.description || ''}`.toLowerCase().includes(query);
  });

  const closeSkillMenu = React.useCallback(() => {
    setIsSkillMenuOpen(false);
  }, [setIsSkillMenuOpen]);

  const handleInputChange = React.useCallback((e) => {
    const nextValue = e.target.value;
    ide.setChatInput(nextValue);
    setIsSkillMenuOpen(getSlashSkillQuery(nextValue) !== null);
  }, [ide, setIsSkillMenuOpen]);

  const handleInputKeyDown = React.useCallback((e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      if (ide.isChatSending && (e.ctrlKey || e.metaKey)) {
        ide.steerChatInput?.();
        closeSkillMenu();
        return;
      }
      submitChatMessage();
      closeSkillMenu();
    }
  }, [closeSkillMenu, ide, submitChatMessage]);

  const handleInputPaste = React.useCallback((e) => {
    const pastedFiles = ide.addClipboardAttachments(e.clipboardData);
    if (pastedFiles > 0) {
      e.preventDefault();
    }
  }, [ide]);

  const handleSkillChoose = React.useCallback((skill, inputValue) => {
    ide.addSkillReference(skill);
    ide.setChatInput(removeSlashSkillQuery(inputValue));
    closeSkillMenu();
    window.setTimeout(() => inputRef.current?.focus(), 0);
  }, [closeSkillMenu, ide, inputRef]);

  return (
    <div className="composer-input-grid">
      <div className="composer-textarea-wrap">
        <TextareaAutosize
          ref={inputRef}
          data-testid="chat-input"
          value={ide.chatInput}
          onChange={handleInputChange}
          onFocus={() => ide.setActivePanel('chat-input')}
          minRows={1}
          maxRows={CHAT_INPUT_MAX_ROWS}
          placeholder={ide.isChatSending ? '继续输入，消息会排队发送...' : '发送指令或拖拽文件...'}
          onKeyDown={handleInputKeyDown}
          onPaste={handleInputPaste}
          className="composer-textarea custom-scrollbar"
        />
      </div>
      <SkillMenu
        isOpen={isSkillMenuOpen}
        skills={filteredSkills}
        inputValue={ide.chatInput}
        onChoose={handleSkillChoose}
      />
      <ComposerActionDock
        attachmentInputRef={attachmentInputRef}
        isChatSending={ide.isChatSending}
        isSendDisabled={isSendDisabled}
        hasDraft={!!ide.chatInput.trim()}
        onSend={submitChatMessage}
        onStop={ide.stopChatGeneration}
        ide={ide}
        panelId={panelId}
      />
    </div>
  );
}
