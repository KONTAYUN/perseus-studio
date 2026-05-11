import { useEffect, useRef, useState } from 'react';
import { apiGet, apiPost } from '../api/client';
import { detectFileType, isAnyTrashPath, isTextPath, pathMatches } from './ideUtils';
import {
  buildActiveFileToolPlan,
  buildInactiveTabWriteConflict,
  filterTabsOutsidePath,
  isSuccessfulEditorToolEvent,
  isTabDirty,
  nextActiveTabAfterClose,
  pathMovesFromToolEvent,
  remapEditorPath,
  remapEditorTabs,
  removeTabByPath,
} from './editorTabUtils';

export default function useEditorTabs({
  confirm,
  loadTree,
  setActivePanel,
  setErrorMessage,
  setSelectedItemPath,
  setSelectedItemType,
} = {}) {
  const [isEditorLoading, setIsEditorLoading] = useState(false);
  const [isSavingFile, setIsSavingFile] = useState(false);
  const [currentFilePath, setCurrentFilePath] = useState('');
  const [currentFileContent, setCurrentFileContent] = useState('');
  const [savedFileContent, setSavedFileContent] = useState('');
  const [currentFileHash, setCurrentFileHash] = useState('');
  const [currentFileVersionId, setCurrentFileVersionId] = useState('');
  const [currentFileType, setCurrentFileType] = useState('text');
  const [currentFileNotice, setCurrentFileNotice] = useState('');
  const [externalFileConflict, setExternalFileConflict] = useState(null);
  const [editorStatusMessage, setEditorStatusMessage] = useState('');
  const [openTabs, setOpenTabs] = useState([]);
  const [activeTabPath, setActiveTabPath] = useState('');

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

  const isTextFile = currentFileType === 'text';
  const isDirty =
    currentFilePath !== '' && isTextFile && currentFileContent !== savedFileContent;

  useEffect(() => {
    currentFilePathRef.current = currentFilePath;
  }, [currentFilePath]);

  useEffect(() => {
    currentFileContentRef.current = currentFileContent;
  }, [currentFileContent]);

  useEffect(() => {
    savedFileContentRef.current = savedFileContent;
  }, [savedFileContent]);

  useEffect(() => {
    isTextFileRef.current = isTextFile;
  }, [isTextFile]);

  useEffect(() => {
    externalFileConflictRef.current = externalFileConflict;
  }, [externalFileConflict]);

  useEffect(() => {
    openTabsRef.current = openTabs;
  }, [openTabs]);

  useEffect(() => {
    activeTabPathRef.current = activeTabPath;
  }, [activeTabPath]);

  useEffect(() => {
    isDirtyRef.current = isDirty;
  }, [isDirty]);

  useEffect(() => {
    if (!currentFilePath || activeTabPath !== currentFilePath) return;
    const snapshot = buildTabSnapshot();
    setOpenTabs((current) =>
      current.map((tab) => (tab.path === currentFilePath ? { ...tab, ...snapshot } : tab))
    );
  }, [
    currentFilePath,
    activeTabPath,
    currentFileContent,
    savedFileContent,
    currentFileHash,
    currentFileVersionId,
    currentFileType,
    currentFileNotice,
    externalFileConflict,
    editorStatusMessage,
  ]);

  useEffect(() => {
    if (autoSaveTimerRef.current) {
      window.clearTimeout(autoSaveTimerRef.current);
      autoSaveTimerRef.current = null;
    }
    if (!currentFilePath || !isTextFile || !isDirty || externalFileConflict || isEditorLoading || isSavingFile) {
      return undefined;
    }
    autoSaveTimerRef.current = window.setTimeout(() => {
      autoSaveTimerRef.current = null;
      saveFileContent({ auto: true }).catch((error) => console.error('autoSave failed', error));
    }, 1000);
    return () => {
      if (autoSaveTimerRef.current) {
        window.clearTimeout(autoSaveTimerRef.current);
        autoSaveTimerRef.current = null;
      }
    };
  }, [currentFilePath, currentFileContent, savedFileContent, isTextFile, isDirty, externalFileConflict, isEditorLoading, isSavingFile]);

  useEffect(() => {
    const onBeforeUnload = (event) => {
      if (!isDirtyRef.current) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, []);

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
    setCurrentFileHash(tab.hash || '');
    setCurrentFileVersionId(tab.versionId || '');
    setCurrentFileType(tab.fileType || detectFileType(tab.path));
    setCurrentFileNotice(tab.notice || '');
    setExternalFileConflict(tab.conflict || null);
    setEditorStatusMessage(tab.statusMessage || '');
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
      fileType: currentFileType,
      notice: currentFileNotice,
      conflict: externalFileConflict,
      statusMessage: editorStatusMessage,
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
    setCurrentFileHash(snapshot.hash || '');
    setCurrentFileVersionId(snapshot.versionId || '');
    setCurrentFileType(snapshot.fileType || detectFileType(snapshot.path));
    setCurrentFileNotice(snapshot.notice || '');
    setExternalFileConflict(snapshot.conflict || null);
    setEditorStatusMessage(snapshot.statusMessage || '');
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
    const content = currentFileContentRef.current;
    const saved = savedFileContentRef.current;
    if (!path || !isTextFileRef.current || content === saved || externalFileConflictRef.current) return;
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
    setEditorStatusMessage('');
    try {
      const result = await apiPost('/api/files/write', {
        path,
        content,
      });
      if (currentFilePathRef.current === path) {
        setSavedFileContent(content);
        setCurrentFileHash(result.hash || '');
        setCurrentFileVersionId(result.version_id || '');
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
                statusMessage: '',
              }
            : tab
        )
      );
      await loadTree?.();
    } catch (error) {
      console.error('saveCurrentFile failed', error);
      setErrorMessage?.(error.message);
      setEditorStatusMessage(auto ? '保存失败' : '');
      throw error;
    } finally {
      setIsSavingFile(false);
    }
  }

  async function loadFileIntoEditor(path, options = {}) {
    const {
      activatePanel = true,
      preserveSelection = false,
      hash = '',
      versionId = '',
      statusMessage = '',
      clearConflict = true,
    } = options;

    const requestId = ++editorLoadRequestRef.current;
    setIsEditorLoading(true);
    setErrorMessage?.('');
    if (clearConflict) {
      setExternalFileConflict(null);
    }
    setEditorStatusMessage(statusMessage);
    if (!preserveSelection) {
      setSelectedItemPath?.(path);
      setSelectedItemType?.('file');
    }

    try {
      const fileType = detectFileType(path);
      setCurrentFilePath(path);
      setCurrentFileType(fileType);
      setCurrentFileHash(hash || '');
      setCurrentFileVersionId(versionId || '');

      if (fileType === 'image') {
        if (requestId !== editorLoadRequestRef.current) return;
        setCurrentFileContent('');
        setSavedFileContent('');
        setCurrentFileNotice('图片文件暂不支持编辑');
        setActiveTabPath(path);
        upsertOpenTab({
          path,
          content: '',
          savedContent: '',
          hash: hash || '',
          versionId: versionId || '',
          fileType,
          notice: '图片文件暂不支持编辑',
          conflict: null,
          statusMessage,
        });
        if (activatePanel) setActivePanel?.('editor');
        return;
      }

      if (fileType === 'binary') {
        if (requestId !== editorLoadRequestRef.current) return;
        setCurrentFileContent('');
        setSavedFileContent('');
        setCurrentFileNotice('该文件类型暂不支持预览或编辑');
        setActiveTabPath(path);
        upsertOpenTab({
          path,
          content: '',
          savedContent: '',
          hash: hash || '',
          versionId: versionId || '',
          fileType,
          notice: '该文件类型暂不支持预览或编辑',
          conflict: null,
          statusMessage,
        });
        if (activatePanel) setActivePanel?.('editor');
        return;
      }

      const file = await apiGet(`/api/files/read?path=${encodeURIComponent(path)}`);
      if (requestId !== editorLoadRequestRef.current) return;
      setCurrentFilePath(file.path);
      setCurrentFileNotice('');
      setCurrentFileContent(file.content);
      setSavedFileContent(file.content);
      setCurrentFileHash(hash || file.hash || '');
      setCurrentFileVersionId(versionId || '');
      setActiveTabPath(file.path);
      upsertOpenTab({
        path: file.path,
        content: file.content,
        savedContent: file.content,
        hash: hash || file.hash || '',
        versionId: versionId || '',
        fileType,
        notice: '',
        conflict: null,
        statusMessage,
      });
      if (activatePanel) setActivePanel?.('editor');
    } catch (error) {
      if (requestId !== editorLoadRequestRef.current) return;
      console.error('loadFileIntoEditor failed', error);
      setErrorMessage?.(error.message);
    } finally {
      if (requestId === editorLoadRequestRef.current) {
        setIsEditorLoading(false);
      }
    }
  }

  function upsertOpenTab(tab) {
    setOpenTabs((current) => {
      const index = current.findIndex((item) => item.path === tab.path);
      if (index === -1) return [...current, { ...tab, updatedAt: Date.now() }];
      const next = [...current];
      next[index] = { ...next[index], ...tab, updatedAt: Date.now() };
      return next;
    });
  }

  function applyPathUpdate(oldPath, newPath) {
    if (!oldPath || !newPath) return;

    if (pathMatches(currentFilePathRef.current, oldPath)) {
      const nextFilePath = remapEditorPath(currentFilePathRef.current, oldPath, newPath);
      setCurrentFilePath(nextFilePath);
      setCurrentFileType(detectFileType(nextFilePath));
    }
    setOpenTabs((current) => remapEditorTabs(current, oldPath, newPath));
    setActiveTabPath((current) => remapEditorPath(current, oldPath, newPath));
  }

  function clearPathState(path) {
    if (!path) return;
    setOpenTabs((current) => filterTabsOutsidePath(current, path));
    if (pathMatches(currentFilePathRef.current, path)) {
      closeCurrentFile();
      setEditorStatusMessage('当前文件已移入回收站');
    }
  }

  function closeCurrentFile() {
    editorLoadRequestRef.current += 1;
    setCurrentFilePath('');
    setCurrentFileContent('');
    setSavedFileContent('');
    setCurrentFileHash('');
    setCurrentFileVersionId('');
    setCurrentFileType('text');
    setCurrentFileNotice('');
    setExternalFileConflict(null);
    setEditorStatusMessage('');
    setActiveTabPath('');
  }

  function keepLocalFileChanges() {
    if (!externalFileConflict) return;
    setExternalFileConflict(null);
    setEditorStatusMessage('已保留本地修改；如需写回磁盘，请手动保存。');
    setErrorMessage?.('');
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

  async function refreshWorkspaceAfterAssistantTools(toolEvents) {
    const successfulEvents = (toolEvents || []).filter(isSuccessfulEditorToolEvent);
    if (successfulEvents.length === 0) return;

    const activePath = currentFilePathRef.current;
    const dirty = isDirtyRef.current;

    for (const event of successfulEvents) {
      const moves = pathMovesFromToolEvent(event);
      if (moves.length > 0) {
        for (const move of moves) {
          applyPathUpdate(move.oldPath, move.newPath);
        }
      } else if (event.tool_name === 'manage_workspace_path' && isAnyTrashPath(event.new_path)) {
        clearPathState(event.old_path || event.path);
      }
      await refreshMatchingOpenTabs(event);
    }

    const { activePath: plannedActivePath, reloadPlan, conflictPlan, closeNotice } = buildActiveFileToolPlan(successfulEvents, {
      activePath,
      dirty,
    });
    if (plannedActivePath && plannedActivePath !== activePath) {
      setCurrentFilePath(plannedActivePath);
      setCurrentFileType(detectFileType(plannedActivePath));
    }

    if (closeNotice) {
      closeCurrentFile();
      setEditorStatusMessage(closeNotice);
      return;
    }

    if (conflictPlan) {
      setExternalFileConflict(conflictPlan);
      setEditorStatusMessage('');
      return;
    }

    if (reloadPlan?.path) {
      await loadFileIntoEditor(reloadPlan.path, {
        preserveSelection: true,
        hash: reloadPlan.hash || '',
        versionId: reloadPlan.versionId || '',
        statusMessage: reloadPlan.statusMessage,
      });
    }
  }

  async function refreshMatchingOpenTabs(event) {
    if (!event?.tool_name) return;
    const tabs = openTabsRef.current;
    if (tabs.length === 0) return;
    if (event.tool_name === 'manage_workspace_path' || event.tool_name === 'organize_workspace_matches') {
      const moves = pathMovesFromToolEvent(event);
      if (moves.length > 0) {
        setOpenTabs((current) =>
          current.map((item) => {
            const matchedMove = moves.find((move) => pathMatches(item.path, move.oldPath));
            if (!matchedMove) return item;
            return {
              ...item,
              path: remapEditorPath(item.path, matchedMove.oldPath, matchedMove.newPath),
              fileType: detectFileType(remapEditorPath(item.path, matchedMove.oldPath, matchedMove.newPath)),
            };
          })
        );
      }
    }
    if (event.tool_name === 'save_workspace_file' && event.path) {
      const tab = tabs.find((item) => item.path === event.path && item.path !== currentFilePathRef.current);
      if (!tab) return;
      const dirty = isTabDirty(tab);
      if (dirty) {
        setOpenTabs((current) =>
          current.map((item) =>
            item.path === event.path
              ? {
                  ...item,
                  conflict: buildInactiveTabWriteConflict(event),
                }
              : item
          )
        );
        return;
      }
      if (!isTextPath(event.path)) return;
      try {
        const file = await apiGet(`/api/files/read?path=${encodeURIComponent(event.path)}`);
        setOpenTabs((current) =>
          current.map((item) =>
            item.path === event.path
              ? {
                  ...item,
                  content: file.content,
                  savedContent: file.content,
                  hash: event.hash || file.hash || '',
                  versionId: event.version_id || item.versionId || '',
                  statusMessage: '已自动刷新 AI 修改后的文件内容。',
                }
              : item
          )
        );
      } catch (error) {
        console.error('refreshMatchingOpenTabs failed', error);
      }
    }
  }

  return {
    activeTabPath,
    activeTabPathRef,
    activateTab,
    applyPathUpdate,
    clearPathState,
    closeCurrentFile,
    closeTab,
    detachTab,
    currentFileContent,
    currentFileHash,
    currentFileNotice,
    currentFilePath,
    currentFilePathRef,
    currentFileType,
    currentFileVersionId,
    editorStatusMessage,
    externalFileConflict,
    flushAutoSave,
    getTabSnapshot,
    isDirty,
    isDirtyRef,
    isEditorLoading,
    isSavingFile,
    isTextFile,
    keepLocalFileChanges,
    loadFileIntoEditor,
    openFile,
    openTabSnapshot,
    openTabs,
    refreshWorkspaceAfterAssistantTools,
    reloadCurrentFileFromConflict,
    saveCurrentFile,
    setCurrentFileContent,
    setEditorStatusMessage,
  };
}
