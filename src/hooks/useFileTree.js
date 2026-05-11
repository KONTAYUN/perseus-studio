import { useEffect, useRef, useState } from 'react';
import { apiGet, apiPost, apiUploadFile } from '../api/client';
import {
  buildUploadPath,
  collectDownloadFiles,
  createZipBlob,
  decorateTree,
  flattenTree,
  getParentPath,
  isTextPath,
  pathMatches,
  seedOpenFolders,
} from './ideUtils';
import {
  defaultCreateBasePath,
  nextAvailablePathInTree,
  reducePathSelectionOnDelete,
  reducePathSelectionOnRename,
  removeOpenFolderPath,
  topLevelSelectedItems,
} from './fileTreeUtils';

export default function useFileTree({
  applyEditorPathUpdate,
  clearEditorPathState,
  confirm,
  getCurrentFile,
  openFile,
  setEditorStatusMessage,
  setIsAddMenuOpen,
} = {}) {
  const [treeData, setTreeData] = useState([]);
  const [openFolders, setOpenFolders] = useState(new Set());
  const [draggingFileId, setDraggingFileId] = useState(null);
  const [draggingFilePath, setDraggingFilePath] = useState('');
  const [fileDragOverId, setFileDragOverId] = useState(null);
  const [fileDropIndicator, setFileDropIndicator] = useState(null);
  const [isTreeLoading, setIsTreeLoading] = useState(true);
  const [errorMessage, setErrorMessage] = useState('');
  const [selectedItemPath, setSelectedItemPath] = useState('');
  const [selectedItemType, setSelectedItemType] = useState('root');
  const [selectedPaths, setSelectedPaths] = useState(new Set());
  const [lastSelectedPath, setLastSelectedPath] = useState('');
  const [inlineRenamePath, setInlineRenamePath] = useState('');
  const [inlineRenameDraft, setInlineRenameDraft] = useState('');
  const [fileDialog, setFileDialog] = useState(null);
  const [uploadStatus, setUploadStatus] = useState('');
  const [trashItems, setTrashItems] = useState([]);
  const [isTrashPanelOpen, setIsTrashPanelOpen] = useState(false);
  const [isTrashLoading, setIsTrashLoading] = useState(false);

  const fileDialogResolver = useRef(null);
  const selectedItemPathRef = useRef('');
  const hasLoadedTreeRef = useRef(false);

  const files = decorateTree(treeData, openFolders);
  const flatFiles = flattenTree(files);
  const selectedItems = flatFiles.filter((item) => selectedPaths.has(item.path));

  useEffect(() => {
    selectedItemPathRef.current = selectedItemPath;
  }, [selectedItemPath]);

  useEffect(() => {
    const clearDragState = () => resetFileDragState();
    window.addEventListener('dragend', clearDragState);
    window.addEventListener('drop', clearDragState);
    return () => {
      window.removeEventListener('dragend', clearDragState);
      window.removeEventListener('drop', clearDragState);
    };
  }, []);

  async function loadTree() {
    setIsTreeLoading(true);
    setErrorMessage('');
    try {
      const data = await apiGet('/api/files/tree');
      setTreeData(data.items || []);
      await loadTrashItems({ silent: true });
      if (!hasLoadedTreeRef.current) {
        setOpenFolders(seedOpenFolders(data.items || []));
        hasLoadedTreeRef.current = true;
      }
    } catch (error) {
      console.error('loadTree failed', error);
      setErrorMessage(error.message);
    } finally {
      setIsTreeLoading(false);
    }
  }

  async function loadTrashItems({ silent = false } = {}) {
    if (!silent) setIsTrashLoading(true);
    try {
      const data = await apiGet('/api/files/trash');
      const items = data.items || [];
      setTrashItems(items);
      return items;
    } catch (error) {
      console.error('loadTrashItems failed', error);
      if (!silent) setErrorMessage(error.message);
      return [];
    } finally {
      if (!silent) setIsTrashLoading(false);
    }
  }

  async function openTrashPanel() {
    setIsAddMenuOpen?.(false);
    setIsTrashPanelOpen(true);
    await loadTrashItems();
  }

  function closeTrashPanel() {
    setIsTrashPanelOpen(false);
  }

  const handleFileDragStart = (e, fileId, filePath = '') => {
    e.stopPropagation();
    setDraggingFileId(fileId);
    setDraggingFilePath(filePath);
    e.dataTransfer.setData('file_id', fileId);
  };

  const handleFileDrop = (e) => {
    e.preventDefault();
    e.stopPropagation();
    resetFileDragState();
  };

  const resetFileDragState = () => {
    setFileDragOverId(null);
    setFileDropIndicator(null);
    setDraggingFileId(null);
    setDraggingFilePath('');
  };

  const toggleFolder = (folderPath) => {
    setOpenFolders((current) => {
      const next = new Set(current);
      if (next.has(folderPath)) next.delete(folderPath);
      else next.add(folderPath);
      return next;
    });
  };

  async function createFolder(path, options = {}) {
    setErrorMessage('');
    setEditorStatusMessage?.('');
    try {
      await apiPost('/api/files/mkdir', { path });
      setOpenFolders((current) => {
        const next = new Set(current);
        const parent = getParentPath(path);
        if (parent) next.add(parent);
        next.add(path);
        return next;
      });
      await loadTree();
      if (options.renameAfterCreate) {
        startInlineRename(path, 'folder');
      }
    } catch (error) {
      console.error('createFolder failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function createFile(path, content = '', options = {}) {
    setErrorMessage('');
    setEditorStatusMessage?.('');
    try {
      await apiPost('/api/files/create', { path, content });
      const parent = getParentPath(path);
      if (parent) {
        setOpenFolders((current) => {
          const next = new Set(current);
          next.add(parent);
          return next;
        });
      }
      await loadTree();
      if (options.renameAfterCreate) {
        startInlineRename(path, 'file');
      } else if (options.openAfterCreate !== false) {
        await openFile?.(path);
      }
    } catch (error) {
      console.error('createFile failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function renamePath(oldPath, newPath) {
    setErrorMessage('');
    setEditorStatusMessage?.('');
    try {
      await apiPost('/api/files/rename', { old_path: oldPath, new_path: newPath });
      applyPathUpdate(oldPath, newPath);
      applyEditorPathUpdate?.(oldPath, newPath);
      await loadTree();
    } catch (error) {
      console.error('renamePath failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function commitInlineRename() {
    const oldPath = inlineRenamePath;
    if (!oldPath) return;
    const oldName = oldPath.split('/').pop() || oldPath;
    const parent = getParentPath(oldPath);
    const nextName = inlineRenameDraft.trim();

    setInlineRenamePath('');
    setInlineRenameDraft('');

    if (!nextName || nextName === oldName) return;
    if (nextName.includes('/') || nextName.includes('\\')) {
      setErrorMessage('请输入文件名，不要输入路径');
      return;
    }

    const nextPath = parent ? `${parent}/${nextName}` : nextName;
    await renamePath(oldPath, nextPath);
  }

  function cancelInlineRename() {
    setInlineRenamePath('');
    setInlineRenameDraft('');
  }

  function startInlineRename(path, type = 'file') {
    const name = path.split('/').pop() || path;
    setInlineRenamePath(path);
    setInlineRenameDraft(name);
    selectItem(path, type);
  }

  async function deletePath(path) {
    setErrorMessage('');
    setEditorStatusMessage?.('');
    try {
      await apiPost('/api/files/delete', { path });
      clearPathState(path);
      clearEditorPathState?.(path);
      await loadTree();
    } catch (error) {
      console.error('deletePath failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function emptyTrash() {
    setErrorMessage('');
    setEditorStatusMessage?.('');
    const latestItems = await loadTrashItems();
    if (latestItems.length === 0) {
      setUploadStatus('回收站不存在或为空');
      window.setTimeout(() => setUploadStatus(''), 1800);
      return;
    }

    const confirmed = await openConfirmDialog({
      title: '清空回收站',
      message: '确认永久删除回收站中的全部内容？workspace 其他文件不会被删除。',
      confirmText: '清空',
      danger: true,
    });
    if (!confirmed) return;

    try {
      const result = await apiPost('/api/files/empty-trash', {});
      setUploadStatus(result.deleted_count > 0 ? `已清空回收站：${result.deleted_count} 项` : result.message || '回收站为空');
      await loadTree();
      await loadTrashItems({ silent: true });
    } catch (error) {
      console.error('emptyTrash failed', error);
      setErrorMessage(error.message);
      throw error;
    } finally {
      window.setTimeout(() => setUploadStatus(''), 2200);
    }
  }

  async function restoreTrashItem(item, overwrite = false) {
    if (!item?.path) return;
    setErrorMessage('');
    try {
      const result = await apiPost('/api/files/trash/restore', {
        path: item.path,
        overwrite,
      });
      await loadTree();
      await loadTrashItems({ silent: true });
      setUploadStatus(`已移出回收站：${result.new_path || item.original_path || item.name}`);
      window.setTimeout(() => setUploadStatus(''), 2200);
    } catch (error) {
      if (!overwrite && (error.status === 409 || error.payload?.conflict || error.message.includes('原路径已存在'))) {
        const ok = await confirm?.({
          title: '覆盖原路径',
          message: `${item.original_path || item.name} 已存在，是否覆盖？`,
          confirmLabel: '覆盖',
          tone: 'red',
        });
        if (ok) {
          await restoreTrashItem(item, true);
        }
        return;
      }
      console.error('restoreTrashItem failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function deleteTrashItem(item) {
    if (!item?.path) return;
    const ok = await confirm?.({
      title: '彻底删除',
      message: `确认彻底删除 ${item.name || item.path}？此操作无法撤销。`,
      confirmLabel: '彻底删除',
      tone: 'red',
    });
    if (!ok) return;
    setErrorMessage('');
    try {
      await apiPost('/api/files/trash/delete', { path: item.path });
      await loadTrashItems({ silent: true });
      setUploadStatus(`已彻底删除：${item.name || item.path}`);
      window.setTimeout(() => setUploadStatus(''), 2200);
    } catch (error) {
      console.error('deleteTrashItem failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function uploadFiles(fileList, targetPath = '') {
    const filesToUpload = Array.from(fileList || []);
    if (filesToUpload.length === 0) return;

    setErrorMessage('');
    setUploadStatus(`正在上传 ${filesToUpload.length} 个文件...`);

    try {
      for (const file of filesToUpload) {
        const relativePath = buildUploadPath(file.name, targetPath);
        try {
          await apiUploadFile(relativePath, file, false);
        } catch (error) {
          if (error.message.toLowerCase().includes('already exists')) {
            const shouldOverwrite = await openConfirmDialog({
              title: '覆盖文件',
              message: `${relativePath} 已存在，是否覆盖？`,
              confirmText: '覆盖',
            });
            if (!shouldOverwrite) {
              continue;
            }
            await apiUploadFile(relativePath, file, true);
          } else {
            console.error('uploadFiles failed', error);
            setErrorMessage(error.message);
            throw error;
          }
        }
      }

      if (targetPath) {
        setOpenFolders((current) => {
          const next = new Set(current);
          next.add(targetPath);
          return next;
        });
      }
      setUploadStatus(`上传完成：${filesToUpload.length} 个文件`);
      await loadTree();
    } finally {
      resetFileDragState();
      window.setTimeout(() => setUploadStatus(''), 1800);
    }
  }

  async function openCreateFileModal(basePath = getDefaultCreateBasePath()) {
    const path = nextAvailablePath(basePath, '新建 文本文档.txt');
    await createFile(path, '', { renameAfterCreate: true, openAfterCreate: false });
    setIsAddMenuOpen?.(false);
  }

  async function openCreateFolderModal(basePath = getDefaultCreateBasePath()) {
    const path = nextAvailablePath(basePath, '新建文件夹');
    await createFolder(path, { renameAfterCreate: true });
    setIsAddMenuOpen?.(false);
  }

  async function openRenameModal(path, type) {
    selectItem(path, type);
    startInlineRename(path, type);
  }

  async function openDeleteModal(path, type) {
    selectItem(path, type);
    const confirmed = await openConfirmDialog({
      title: '删除',
      message: `确认删除 ${path}？`,
      confirmText: '删除',
      danger: true,
    });
    if (!confirmed) return;
    await deletePath(path);
  }

  async function movePath(sourcePath, targetFolderPath = '') {
    if (!sourcePath) return;
    if (targetFolderPath && pathMatches(targetFolderPath, sourcePath)) {
      showTransientUploadStatus('不能移动到自身文件夹内部');
      return;
    }

    const baseName = sourcePath.split('/').pop();
    const nextPath = targetFolderPath ? `${targetFolderPath}/${baseName}` : baseName;

    if (nextPath === sourcePath) return;

    try {
      await renamePath(sourcePath, nextPath);
      if (targetFolderPath) {
        setOpenFolders((current) => {
          const next = new Set(current);
          next.add(targetFolderPath);
          return next;
        });
      }
      setSelectedItemPath(nextPath);
      setSelectedPaths(new Set([nextPath]));
      setLastSelectedPath(nextPath);
      setUploadStatus(`已移动到 ${targetFolderPath || 'workspace 根目录'}：${nextPath}`);
      window.setTimeout(() => setUploadStatus(''), 2200);
    } catch (error) {
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function reorderPath({ sourcePath, targetParentPath = '', previousPath = '', nextPath = '' } = {}) {
    if (!sourcePath) return;
    if (targetParentPath && pathMatches(targetParentPath, sourcePath)) {
      showTransientUploadStatus('不能移动到自身文件夹内部');
      return;
    }

    setErrorMessage('');
    setEditorStatusMessage?.('');
    try {
      const result = await apiPost('/api/files/reorder', {
        path: sourcePath,
        target_parent_path: targetParentPath,
        previous_path: previousPath,
        next_path: nextPath,
      });
      const movedPath = result?.new_path || result?.path || sourcePath;
      if (result?.old_path && result?.new_path && result.old_path !== result.new_path) {
        applyPathUpdate(result.old_path, result.new_path);
        applyEditorPathUpdate?.(result.old_path, result.new_path);
      }
      if (targetParentPath) {
        setOpenFolders((current) => {
          const next = new Set(current);
          next.add(targetParentPath);
          return next;
        });
      }
      setSelectedItemPath(movedPath);
      setSelectedPaths(new Set([movedPath]));
      setLastSelectedPath(movedPath);
      await loadTree();
      showTransientUploadStatus('已调整顺序');
    } catch (error) {
      console.error('reorderPath failed', error);
      if (error.message === '不能移动到自身文件夹内部') {
        showTransientUploadStatus(error.message);
        return;
      }
      setErrorMessage(error.message);
      throw error;
    }
  }

  function selectItem(path, type) {
    setSelectedItemPath(path);
    setSelectedItemType(type);
    setSelectedPaths(new Set([path]));
    setLastSelectedPath(path);
  }

  function clearSelection() {
    setSelectedItemPath('');
    setSelectedItemType('root');
    setSelectedPaths(new Set());
    setLastSelectedPath('');
  }

  function selectTreeItem(path, type, event) {
    setSelectedItemPath(path);
    setSelectedItemType(type);

    if (event?.shiftKey && lastSelectedPath) {
      const paths = flatFiles.map((item) => item.path);
      const start = paths.indexOf(lastSelectedPath);
      const end = paths.indexOf(path);
      if (start !== -1 && end !== -1) {
        const [from, to] = start < end ? [start, end] : [end, start];
        setSelectedPaths(new Set(paths.slice(from, to + 1)));
        return;
      }
    }

    if (event?.metaKey || event?.ctrlKey) {
      setSelectedPaths((current) => {
        const next = new Set(current);
        if (next.has(path)) next.delete(path);
        else next.add(path);
        return next;
      });
      setLastSelectedPath(path);
      return;
    }

    setSelectedPaths(new Set([path]));
    setLastSelectedPath(path);
  }

  function getDefaultCreateBasePath() {
    return defaultCreateBasePath(selectedItemPath, selectedItemType);
  }

  function nextAvailablePath(basePath, defaultName) {
    return nextAvailablePathInTree(flatFiles, basePath, defaultName);
  }

  function downloadCurrentFile() {
    const current = getCurrentFile?.() || {};
    if (!current.path || !current.isTextFile) return;
    downloadText(current.path, current.content || '');
  }

  async function downloadFile(path) {
    if (!isTextPath(path)) {
      setErrorMessage('第一版仅支持文本文件下载');
      return;
    }
    try {
      const file = await apiGet(`/api/files/read?path=${encodeURIComponent(path)}`);
      downloadText(file.path, file.content);
    } catch (error) {
      console.error('downloadFile failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function downloadItem(path, type) {
    if (type === 'folder') {
      await downloadPathsAsZip([{ path, type }], `${path.split('/').pop() || 'workspace'}.zip`);
      return;
    }
    await downloadFile(path);
  }

  async function downloadFromTreeAction(path, type) {
    if (selectedPaths.size > 1 && selectedPaths.has(path)) {
      await downloadSelectedItems();
      return;
    }
    await downloadItem(path, type);
  }

  async function downloadSelectedItems() {
    if (selectedItems.length === 0) return;

    if (selectedItems.length === 1 && selectedItems[0].type === 'file') {
      await downloadFile(selectedItems[0].path);
      return;
    }

    await downloadPathsAsZip(selectedItems, 'workspace-selection.zip');
  }

  async function deleteSelectedItems() {
    if (selectedItems.length === 0) return;
    const itemsToDelete = topLevelSelectedItems(selectedItems);
    const count = itemsToDelete.length;
    const ok = await confirm?.({
      title: '移入回收站',
      message: `确定要将选中的 ${count} 项移入回收站吗？`,
      confirmLabel: '移入回收站',
      tone: 'red',
    });
    if (!ok) return;
    setErrorMessage('');
    try {
      for (const item of itemsToDelete) {
        await apiPost('/api/files/delete', { path: item.path });
        clearPathState(item.path);
        clearEditorPathState?.(item.path);
      }
      setSelectedPaths(new Set());
      setSelectedItemPath('');
      setSelectedItemType('root');
      await loadTree();
      setUploadStatus(`已移入回收站：${count} 项`);
      window.setTimeout(() => setUploadStatus(''), 2200);
    } catch (error) {
      console.error('deleteSelectedItems failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  async function downloadPathsAsZip(items, fileName) {
    setErrorMessage('');
    const downloadFiles = collectDownloadFiles(items, treeData);
    const textFiles = downloadFiles.filter((file) => isTextPath(file.path));
    const skippedCount = downloadFiles.length - textFiles.length;

    if (textFiles.length === 0) {
      setErrorMessage('没有可下载的文本文件');
      return;
    }

    try {
      const entries = [];
      for (const item of textFiles) {
        const file = await apiGet(`/api/files/read?path=${encodeURIComponent(item.path)}`);
        entries.push({ path: file.path, content: file.content });
      }
      downloadBlob(fileName, createZipBlob(entries));
      if (skippedCount > 0) {
        setErrorMessage(`已跳过 ${skippedCount} 个暂不支持的二进制/图片文件`);
      }
    } catch (error) {
      console.error('downloadPathsAsZip failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  function submitFileDialog(value) {
    if (fileDialogResolver.current) {
      fileDialogResolver.current(value);
      fileDialogResolver.current = null;
    }
    setFileDialog(null);
  }

  function cancelFileDialog() {
    if (fileDialogResolver.current) {
      fileDialogResolver.current(fileDialog?.kind === 'confirm' ? false : '');
      fileDialogResolver.current = null;
    }
    setFileDialog(null);
  }

  function openInputDialog(config) {
    return new Promise((resolve) => {
      fileDialogResolver.current = resolve;
      setFileDialog({ kind: 'input', ...config });
    });
  }

  function openConfirmDialog(config) {
    return new Promise((resolve) => {
      fileDialogResolver.current = resolve;
      setFileDialog({ kind: 'confirm', cancelText: '取消', ...config });
    });
  }

  function downloadText(path, content) {
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    downloadBlob(path.split('/').pop() || 'download.txt', blob);
  }

  function downloadBlob(fileName, blob) {
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = fileName;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  function showTransientUploadStatus(message, timeout = 2200) {
    setUploadStatus(message);
    window.setTimeout(() => setUploadStatus(''), timeout);
  }

  function handleDownloadDragStart(e, path, content) {
    if (!path || !isTextPath(path)) return;
    const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const fileName = path.split('/').pop() || 'download.txt';
    e.dataTransfer.effectAllowed = 'copy';
    e.dataTransfer.setData('DownloadURL', `text/plain:${fileName}:${url}`);
    e.dataTransfer.setData('text/plain', content);
    window.setTimeout(() => URL.revokeObjectURL(url), 30000);
  }

  function applyPathUpdate(oldPath, newPath) {
    if (!oldPath || !newPath) return;

    const next = reducePathSelectionOnRename(selectedItemPathRef.current, selectedPaths, openFolders, oldPath, newPath);
    setSelectedItemPath(next.selectedItemPath);
    setSelectedPaths(next.selectedPaths);
    setOpenFolders(next.openFolders);
  }

  function clearPathState(path) {
    if (!path) return;
    const next = reducePathSelectionOnDelete(selectedItemPathRef.current, selectedItemType, selectedPaths, path);
    setSelectedItemPath(next.selectedItemPath);
    setSelectedItemType(next.selectedItemType);
    setSelectedPaths(next.selectedPaths);
    setOpenFolders((current) => removeOpenFolderPath(current, path));
  }

  return {
    applyPathUpdate,
    cancelFileDialog,
    cancelInlineRename,
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
    files,
    flatFiles,
    getDefaultCreateBasePath,
    handleDownloadDragStart,
    handleFileDragStart,
    handleFileDrop,
    inlineRenameDraft,
    inlineRenamePath,
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
