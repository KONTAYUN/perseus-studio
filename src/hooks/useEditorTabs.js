import { useCallback, useRef, useState } from 'react';
import { apiPost } from '../api/client';
import { createEditorFileLoader } from './createEditorFileLoader';
import { createEditorHistoryActions } from './createEditorHistoryActions';
import { createEditorToolRefreshActions } from './editorToolRefreshActions';
import { detectFileType } from './ideUtils';
import {
  isTabDirty,
  nextActiveTabAfterClose,
  removeTabByPath,
  reorderTabsByPath,
} from './editorTabUtils';
import useActiveEditorTabSync from './useActiveEditorTabSync';
import useEditorTabAutoSave from './useEditorTabAutoSave';
import useEditorTabRefSync from './useEditorTabRefSync';
import {
  canApplyAIEditPreviewDelta,
  mergeAIEditPreview,
  mergePreviewTabByIdentity,
  removeOrphanedMutationPreviewTabs,
} from './aiPreviewState';
import {
  defaultMarkdownViewState,
  markdownViewStateFromTab,
} from '../components/editor/workspaceMarkdownUtils.js';
import { isNotePath } from './noteTreeUtils.js';

export default function useEditorTabs({
  confirm,
  loadTree,
  setActivePanel,
  setErrorMessage,
  setSelectedItemPath,
  setSelectedItemType,
  notify,
} = {}) {
  const [isEditorLoading, setIsEditorLoading] = useState(false);
  const [isSavingFile, setIsSavingFile] = useState(false);
  const [currentFilePath, setCurrentFilePath] = useState('');
  const [currentFileContent, setCurrentFileContent] = useState('');
  const [savedFileContent, setSavedFileContent] = useState('');
  const [currentFileHash, setCurrentFileHash] = useState('');
  const [currentFileVersionId, setCurrentFileVersionId] = useState('');
  const [currentDocumentVersion, setCurrentDocumentVersion] = useState('');
  const [currentFileType, setCurrentFileType] = useState('text');
  const [currentFileNotice, setCurrentFileNotice] = useState('');
  const [externalFileConflict, setExternalFileConflict] = useState(null);
  const [editorStatusMessage, setEditorStatusMessageState] = useState('');
  const [openTabs, setOpenTabs] = useState([]);
  const [activeTabPath, setActiveTabPath] = useState('');
  const [aiPreview, setAIEditPreviewState] = useState(null);
  const [userRevision, setUserRevision] = useState(0);
  const [savedUserRevision, setSavedUserRevision] = useState(0);
  const [markdownViewState, setMarkdownViewState] = useState(defaultMarkdownViewState());
  const isAuthoritativeSyncing =
    !!aiPreview && ['previewing', 'committing', 'reconciling'].includes(aiPreview.status);

  const currentFilePathRef = useRef('');
  const currentFileContentRef = useRef('');
  const savedFileContentRef = useRef('');
  const isTextFileRef = useRef(false);
  const externalFileConflictRef = useRef(null);
  const isDirtyRef = useRef(false);
  const openTabsRef = useRef([]);
  const activeTabPathRef = useRef('');
  const autoSaveTimerRef = useRef(null);
  const editorLoadRequestRef = useRef(0);
  const userRevisionRef = useRef(0);
  const savedUserRevisionRef = useRef(0);
  const currentDocumentVersionRef = useRef('');
  currentDocumentVersionRef.current = currentDocumentVersion;
  const setEditorStatusMessage = useCallback((message, options) => {
    setEditorStatusMessageState(message);
    if (typeof message === 'string' && message.trim()) {
      notify?.(message, options);
    }
  }, [notify]);

  function markdownStateForTab(tab = {}) {
    return markdownViewStateFromTab({
      ...tab,
      path: tab.aiPreview?.path || tab.previewSourcePath || tab.path,
    });
  }

  const isTextFile = currentFileType === 'text';
  const isDirty =
    currentFilePath !== '' && isTextFile && userRevision > savedUserRevision;

  useEditorTabRefSync({
    activeTabPath,
    activeTabPathRef,
    currentFileContent,
    currentFileContentRef,
    currentFilePath,
    currentFilePathRef,
    externalFileConflict,
    externalFileConflictRef,
    isDirty,
    isDirtyRef,
    isTextFile,
    isTextFileRef,
    openTabs,
    openTabsRef,
    savedFileContent,
    savedFileContentRef,
  });

  useActiveEditorTabSync({
    activeTabPath,
    buildTabSnapshot,
    currentFilePath,
    currentFileContent,
    currentFileHash,
    currentFileNotice,
    currentFileType,
    currentFileVersionId,
    currentDocumentVersion,
    editorStatusMessage,
    externalFileConflict,
    aiPreview,
    savedFileContent,
    userRevision,
    savedUserRevision,
    markdownViewState,
    setOpenTabs,
  });

  useEditorTabAutoSave({
    autoSaveTimerRef,
    currentFileContent,
    currentFilePath,
    externalFileConflict,
    isDirty,
    isDirtyRef,
    isEditorLoading,
    isAuthoritativeSyncing,
    isSavingFile,
    isTextFile,
    saveFileContent,
    userRevision,
    savedUserRevision,
  });

  const loadFileIntoEditor = createEditorFileLoader({
    editorLoadRequestRef,
    currentFileContentRef,
    savedFileContentRef,
    isDirtyRef,
    setActivePanel,
    setAIEditPreview: setAIEditPreviewState,
    setActiveTabPath,
    setCurrentFileContent,
    setCurrentFileHash,
    setCurrentFileNotice,
    setCurrentFilePath,
    setCurrentFileType,
    setCurrentFileVersionId,
    setCurrentDocumentVersion,
    setUserRevision: setUserRevisionState,
    setSavedUserRevision: setSavedUserRevisionState,
    setEditorStatusMessage,
    setErrorMessage,
    setExternalFileConflict,
    setIsEditorLoading,
    setOpenTabs,
    setSavedFileContent,
    setMarkdownViewState,
    setSelectedItemPath,
    setSelectedItemType,
  });

  const {
    clearFileHistory,
    deleteFileVersion,
    restoreFileVersion,
  } = createEditorHistoryActions({
    currentFilePathRef,
    loadFileIntoEditor,
    loadTree,
    setEditorStatusMessage,
    setErrorMessage,
  });

  async function openFile(path) {
    await flushAutoSave();
    const existing = openTabsRef.current.find((tab) => tab.path === path);
    if (existing) {
      activateTab(path);
      return;
    }
    await loadFileIntoEditor(path);
  }

  async function activateTab(path, options = {}) {
    const tab = openTabsRef.current.find((item) => item.path === path);
    if (!tab) return;
    if (!options.skipFlush) {
      await flushAutoSave();
    }
    editorLoadRequestRef.current += 1;
    setActiveTabPath(tab.path);
    setCurrentFilePath(tab.path);
    setCurrentFileContent(tab.content || '');
    setSavedFileContent(tab.savedContent || '');
    setUserRevisionState(tab.userRevision || 0);
    setSavedUserRevisionState(tab.savedUserRevision || 0);
    setCurrentFileHash(tab.hash || '');
    setCurrentFileVersionId(tab.versionId || '');
    setCurrentDocumentVersion(tab.documentVersion || tab.hash || '');
    setCurrentFileType(tab.fileType || detectFileType(tab.path));
    setCurrentFileNotice(tab.notice || '');
    setExternalFileConflict(tab.conflict || null);
    setAIEditPreviewState(tab.aiPreview || null);
    setMarkdownViewState(markdownStateForTab(tab));
    setEditorStatusMessageState(tab.statusMessage || '');
    setSelectedItemPath?.(tab.path);
    setSelectedItemType?.('file');
    setActivePanel?.('editor');
  }

  async function closeTab(path) {
    const tabs = openTabsRef.current;
    const tab = tabs.find((item) => item.path === path);
    if (!tab) return;
    if (isTabDirty(tab)) {
      const ok = await confirm?.({
        title: '关闭未保存文件',
        message: `${tab.path} 有未保存内容，确认关闭？`,
        confirmLabel: '关闭文件',
        tone: 'red',
      });
      if (!ok) return;
    }
    const nextTabs = removeTabByPath(tabs, path);
    setOpenTabs(nextTabs);
    if (activeTabPathRef.current !== path) return;
    const next = nextActiveTabAfterClose(tabs, path);
    if (next) {
      openTabsRef.current = nextTabs;
      await activateTab(next.path, { skipFlush: true });
      return;
    }
    closeCurrentFile();
  }

  function reorderTab(sourcePath, targetPath, position = 'before') {
    const current = openTabsRef.current;
    const currentWithActiveSnapshot = current.map((tab) =>
      tab.path === currentFilePathRef.current ? { ...tab, ...buildTabSnapshot({ path: tab.path }) } : tab
    );
    const nextTabs = reorderTabsByPath(currentWithActiveSnapshot, sourcePath, targetPath, position);
    const changed =
      nextTabs.length === current.length &&
      nextTabs.some((tab, index) => tab.path !== current[index]?.path);
    if (!changed) return false;
    openTabsRef.current = nextTabs;
    setOpenTabs(nextTabs);
    return true;
  }

  async function detachTab(path) {
    const tabs = openTabsRef.current;
    const tab = tabs.find((item) => item.path === path);
    if (!tab) return true;
    if (isTabDirty(tab)) {
      const ok = await confirm?.({
        title: '移动未保存文件',
        message: `${tab.path} 有未保存内容，确认移动到其他编辑器？`,
        confirmLabel: '移动文件',
        tone: 'red',
      });
      if (!ok) return false;
    }
    const nextTabs = removeTabByPath(tabs, path);
    openTabsRef.current = nextTabs;
    setOpenTabs(nextTabs);
    if (activeTabPathRef.current === path) {
      const next = nextActiveTabAfterClose(tabs, path);
      if (next) {
        activateTab(next.path, { skipFlush: true });
      } else {
        closeCurrentFile();
      }
    }
    return true;
  }

  function getTabSnapshot(path) {
    if (!path) return null;
    if (activeTabPathRef.current === path) return buildTabSnapshot({ path });
    const tab = openTabsRef.current.find((item) => item.path === path);
    return tab ? { ...tab } : null;
  }

  function buildTabSnapshot(overrides = {}) {
    return {
      path: currentFilePath,
      content: currentFileContent,
      savedContent: savedFileContent,
      hash: currentFileHash,
      versionId: currentFileVersionId,
      documentVersion: currentDocumentVersion,
      fileType: currentFileType,
      notice: currentFileNotice,
      conflict: externalFileConflict,
      statusMessage: editorStatusMessage,
      aiPreview,
      userRevision,
      savedUserRevision,
      ...markdownViewState,
      updatedAt: Date.now(),
      ...overrides,
    };
  }

  function openTabSnapshot(tab) {
    if (!tab?.path) return;
    const snapshot = { ...tab, updatedAt: Date.now() };
    const current = openTabsRef.current;
    const index = current.findIndex((item) => item.path === snapshot.path);
    const nextTabs = index === -1 ? [...current, snapshot] : current.map((item) => (item.path === snapshot.path ? snapshot : item));
    editorLoadRequestRef.current += 1;
    openTabsRef.current = nextTabs;
    setOpenTabs(nextTabs);
    setActiveTabPath(snapshot.path);
    setCurrentFilePath(snapshot.path);
    setCurrentFileContent(snapshot.content || '');
    setSavedFileContent(snapshot.savedContent || '');
    setUserRevisionState(snapshot.userRevision || 0);
    setSavedUserRevisionState(snapshot.savedUserRevision || 0);
    setCurrentFileHash(snapshot.hash || '');
    setCurrentFileVersionId(snapshot.versionId || '');
    setCurrentDocumentVersion(snapshot.documentVersion || snapshot.hash || '');
    setCurrentFileType(snapshot.fileType || detectFileType(snapshot.path));
    setCurrentFileNotice(snapshot.notice || '');
    setExternalFileConflict(snapshot.conflict || null);
    setAIEditPreviewState(snapshot.aiPreview || null);
    setMarkdownViewState(markdownStateForTab(snapshot));
    setEditorStatusMessageState(snapshot.statusMessage || '');
    setSelectedItemPath?.(snapshot.path);
    setSelectedItemType?.('file');
    setActivePanel?.('editor');
  }

  async function saveCurrentFile() {
    try {
      await saveFileContent({ auto: false });
    } catch {
      // saveFileContent already surfaced the error in the editor.
    }
  }

  async function flushAutoSave() {
    if (autoSaveTimerRef.current) {
      window.clearTimeout(autoSaveTimerRef.current);
      autoSaveTimerRef.current = null;
    }
    const path = currentFilePathRef.current;
    if (!path || !isTextFileRef.current || userRevisionRef.current <= savedUserRevisionRef.current || externalFileConflictRef.current) return;
    await saveFileContent({ auto: true, flush: true });
  }

  async function saveFileContent({ auto = false } = {}) {
    const path = currentFilePathRef.current;
    const content = currentFileContentRef.current;
    if (!path || !isTextFileRef.current) return;
    if (externalFileConflictRef.current?.reloadPath && path === externalFileConflictRef.current.reloadPath) {
      setErrorMessage?.('该文件已被 AI 修改，但你有未保存内容，请先选择保留本地修改或重新加载文件');
      return;
    }
    setIsSavingFile(true);
    setErrorMessage?.('');
    setEditorStatusMessageState('');
    try {
      const result = await apiPost('/api/files/write', {
        path,
        content,
        base_document_version: currentDocumentVersionRef.current,
        user_revision: userRevisionRef.current,
        save_request_id:
          globalThis.crypto?.randomUUID?.() ||
          `save-${Date.now()}-${Math.random().toString(16).slice(2)}`,
      });
      if (currentFilePathRef.current === path) {
        setSavedFileContent(content);
        setSavedUserRevisionState(userRevisionRef.current);
        setCurrentFileHash(result.hash || '');
        setCurrentFileVersionId(result.version_id || '');
        setCurrentDocumentVersion(result.document_version || result.hash || '');
        setEditorStatusMessage('');
      }
      setOpenTabs((current) =>
        current.map((tab) =>
          tab.path === path
            ? {
                ...tab,
                content,
                savedContent: content,
                hash: result.hash || '',
                versionId: result.version_id || '',
                documentVersion: result.document_version || result.hash || '',
                savedUserRevision: tab.userRevision || userRevisionRef.current,
                statusMessage: '',
              }
            : tab
        )
      );
      await loadTree?.();
    } catch (error) {
      console.error('saveCurrentFile failed', error);
      if (error?.code === 'document_version_stale') {
        setExternalFileConflict({
          path,
          reloadPath: path,
          code: error.code,
          message: error.message,
        });
      }
      setErrorMessage?.(error.message);
      setEditorStatusMessage(auto ? '保存失败' : '');
      throw error;
    } finally {
      setIsSavingFile(false);
    }
  }

  function closeCurrentFile() {
    editorLoadRequestRef.current += 1;
    setCurrentFilePath('');
    setCurrentFileContent('');
    setSavedFileContent('');
    setCurrentFileHash('');
    setCurrentFileVersionId('');
    setCurrentDocumentVersion('');
    setCurrentFileType('text');
    setCurrentFileNotice('');
    setExternalFileConflict(null);
    setAIEditPreviewState(null);
    setUserRevisionState(0);
    setSavedUserRevisionState(0);
    setMarkdownViewState(defaultMarkdownViewState());
    setEditorStatusMessage('');
    setActiveTabPath('');
  }

  function keepLocalFileChanges() {
    if (!externalFileConflict) return;
    setExternalFileConflict(null);
    setEditorStatusMessage('已保留本地修改；如需写回磁盘，请手动保存。', { tone: 'warning' });
    setErrorMessage?.('');
  }

  function updateMarkdownViewState(patch) {
    setMarkdownViewState((current) => ({
      ...current,
      ...(typeof patch === 'function' ? patch(current) : patch),
    }));
  }

  async function reloadCurrentFileFromConflict() {
    if (!externalFileConflict?.reloadPath) return;
    await loadFileIntoEditor(externalFileConflict.reloadPath, {
      preserveSelection: true,
      hash: externalFileConflict.hash || '',
      versionId: externalFileConflict.versionId || '',
      statusMessage: '已重新加载 AI 修改后的文件内容。',
    });
  }

  function applyAIEditPreview(event) {
    const path = event?.path || '';
    const previewID = event?.preview_id || '';
    if (!path || !previewID) return false;

    const currentTabs = openTabsRef.current;
    const existingPreview = currentTabs.find(
      (tab) => tab.aiPreview?.preview_id === previewID
    );
    const existing = currentTabs.find((tab) => tab.path === path);
    const currentPreview = existingPreview?.aiPreview || existing?.aiPreview;
    const incomingRevision = Number(event.revision || 0);
    const currentRevision = Number(currentPreview?.revision || 0);
    if (
      currentPreview?.preview_id === previewID &&
      ((incomingRevision > 0 && currentRevision >= incomingRevision) ||
        (event.content != null && currentPreview.content === event.content))
    ) {
      return true;
    }
    const reverted =
      event.status === 'reverted' || event.type === 'workspace_edit_preview_reverted';
    const completed =
      event.status === 'completed' || event.type === 'workspace_edit_preview_completed';

    if (completed) {
      const terminalTab = existingPreview || existing;
      if (!terminalTab?.aiPreview || terminalTab.aiPreview.preview_id !== previewID) return false;
      const committingPreview = { ...terminalTab.aiPreview, ...event, status: 'committing' };
      const nextTabs = currentTabs.map((tab) =>
        tab.path === terminalTab.path ? { ...tab, aiPreview: committingPreview } : tab
      );
      openTabsRef.current = nextTabs;
      setOpenTabs(nextTabs);
      if (currentFilePathRef.current === terminalTab.path) {
        setAIEditPreviewState(committingPreview);
        setEditorStatusMessageState('AI 修改正在提交');
      }
      return true;
    }

    if (reverted) {
      const terminalTab = existingPreview || existing;
      if (!terminalTab?.aiPreview || terminalTab.aiPreview.preview_id !== previewID) return false;
      if (terminalTab.previewOnly || terminalTab.previewSourcePath) {
        const nextTabs = currentTabs.filter((tab) => tab.path !== terminalTab.path);
        openTabsRef.current = nextTabs;
        setOpenTabs(nextTabs);
        if (currentFilePathRef.current === terminalTab.path) {
          const next = nextTabs[nextTabs.length - 1];
          if (next) void activateTab(next.path, { skipFlush: true });
          else closeCurrentFile();
        }
        return true;
      }
      const nextTabs = currentTabs.map((tab) =>
        tab.path === terminalTab.path ? { ...tab, aiPreview: null } : tab
      );
      openTabsRef.current = nextTabs;
      setOpenTabs(nextTabs);
      if (currentFilePathRef.current === path) setAIEditPreviewState(null);
      return true;
    }

    if (!canApplyAIEditPreviewDelta(currentPreview, event)) {
      return false;
    }
    const preview = mergeAIEditPreview(currentPreview, event);
    cancelPendingAutoSave();
    if (existingPreview) {
      const merged = mergePreviewTabByIdentity(currentTabs, previewID, path, preview);
      if (!merged) return false;
      const previousPath = merged.previousPath;
      const nextTabs = merged.tabs.map((tab) =>
        tab.aiPreview?.preview_id === previewID
          ? { ...tab, fileType: detectFileType(path) }
          : tab
      );
      openTabsRef.current = nextTabs;
      setOpenTabs(nextTabs);
      if (currentFilePathRef.current === previousPath) {
        currentFilePathRef.current = path;
        activeTabPathRef.current = path;
        setCurrentFilePath(path);
        setActiveTabPath(path);
        setCurrentFileType(detectFileType(path));
        setSelectedItemPath?.(path);
        setSelectedItemType?.('file');
        setAIEditPreviewState(preview);
      }
      return true;
    }
    if (existing) {
      const dirty = (existing.content || '') !== (existing.savedContent || '');
      if (dirty) {
        openTabSnapshot({
          path: `__ai_preview__/${previewID}`,
          tabId: previewID,
          content: '',
          savedContent: '',
          hash: event.base_hash || existing.hash || '',
          versionId: '',
          fileType: detectFileType(path),
          notice: '',
          conflict: null,
          statusMessage: '',
          aiPreview: preview,
          previewOnly: true,
          previewSourcePath: path,
          userRevision: existing.userRevision || 0,
          savedUserRevision: existing.savedUserRevision || 0,
          ...markdownStateForTab({ ...existing, path }),
        });
        return true;
      }
      const nextTabs = currentTabs.map((tab) =>
        tab.path === path ? { ...tab, aiPreview: preview } : tab
      );
      openTabsRef.current = nextTabs;
      setOpenTabs(nextTabs);
      if (currentFilePathRef.current === path) setAIEditPreviewState(preview);
      return true;
    }

    openTabSnapshot({
      path,
      tabId: previewID,
      content: '',
      savedContent: '',
      hash: event.base_hash || '',
      versionId: '',
      fileType: detectFileType(path),
      notice: '',
      conflict: null,
      statusMessage: '',
      aiPreview: preview,
      previewOnly: !!event.is_new,
      userRevision: 0,
      savedUserRevision: 0,
    });
    return true;
  }

  const {
    applyPathUpdate,
    clearPathState,
    refreshWorkspaceAfterAssistantTools,
  } = createEditorToolRefreshActions({
    autoSaveTimerRef,
    closeCurrentFile,
    currentFilePathRef,
    isDirtyRef,
    applyAuthoritativeFileSnapshot,
    reconcileAuthoritativePreviewTabs,
    loadFileIntoEditor,
    openTabsRef,
    setActiveTabPath,
    setCurrentFilePath,
    setCurrentFileType,
    setEditorStatusMessage,
    setExternalFileConflict,
    setOpenTabs,
  });

  function setUserRevisionState(value) {
    const next = Number(value || 0);
    userRevisionRef.current = next;
    setUserRevision(next);
  }

  function setSavedUserRevisionState(value) {
    const next = Number(value || 0);
    savedUserRevisionRef.current = next;
    setSavedUserRevision(next);
  }

  function applyUserFileContent(content) {
    if (aiPreview && ['previewing', 'committing', 'reconciling'].includes(aiPreview.status)) {
      return;
    }
    const next = typeof content === 'string' ? content : '';
    const nextRevision = userRevisionRef.current + 1;
    currentFileContentRef.current = next;
    userRevisionRef.current = nextRevision;
    setCurrentFileContent(next);
    setUserRevision(nextRevision);
  }

  function cancelPendingAutoSave() {
    if (!autoSaveTimerRef.current) return;
    window.clearTimeout(autoSaveTimerRef.current);
    autoSaveTimerRef.current = null;
  }

  function applyAuthoritativeFileSnapshot(path, file, event = {}) {
    if (!path || !file) return;
    cancelPendingAutoSave();
    const nextRevision = Math.max(userRevisionRef.current, savedUserRevisionRef.current);
    const documentVersion = event.document_version || file.document_version || file.hash || event.hash || '';
    const requiresEditorReconciliation = isNotePath(path);
    const nextTab = (tab) => {
      const reconcilingPreview = requiresEditorReconciliation && tab.aiPreview
        ? {
            ...tab.aiPreview,
            status: 'reconciling',
            authoritative_html: file.content,
            authoritative_document_version: documentVersion,
          }
        : null;
      return {
        ...tab,
        content: file.content,
        savedContent: file.content,
        hash: event.file_hash || event.hash || file.hash || '',
        versionId: event.version_id || tab.versionId || '',
        documentVersion,
        userRevision: tab.userRevision || nextRevision,
        savedUserRevision: tab.userRevision || nextRevision,
        aiPreview: reconcilingPreview,
        previewOnly: false,
        activeMutationId: event.mutation_id || '',
        statusMessage: '',
      };
    };
    const activeTab = openTabsRef.current.find((tab) => tab.path === path);
    const reconcilingPreview = requiresEditorReconciliation && activeTab?.aiPreview
      ? {
          ...activeTab.aiPreview,
          status: 'reconciling',
          authoritative_html: file.content,
          authoritative_document_version: documentVersion,
        }
      : null;
    setOpenTabs((current) => current.map((tab) => (tab.path === path ? nextTab(tab) : tab)));
    if (currentFilePathRef.current !== path) return;
    currentFileContentRef.current = file.content;
    savedFileContentRef.current = file.content;
    isDirtyRef.current = false;
    setCurrentFileContent(file.content);
    setSavedFileContent(file.content);
    setCurrentFileHash(event.file_hash || event.hash || file.hash || '');
    setCurrentFileVersionId(event.version_id || '');
    setCurrentDocumentVersion(documentVersion);
    setUserRevisionState(nextRevision);
    setSavedUserRevisionState(nextRevision);
    setAIEditPreviewState(reconcilingPreview);
    setExternalFileConflict(null);
    setEditorStatusMessageState('');
  }

  function reconcileAuthoritativePreviewTabs(event = {}) {
    const mutationID = event.mutation_id || '';
    if (!mutationID) return;
    const changedPaths = new Set(
      (event.changed_paths?.length ? event.changed_paths : [event.path]).filter(Boolean)
    );
    if (changedPaths.size === 0) return;
    const currentTabs = openTabsRef.current;
    const nextTabs = removeOrphanedMutationPreviewTabs(currentTabs, event);
    if (nextTabs.length === currentTabs.length) return;
    openTabsRef.current = nextTabs;
    setOpenTabs(nextTabs);
    if (!nextTabs.some((tab) => tab.path === currentFilePathRef.current)) {
      const survivingTarget = nextTabs.find(
        (tab) =>
          tab.aiPreview?.mutation_id === mutationID &&
          changedPaths.has(tab.aiPreview?.path || tab.path)
      );
      const fallback = survivingTarget || nextTabs[nextTabs.length - 1];
      if (fallback) void activateTab(fallback.path, { skipFlush: true });
      else closeCurrentFile();
    }
  }

  function acknowledgeAIEditReconciled(previewID) {
    if (!previewID) return;
    setOpenTabs((current) =>
      current.map((tab) =>
        tab.aiPreview?.preview_id === previewID
          ? { ...tab, aiPreview: null, statusMessage: '' }
          : tab
      )
    );
    setAIEditPreviewState((current) =>
      current?.preview_id === previewID ? null : current
    );
  }

  return {
    activeTabPath,
    activeTabPathRef,
    activateTab,
    aiPreview,
    acknowledgeAIEditReconciled,
    applyAIEditPreview,
    reconcileAuthoritativePreviewTabs,
    applyPathUpdate,
    clearPathState,
    closeCurrentFile,
    closeTab,
    detachTab,
    clearFileHistory,
    currentFileContent,
    currentFileHash,
    currentFileNotice,
    currentFilePath,
    currentFilePathRef,
    currentFileType,
    currentFileVersionId,
    currentDocumentVersion,
    editorStatusMessage,
    externalFileConflict,
    flushAutoSave,
    getTabSnapshot,
    isDirty,
    isDirtyRef,
    isEditorLoading,
    isAuthoritativeSyncing,
    isSavingFile,
    isTextFile,
    markdownViewState,
    keepLocalFileChanges,
    loadFileIntoEditor,
    openFile,
    openTabSnapshot,
    openTabs,
    refreshWorkspaceAfterAssistantTools,
    reloadCurrentFileFromConflict,
    reorderTab,
    saveCurrentFile,
    deleteFileVersion,
    restoreFileVersion,
    setCurrentFileContent: applyUserFileContent,
    setEditorStatusMessage,
    updateMarkdownViewState,
    userRevision,
    savedUserRevision,
  };
}
