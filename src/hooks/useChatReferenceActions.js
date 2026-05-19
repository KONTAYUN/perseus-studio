import { useState } from 'react';
import {
  filesFromClipboard,
  filesToChatAttachments,
  mergeChatAttachments,
  mergeSkillReference,
  mergeWorkspaceReferences,
  normalizeSkillId,
} from './chatReferenceUtils.js';

export default function useChatReferenceActions() {
  const [chatAttachments, setChatAttachments] = useState([]);
  const [chatFileReferences, setChatFileReferences] = useState([]);
  const [chatSkillReferences, setChatSkillReferences] = useState([]);

  function addChatAttachments(fileList) {
    const nextFiles = filesToChatAttachments(fileList);
    if (nextFiles.length === 0) return;
    setChatAttachments((current) => mergeChatAttachments(current, nextFiles));
  }

  function addClipboardAttachments(clipboardData) {
    const files = filesFromClipboard(clipboardData);
    addChatAttachments(files);
    return files.length;
  }

  function removeChatAttachment(id) {
    setChatAttachments((current) => current.filter((item) => item.id !== id));
  }

  function addWorkspaceFileReferences(refs) {
    setChatFileReferences((current) => mergeWorkspaceReferences(current, refs));
  }

  function removeWorkspaceFileReference(path) {
    setChatFileReferences((current) => current.filter((item) => item.path !== path));
  }

  function addSkillReference(skill) {
    setChatSkillReferences((current) => mergeSkillReference(current, skill));
  }

  function removeSkillReference(skillId) {
    const normalized = normalizeSkillId(skillId);
    setChatSkillReferences((current) => current.filter((item) => normalizeSkillId(item.skill_id) !== normalized));
  }

  return {
    addChatAttachments,
    addClipboardAttachments,
    addSkillReference,
    addWorkspaceFileReferences,
    chatAttachments,
    chatFileReferences,
    chatSkillReferences,
    removeChatAttachment,
    removeSkillReference,
    removeWorkspaceFileReference,
    setChatAttachments,
    setChatFileReferences,
    setChatSkillReferences,
  };
}
