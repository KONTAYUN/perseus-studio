import { useRef, useState } from 'react';
import useChatState from './useChatState';
import useIdeEditorPanels from './useIdeEditorPanels';
import useFileTree from './useFileTree';
import useLayoutState from './useLayoutState';
import useSettingsData from './useSettingsData';
import useSkills from './useSkills';
import { detectFileType, filterFreshEditorToolEvents, isAnyTrashPath, isTextPath, isWorkspaceRefreshEvent, shouldReloadWorkspaceForToolEvents } from './ideUtils';
import useIdeLifecycleEffects from './useIdeLifecycleEffects';

export default function useIdeState(options = {}) {
  const isEnabled = options.enabled !== false;
  const dialog = options.dialog || {};
  const notify = options.notify || (() => {});
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isHistoryOpen, setIsHistoryOpen] = useState(false);
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
    notify,
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
    exitingFilePaths,
    exportNotes,
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
    importNotes,
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
    recentFileTreePaths,
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

  const editorState = useIdeEditorPanels({
    confirm: dialog.confirm,
    editorRef,
    layout,
    loadTree,
    notify,
    setErrorMessage,
    setSelectedItemPath,
    setSelectedItemType,
  });
  const { activeEditor, editorPanels, openFile } = editorState;

  const chat = useChatState({
    buildCurrentFileContext: editorState.buildCurrentFileContext,
    onAssistantToolEvents: refreshWorkspaceAfterAssistantTools,
    onAssistantPreviewEvent: editorState.applyAIEditPreview,
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
    notify,
    setLayoutNotice: layout.setLayoutNotice,
  });

  async function refreshWorkspaceAfterAssistantTools(toolEvents) {
    const successfulSkillEvents = (toolEvents || []).filter(
      (item) => item?.status === 'success' && ['install_skill_from_attachment', 'delete_skill'].includes(item?.tool_name)
    );
    if (successfulSkillEvents.length > 0) await skillsState.loadSkills();

    const successfulEvents = (toolEvents || []).filter((item) => item?.status === 'success' && isWorkspaceRefreshEvent(item));
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
        (event.operation === 'move' || event.operation === 'rename' || event.operation === 'organize') &&
        event.old_path &&
        event.new_path &&
        !isAnyTrashPath(event.new_path)
      ) {
        applyPathUpdate(event.old_path, event.new_path);
      } else if ((event.operation === 'delete' || event.operation === 'trash') && isAnyTrashPath(event.new_path)) {
        clearPathState(event.old_path || event.path);
      }
    }
    for (const item of Object.values(editorPanels)) {
      await item.refreshWorkspaceAfterAssistantTools(freshSuccessfulEvents);
    }
  }

  return {
    ...layout,
    ...settings,
    ...skillsState,
    ...chat,
    currentUser: options.currentUser || null,
    showToast: notify,
    activeTabPath: activeEditor.activeTabPath,
    activateTab: activeEditor.activateTab,
    addPanel: editorState.addPanel,
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
    closeDraggedPanel: editorState.closeDraggedPanel,
    closePanel: editorState.closePanel,
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
    clearFileHistory: activeEditor.clearFileHistory,
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
    exitingFilePaths,
    externalFileConflict: activeEditor.externalFileConflict,
    exportNotes,
    fileDialog,
    fileDropIndicator,
    fileDragOverId,
    fileSearchError,
    fileSearchQuery,
    fileSearchResults,
    files,
    flatFiles,
    getDefaultCreateBasePath,
    getEditorPanelState: editorState.getEditorPanelState,
    handleDownloadDragStart,
    handleFileDragStart,
    handleFileDrop,
    inlineRenameDraft,
    inlineRenamePath,
    importNotes,
    isAddMenuOpen,
    isConversationMenuOpen: chat.isConversationMenuOpen,
    isDirty: activeEditor.isDirty,
    isEditorLoading: activeEditor.isEditorLoading,
    isFileSearchLoading,
    isFileSearchOpen,
    isHistoryOpen,
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
    openFileInEditorPanel: editorState.openFileInEditorPanel,
    openRenameModal,
    openSearchResult,
    openTrashPanel,
    openTabs: activeEditor.openTabs,
    refreshWorkspaceAfterAssistantTools,
    reloadCurrentFileFromConflict: activeEditor.reloadCurrentFileFromConflict,
    removeChatAttachment: chat.removeChatAttachment,
	retryChatAttachment: chat.retryChatAttachment,
    removeSkillReference: chat.removeSkillReference,
    removeWorkspaceFileReference: chat.removeWorkspaceFileReference,
    renameConversation: chat.renameConversation,
    renamePath,
    recentFileTreePaths,
    reorderPath,
    resetFileDragState,
    restoreTrashItem,
    returnChatHome: chat.returnChatHome,
    saveCurrentFile: activeEditor.saveCurrentFile,
    deleteFileVersion: activeEditor.deleteFileVersion,
    restoreFileVersion: activeEditor.restoreFileVersion,
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
    setIsHistoryOpen,
    setIsSettingsOpen,
    submitFileDialog,
    toggleFolder,
    trashItems,
    uploadFiles,
    uploadStatus,
  };
}
