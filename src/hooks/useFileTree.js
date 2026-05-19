import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { apiGet } from '../api/client';
import {
  decorateTree,
  flattenTree,
  persistOpenFolders,
  readStoredOpenFolders,
} from './ideUtils';
import { createFileTreeDownloadActions } from './fileTreeDownloadActions';
import { createFileTreeMutationActions } from './fileTreeMutationActions';
import { useFileDialogState } from './useFileDialogState';
import { useFileDragState } from './useFileDragState';
import { useFileSearchState } from './useFileSearchState';
import { useFileSelectionState } from './useFileSelectionState';
import { useFileTreeTrashActions } from './useFileTreeTrashActions';

export default function useFileTree({
  applyEditorPathUpdate,
  clearEditorPathState,
  confirm,
  getCurrentFile,
  openFile,
  setEditorStatusMessage,
  setIsAddMenuOpen,
  userId = '',
} = {}) {
  const [treeData, setTreeData] = useState([]);
  const [openFolders, setOpenFolders] = useState(() => readStoredOpenFolders(userId) || new Set());
  const [isTreeLoading, setIsTreeLoading] = useState(true);
  const [errorMessage, setErrorMessage] = useState('');
  const [inlineRenamePath, setInlineRenamePath] = useState('');
  const [inlineRenameDraft, setInlineRenameDraft] = useState('');
  const [uploadStatus, setUploadStatus] = useState('');

  const selectedItemPathRef = useRef('');
  const {
    cancelFileDialog,
    fileDialog,
    openConfirmDialog,
    submitFileDialog,
  } = useFileDialogState();
  const files = decorateTree(treeData, openFolders);
  const flatFiles = flattenTree(files);
  const {
    clearSelection,
    lastSelectedPath,
    selectItem,
    selectedItemPath,
    selectedItemType,
    selectedItems,
    selectedPaths,
    selectTreeItem,
    setLastSelectedPath,
    setSelectedItemPath,
    setSelectedItemType,
    setSelectedPaths,
  } = useFileSelectionState(flatFiles);
  const {
    draggingFileId,
    draggingFilePath,
    fileDragOverId,
    fileDropIndicator,
    handleFileDragStart,
    handleFileDrop,
    resetFileDragState,
    setFileDragOverId,
    setFileDropIndicator,
  } = useFileDragState();
  const {
    closeTrashPanel,
    deleteTrashItem,
    emptyTrash,
    isTrashLoading,
    isTrashPanelOpen,
    loadTrashItems,
    openTrashPanel,
    restoreTrashItem,
    setIsTrashPanelOpen,
    trashItems,
  } = useFileTreeTrashActions({
    confirm,
    loadTree,
    openConfirmDialog,
    setEditorStatusMessage,
    setErrorMessage,
    setIsAddMenuOpen,
    setUploadStatus,
  });
  const {
    closeFileSearch,
    fileSearchError,
    fileSearchQuery,
    fileSearchResults,
    isFileSearchLoading,
    isFileSearchOpen,
    openFileSearch,
    openSearchResult,
    setFileSearchQuery,
  } = useFileSearchState({
    openFile,
    setIsAddMenuOpen,
    setIsTrashPanelOpen,
    setLastSelectedPath,
    setOpenFolders,
    setSelectedItemPath,
    setSelectedItemType,
    setSelectedPaths,
  });
  const {
    downloadCurrentFile,
    downloadFile,
    downloadFromTreeAction,
    downloadItem,
    downloadSelectedItems,
    handleDownloadDragStart,
  } = createFileTreeDownloadActions({
    getCurrentFile,
    selectedItems,
    selectedPaths,
    setErrorMessage,
    treeData,
  });
  const {
    applyPathUpdate,
    cancelInlineRename,
    clearPathState,
    commitInlineRename,
    createFile,
    createFolder,
    deletePath,
    deleteSelectedItems,
    getDefaultCreateBasePath,
    movePath,
    openCreateFileModal,
    openCreateFolderModal,
    openDeleteModal,
    openRenameModal,
    renamePath,
    reorderPath,
    uploadFiles,
  } = createFileTreeMutationActions({
    applyEditorPathUpdate,
    clearEditorPathState,
    confirm,
    flatFiles,
    inlineRenameDraft,
    inlineRenamePath,
    loadTree,
    openConfirmDialog,
    openFile,
    openFolders,
    resetFileDragState,
    selectItem,
    selectedItemPath,
    selectedItemPathRef,
    selectedItemType,
    selectedItems,
    selectedPaths,
    setEditorStatusMessage,
    setErrorMessage,
    setInlineRenameDraft,
    setInlineRenamePath,
    setIsAddMenuOpen,
    setLastSelectedPath,
    setOpenFolders,
    setSelectedItemPath,
    setSelectedItemType,
    setSelectedPaths,
    setUploadStatus,
  });

  useEffect(() => {
    selectedItemPathRef.current = selectedItemPath;
  }, [selectedItemPath]);

  useLayoutEffect(() => {
    setOpenFolders(readStoredOpenFolders(userId) || new Set());
  }, [userId]);

  useEffect(() => {
    if (!userId) return;
    persistOpenFolders(userId, openFolders);
  }, [openFolders, userId]);

  async function loadTree() {
    setIsTreeLoading(true);
    setErrorMessage('');
    try {
      const data = await apiGet('/api/files/tree');
      setTreeData(data.items || []);
      await loadTrashItems({ silent: true });
    } catch (error) {
      console.error('loadTree failed', error);
      setErrorMessage(error.message);
    } finally {
      setIsTreeLoading(false);
    }
  }

  const toggleFolder = (folderPath) => {
    setOpenFolders((current) => {
      const next = new Set(current);
      if (next.has(folderPath)) next.delete(folderPath);
      else next.add(folderPath);
      return next;
    });
  };

  return {
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
    isTreeLoading,
    isTrashLoading,
    isTrashPanelOpen,
    loadTrashItems,
    loadTree,
    movePath,
    openCreateFileModal,
    openCreateFolderModal,
    openDeleteModal,
    openRenameModal,
    openFileSearch,
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
  };
}
