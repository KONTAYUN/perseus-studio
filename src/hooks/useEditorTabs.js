import { useRef, useState } from 'react';
import { createEditorSaveAction } from './editorSaveActions';
import { createEditorFileLoader } from './createEditorFileLoader';
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
  const [documentModes, setDocumentModes] = useState({});

  const currentFileHashRef = useRef('');
  currentFileHashRef.current = currentFileHash;
  const saveRequestRef = useRef(null);
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
    editorStatusMessage,
    externalFileConflict,
    savedFileContent,
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
    isSavingFile,
    isTextFile,
    saveFileContent,
    savedFileContent,
  });

  const loadFileIntoEditor = createEditorFileLoader({
    editorLoadRequestRef,
    currentFileContentRef,
    isDirtyRef,
    setActivePanel,
    setActiveTabPath,
    setCurrentFileContent,
    setCurrentFileHash,
    setCurrentFileNotice,
    setCurrentFilePath,
    setCurrentFileType,
    setCurrentFileVersionId,
    setEditorStatusMessage,
    setErrorMessage,
    setExternalFileConflict,
    setIsEditorLoading,
    setOpenTabs,
    setSavedFileContent,
    setSelectedItemPath,
    setSelectedItemType,
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
    currentFilePathRef.current = snapshot.path;
    activeTabPathRef.current = snapshot.path;
    currentFileContentRef.current = snapshot.content || '';
    savedFileContentRef.current = snapshot.savedContent || '';
    currentFileHashRef.current = snapshot.hash || '';
    externalFileConflictRef.current = snapshot.conflict || null;
    isDirtyRef.current = isTabDirty(snapshot);
    isTextFileRef.current = (snapshot.fileType || detectFileType(snapshot.path)) === 'text';
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

  async function saveFileContent(options) {
    return createEditorSaveAction({
      currentFilePathRef, currentFileContentRef, currentFileHashRef, savedFileContentRef,
      isTextFileRef, externalFileConflictRef, saveRequestRef,
      setIsSavingFile, setErrorMessage, setEditorStatusMessage, setSavedFileContent,
      setCurrentFileHash, setCurrentFileVersionId, setOpenTabs, setExternalFileConflict, loadTree,
    })(options);
  }

  async function prepareCurrentFileForAgent({ path, content }) {
    const matches = () => currentFilePathRef.current === path && currentFileContentRef.current === content
      && isTextFileRef.current && !externalFileConflictRef.current;
    if (!matches()) throw new Error('文件已变化或存在保存冲突，请先处理后再修复。');
    await saveFileContent({ auto: true });
    if (!matches() || savedFileContentRef.current !== content) {
      throw new Error('未能保存当前版本，已停止发送修复请求。');
    }
    return { path, hash: currentFileHashRef.current };
  }

  function closeCurrentFile() {
    editorLoadRequestRef.current += 1;
    currentFilePathRef.current = '';
    activeTabPathRef.current = '';
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
    const conflict = { ...externalFileConflict, keepLocal: true, message: '已保留本地修改；如需覆盖磁盘版本，请手动保存。' };
    externalFileConflictRef.current = conflict;
    setExternalFileConflict(conflict);
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

  const {
    applyPathUpdate,
    clearPathState,
    refreshWorkspaceAfterAssistantTools,
  } = createEditorToolRefreshActions({
    openTabSnapshot,
    closeCurrentFile,
    currentFilePathRef,
    isDirtyRef,
    loadFileIntoEditor,
    openTabsRef,
    setActiveTabPath,
    setCurrentFilePath,
    setCurrentFileType,
    setEditorStatusMessage,
    setExternalFileConflict,
    setOpenTabs,
  });

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
    documentMode: documentModes[currentFilePath] || 'rich',
    setDocumentMode: mode => setDocumentModes(current => ({ ...current, [currentFilePath]: mode })),
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
    prepareCurrentFileForAgent,
    refreshWorkspaceAfterAssistantTools,
    reloadCurrentFileFromConflict,
    reorderTab,
    saveCurrentFile,
    setCurrentFileContent,
    setEditorStatusMessage,
  };
}
