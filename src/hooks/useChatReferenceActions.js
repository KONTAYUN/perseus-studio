import { useCallback, useEffect, useRef, useState } from 'react';
import { apiDelete, apiUploadChatAttachment } from '../api/client.js';
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

  const attachmentsRef = useRef([]);
  const attachmentClientsRef = useRef(new Set());

  useEffect(() => {
    attachmentsRef.current = chatAttachments;
  }, [chatAttachments]);

  useEffect(() => () => {
    for (const item of attachmentsRef.current) revokePreview(item);
  }, []);

  const uploadAttachment = useCallback(async (clientId, file) => {
    setChatAttachments((current) => current.map((item) =>
      item.client_id === clientId ? { ...item, upload_status: 'uploading', upload_error: '' } : item
    ));
    try {
      const uploaded = await apiUploadChatAttachment(file);
      const retained = attachmentClientsRef.current.has(clientId);
      setChatAttachments((current) => current.map((item) => {
        if (item.client_id !== clientId) return item;
        return {
          ...item,
          ...uploaded,
          client_id: clientId,
          file: null,
          fingerprint: item.fingerprint,
          preview_url: item.preview_url,
          upload_status: 'uploaded',
          upload_error: '',
        };
      }));
      if (!retained) {
        apiDelete(`/api/chat-attachments/${encodeURIComponent(uploaded.id)}`).catch(() => {});
      }
    } catch (error) {
      setChatAttachments((current) => current.map((item) =>
        item.client_id === clientId
          ? { ...item, upload_status: 'failed', upload_error: error?.message || '图片上传失败' }
          : item
      ));
    }
  }, []);

  function addChatAttachments(fileList) {
    const nextFiles = filesToChatAttachments(fileList);
    if (nextFiles.length === 0) return;
    const current = attachmentsRef.current;
    const merged = mergeChatAttachments(current, nextFiles);
    const accepted = merged === current ? [] : merged.slice(current.length);
    for (const item of nextFiles) {
      if (!accepted.includes(item)) revokePreview(item);
    }
    if (accepted.length === 0) return;
    attachmentsRef.current = merged;
    for (const item of accepted) attachmentClientsRef.current.add(item.client_id);
    setChatAttachments(merged);
    for (const item of accepted) {
      if (item.kind === 'image') uploadAttachment(item.client_id, item.file);
    }
  }

  function addClipboardAttachments(clipboardData) {
    const files = filesFromClipboard(clipboardData);
    addChatAttachments(files);
    return files.length;
  }

  function removeChatAttachment(id) {
    setChatAttachments((current) => {
      const removed = current.find((item) => item.id === id || item.client_id === id);
      if (!removed) return current;
      attachmentClientsRef.current.delete(removed.client_id);
      revokePreview(removed);
      if (removed.upload_status === 'uploaded' && removed.id) {
        apiDelete(`/api/chat-attachments/${encodeURIComponent(removed.id)}`).catch(() => {});
      }
      const next = current.filter((item) => item !== removed);
      attachmentsRef.current = next;
      return next;
    });
  }

  function retryChatAttachment(id) {
    const item = attachmentsRef.current.find((candidate) => candidate.id === id || candidate.client_id === id);
    if (!item?.file && !item?.preview_url) return;
    if (item.file) uploadAttachment(item.client_id, item.file);
  }

  function clearChatAttachments() {
    for (const item of attachmentsRef.current) revokePreview(item);
    attachmentClientsRef.current.clear();
    attachmentsRef.current = [];
    setChatAttachments([]);
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
	clearChatAttachments,
    removeChatAttachment,
    retryChatAttachment,
    removeSkillReference,
    removeWorkspaceFileReference,
    setChatAttachments,
    setChatFileReferences,
    setChatSkillReferences,
  };
}

function revokePreview(item) {
  if (item?.preview_url?.startsWith('blob:')) {
    URL.revokeObjectURL(item.preview_url);
  }
}
