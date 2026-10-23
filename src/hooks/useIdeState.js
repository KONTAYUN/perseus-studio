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

  const editorState = useIdeEditorPanels({
    confirm: dialog.confirm,
    editorRef,
    layout,
    loadTree,
    setErrorMessage,
    setSelectedItemPath,
    setSelectedItemType,
  });
  const { activeEditor, editorPanels, openFile } = editorState;

  const chat = useChatState({
    userId: options.currentUser?.id || options.currentUser?.user_id || '',
    buildCurrentFileContext: editorState.buildCurrentFileContext,
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
    getEditorPanelState: editorState.getEditorPanelState,
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
    openFileInEditorPanel: editorState.openFileInEditorPanel,
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
