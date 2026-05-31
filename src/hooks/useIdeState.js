import { useRef, useState } from 'react';
import useChatState from './useChatState';
import useEditorTabs from './useEditorTabs';
import useFileTree from './useFileTree';
import useLayoutState from './useLayoutState';
import useSettingsData from './useSettingsData';
import useSkills from './useSkills';
import { detectFileType, filterFreshEditorToolEvents, isAnyTrashPath, isTextPath, shouldReloadWorkspaceForToolEvents } from './ideUtils';
import { buildEditorContextFromEntries } from './ideContextUtils';
import useIdeLifecycleEffects from './useIdeLifecycleEffects';

export default function useIdeState(options = {}) {
  const isEnabled = options.enabled !== false;
  const dialog = options.dialog || {};
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const layout = useLayoutState({ userId: options.currentUser?.id || options.currentUser?.user_id || '' });
  const [isAddMenuOpen, setIsAddMenuOpen] = useState(false);
  const settings = useSettingsData({ enabled: isEnabled });
  const skillsState = useSkills({ enabled: isEnabled, setSettingsError: settings.setSettingsError });
  const editorRef = useRef(null);
  const processedEditorToolEventKeysRef = useRef(new Set());

  const fileTree = useFileTree({
    applyEditorPathUpdate: (...args) => editorRef.current?.applyPathUpdate?.(...args),
    clearEditorPathState: (...args) => editorRef.current?.clearPathState?.(...args),
    getCurrentFile: () => ({
      content: editorRef.current?.currentFileContent || '',
      isTextFile: !!editorRef.current?.isTextFile,
      path: editorRef.current?.currentFilePath || '',
    }),
    openFile: (...args) => editorRef.current?.openFile?.(...args),
    confirm: dialog.confirm,
    setEditorStatusMessage: (...args) => editorRef.current?.setEditorStatusMessage?.(...args),
    setIsAddMenuOpen,
    userId: options.currentUser?.id || options.currentUser?.user_id || '',
  });

  const {
    applyPathUpdate,
    cancelFileDialog,
    cancelInlineRename,
    closeFileSearch,
    clearPathState,
    clearSelection,
    closeTrashPanel,
    commitInlineRename,
    createFile,
    createFolder,
    deletePath,
    deleteSelectedItems,
    deleteTrashItem,
    downloadCurrentFile,
    downloadFile,
    downloadFromTreeAction,
    downloadItem,
    downloadSelectedItems,
    draggingFileId,
    draggingFilePath,
    emptyTrash,
    errorMessage,
    fileDialog,
    fileDropIndicator,
    fileDragOverId,
    fileSearchError,
    fileSearchQuery,
    fileSearchResults,
    files,
    flatFiles,
    getDefaultCreateBasePath,
    handleDownloadDragStart,
    handleFileDragStart,
    handleFileDrop,
    inlineRenameDraft,
    inlineRenamePath,
    isFileSearchLoading,
    isFileSearchOpen,
    isTrashLoading,
    isTrashPanelOpen,
    isTreeLoading,
    loadTrashItems,
    loadTree,
    movePath,
    openCreateFileModal,
    openCreateFolderModal,
    openDeleteModal,
    openFileSearch,
    openRenameModal,
    openSearchResult,
    openTrashPanel,
    renamePath,
    reorderPath,
    resetFileDragState,
    restoreTrashItem,
    selectItem,
    selectTreeItem,
    selectedItems,
    selectedPaths,
    setErrorMessage,
    setFileDropIndicator,
    setFileDragOverId,
    setFileSearchQuery,
    setInlineRenameDraft,
    setSelectedItemPath,
    setSelectedItemType,
    submitFileDialog,
    toggleFolder,
    trashItems,
    uploadFiles,
    uploadStatus,
  } = fileTree;

  const makeEditor = (panelId) =>
    useEditorTabs({
      loadTree,
      setActivePanel: (id) => {
        layout.setActivePanel(id === 'editor' ? panelId : id);
        layout.setActiveEditorPanelId(panelId);
      },
      setErrorMessage,
      setSelectedItemPath,
      setSelectedItemType,
      confirm: dialog.confirm,
    });

  const editor1 = makeEditor('editor_1');
  const editor2 = makeEditor('editor_2');
  const editor3 = makeEditor('editor_3');
  const editor4 = makeEditor('editor_4');
  const editor5 = makeEditor('editor_5');
  const editorPanels = {
    editor_1: editor1,
    editor_2: editor2,
    editor_3: editor3,
    editor_4: editor4,
    editor_5: editor5,
  };
  const activeEditor = editorPanels[layout.activeEditorPanelId] || editor1;

  function visibleEditorEntries() {
    return layout.panels
      .filter((panel) => panel.type === 'editor')
      .map((panel) => ({ panel, editor: editorPanels[panel.id] }))
      .filter((item) => item.editor);
  }

  function preferredEditorPanelId() {
    if (layout.activeEditorPanelId && layout.panels.some((panel) => panel.id === layout.activeEditorPanelId && panel.type === 'editor')) {
      return layout.activeEditorPanelId;
    }
    return layout.panels.find((panel) => panel.type === 'editor')?.id || '';
  }

  function ensureEditorPanel() {
    const existing = preferredEditorPanelId();
    if (existing) return existing;
    return layout.addPanel('editor') || 'editor_1';
  }

  async function openFile(path) {
    const existing = visibleEditorEntries().find(({ editor }) => editor.openTabs.some((tab) => tab.path === path));
    if (existing) {
      layout.setActiveEditorPanelId(existing.panel.id);
      layout.setActivePanel(existing.panel.id);
      await existing.editor.activateTab(path);
      return;
    }
    const panelId = ensureEditorPanel();
    const target = editorPanels[panelId] || editor1;
    layout.setActiveEditorPanelId(panelId);
    layout.setActivePanel(panelId);
    await target.openFile(path);
  }

  async function openFileInEditorPanel(panelId, path, options = {}) {
    if (!path) return;
    const target = editorPanels[panelId] || editor1;
    let movedSnapshot = null;
    if (options.sourcePanelId && options.sourcePanelId !== panelId) {
      const source = editorPanels[options.sourcePanelId];
      movedSnapshot = source?.getTabSnapshot(path) || null;
      if (source && !(await source.detachTab(path))) return;
    }
    layout.setActiveEditorPanelId(panelId);
    layout.setActivePanel(panelId);
    if (movedSnapshot) {
      target.openTabSnapshot(movedSnapshot);
      return;
    }
    await target.openFile(path);
  }

  editorRef.current = {
    applyPathUpdate: (...args) => Object.values(editorPanels).forEach((item) => item.applyPathUpdate(...args)),
    clearPathState: (...args) => Object.values(editorPanels).forEach((item) => item.clearPathState(...args)),
    get currentFileContent() {
      return (editorPanels[preferredEditorPanelId()] || editor1).currentFileContent;
    },
    get currentFilePath() {
      return (editorPanels[preferredEditorPanelId()] || editor1).currentFilePath;
    },
    get isTextFile() {
      return (editorPanels[preferredEditorPanelId()] || editor1).isTextFile;
    },
    openFile,
    setEditorStatusMessage: (...args) => (editorPanels[preferredEditorPanelId()] || editor1).setEditorStatusMessage(...args),
  };

  const chat = useChatState({
    buildCurrentFileContext,
    onAssistantToolEvents: refreshWorkspaceAfterAssistantTools,
    onTasksChanged: options.onTasksChanged,
    selectedModel: settings.selectedModel,
    setSelectedModelId: settings.setSelectedModelId,
    confirm: dialog.confirm,
  });

  useIdeLifecycleEffects({
    isDarkMode: layout.isDarkMode,
    isEnabled,
    layoutNotice: layout.layoutNotice,
    loadConversations: chat.loadConversations,
    loadTree,
    setLayoutNotice: layout.setLayoutNotice,
  });

  function buildCurrentFileContext() {
    return buildEditorContextFromEntries(visibleEditorEntries());
  }

  async function refreshWorkspaceAfterAssistantTools(toolEvents) {
    const successfulSkillEvents = (toolEvents || []).filter(
      (item) => item?.status === 'success' && ['install_skill_from_attachment', 'delete_skill'].includes(item?.tool_name)
    );
    if (successfulSkillEvents.length > 0) await skillsState.loadSkills();

    const successfulEvents = (toolEvents || []).filter(
      (item) =>
        item?.status === 'success' &&
        [
          'edit_workspace_text',
          'replace_workspace_lines',
          'save_workspace_file',
          'write_workspace_chunk',
          'finalize_workspace_write',
          'manage_workspace_path',
          'organize_workspace_matches',
          'run_workspace_command',
        ].includes(item?.tool_name)
    );
    const freshSuccessfulEvents = filterFreshEditorToolEvents(successfulEvents, processedEditorToolEventKeysRef.current);
    if (processedEditorToolEventKeysRef.current.size > 500) {
      processedEditorToolEventKeysRef.current = new Set([...processedEditorToolEventKeysRef.current].slice(-250));
    }
    if (freshSuccessfulEvents.length === 0) return;

    if (shouldReloadWorkspaceForToolEvents(freshSuccessfulEvents)) {
      await loadTree();
    }
    for (const event of freshSuccessfulEvents) {
      if (
        event.tool_name === 'manage_workspace_path' &&
        event.old_path &&
        event.new_path &&
        !isAnyTrashPath(event.new_path)
      ) {
        applyPathUpdate(event.old_path, event.new_path);
      } else if (event.tool_name === 'manage_workspace_path' && isAnyTrashPath(event.new_path)) {
        clearPathState(event.old_path || event.path);
      }
    }
    for (const item of Object.values(editorPanels)) {
      await item.refreshWorkspaceAfterAssistantTools(freshSuccessfulEvents);
    }
  }

  function getEditorPanelState(panelId) {
    return editorPanels[panelId] || editor1;
  }

  function addPanel(type) {
    const panelId = layout.addPanel(type);
    if (type === 'editor' && panelId) layout.setActiveEditorPanelId(panelId);
  }

  async function closePanel(panelId) {
    const canRemove = await (async () => {
      const panel = layout.panels.find((item) => item.id === panelId);
      if (!panel || panel.type !== 'editor') return true;
      const target = editorPanels[panel.id];
      const hasDirtyTab = target?.isDirty || target?.openTabs?.some((tab) => (tab.content || '') !== (tab.savedContent || ''));
      if (!hasDirtyTab) return true;
      if (!dialog.confirm) return false;
      return dialog.confirm({
        title: '关闭编辑器面板',
        message: '该编辑器有未保存内容，确认关闭这个面板？',
        confirmLabel: '关闭面板',
        tone: 'red',
      });
    })();
    if (!canRemove) return false;
    return layout.removePanel(panelId, (panel) => {
      if (panel.type !== 'editor') return true;
      const target = editorPanels[panel.id];
      const hasDirtyTab = target?.isDirty || target?.openTabs?.some((tab) => (tab.content || '') !== (tab.savedContent || ''));
      if (!hasDirtyTab) return true;
      return true;
    });
  }

  async function closeDraggedPanel() {
    if (!layout.draggingPanelId) return;
    await closePanel(layout.draggingPanelId);
    layout.handlePanelDragEnd();
  }

  return {
    ...layout,
    ...settings,
    ...skillsState,
    ...chat,
    currentUser: options.currentUser || null,
    activeTabPath: activeEditor.activeTabPath,
    activateTab: activeEditor.activateTab,
    addPanel,
    addChatAttachments: chat.addChatAttachments,
    addClipboardAttachments: chat.addClipboardAttachments,
    addSkillReference: chat.addSkillReference,
    addWorkspaceFileReferences: chat.addWorkspaceFileReferences,
    cancelFileDialog,
    cancelInlineRename,
    closeFileSearch,
    chatAttachments: chat.chatAttachments,
    chatError: chat.chatError,
    chatFileReferences: chat.chatFileReferences,
    chatInput: chat.chatInput,
    chatSkillReferences: chat.chatSkillReferences,
    clearPathState,
    clearSelection,
    closeTrashPanel,
    closeCurrentFile: activeEditor.closeCurrentFile,
    closeDraggedPanel,
    closePanel,
    closeTab: activeEditor.closeTab,
    commitInlineRename,
    conversations: chat.conversations,
    conversationMessages: chat.conversationMessages,
    createFile,
    createFolder,
    currentConversationId: chat.currentConversationId,
    currentFileContent: activeEditor.currentFileContent,
    currentFileHash: activeEditor.currentFileHash,
    currentFileNotice: activeEditor.currentFileNotice,
    currentFilePath: activeEditor.currentFilePath,
    currentFileType: activeEditor.currentFileType,
    currentFileVersionId: activeEditor.currentFileVersionId,
    deleteConversation: chat.deleteConversation,
    deletePath,
    deleteSelectedItems,
    deleteTrashItem,
    detectFileType,
    downloadCurrentFile,
    downloadFile,
    downloadFromTreeAction,
    downloadItem,
    downloadSelectedItems,
    draggingFileId,
    draggingFilePath,
    editorStatusMessage: activeEditor.editorStatusMessage,
    emptyTrash,
    errorMessage,
    externalFileConflict: activeEditor.externalFileConflict,
    fileDialog,
    fileDropIndicator,
    fileDragOverId,
    fileSearchError,
    fileSearchQuery,
    fileSearchResults,
    files,
    flatFiles,
    getDefaultCreateBasePath,
    getEditorPanelState,
    handleDownloadDragStart,
    handleFileDragStart,
    handleFileDrop,
    inlineRenameDraft,
    inlineRenamePath,
    isAddMenuOpen,
    isConversationMenuOpen: chat.isConversationMenuOpen,
    isDirty: activeEditor.isDirty,
    isEditorLoading: activeEditor.isEditorLoading,
    isFileSearchLoading,
    isFileSearchOpen,
    isSavingFile: activeEditor.isSavingFile,
    isSettingsOpen,
    isTextFile: activeEditor.isTextFile,
    isTextPath,
    isTrashLoading,
    isTrashPanelOpen,
    isTreeLoading,
    keepLocalFileChanges: activeEditor.keepLocalFileChanges,
    loadConversationMessages: chat.loadConversationMessages,
    loadConversations: chat.loadConversations,
    loadTrashItems,
    loadTree,
    movePath,
    openCreateFileModal,
    openCreateFolderModal,
    openDeleteModal,
    openFile,
    openFileSearch,
    openFileInEditorPanel,
    openRenameModal,
    openSearchResult,
    openTrashPanel,
    openTabs: activeEditor.openTabs,
    refreshWorkspaceAfterAssistantTools,
    reloadCurrentFileFromConflict: activeEditor.reloadCurrentFileFromConflict,
    removeChatAttachment: chat.removeChatAttachment,
    removeSkillReference: chat.removeSkillReference,
    removeWorkspaceFileReference: chat.removeWorkspaceFileReference,
    renameConversation: chat.renameConversation,
    renamePath,
    reorderPath,
    resetFileDragState,
    restoreTrashItem,
    returnChatHome: chat.returnChatHome,
    saveCurrentFile: activeEditor.saveCurrentFile,
    selectConversation: chat.selectConversation,
    selectItem,
    selectTreeItem,
    selectedItems,
    selectedPaths,
    sendChatMessage: chat.sendChatMessage,
    compactCurrentConversation: chat.compactCurrentConversation,
    stopChatGeneration: chat.stopChatGeneration,
    setChatInput: chat.setChatInput,
    setCurrentConversationId: chat.setCurrentConversationId,
    setCurrentFileContent: activeEditor.setCurrentFileContent,
    setFileDropIndicator,
    setFileDragOverId,
    setFileSearchQuery,
    setInlineRenameDraft,
    setIsAddMenuOpen,
    setIsConversationMenuOpen: chat.setIsConversationMenuOpen,
    setIsSettingsOpen,
    submitFileDialog,
    toggleFolder,
    trashItems,
    uploadFiles,
    uploadStatus,
  };
}
