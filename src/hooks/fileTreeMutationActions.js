import { apiPost, apiUploadFile } from '../api/client';
import { buildUploadPath, getParentPath, pathMatches } from './ideUtils';
import {
  defaultCreateBasePath,
  nextAvailablePathInTree,
  reducePathSelectionOnDelete,
  reducePathSelectionOnRename,
  removeOpenFolderPath,
  topLevelSelectedItems,
} from './fileTreeUtils';

export function createFileTreeMutationActions({
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
}) {
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

  async function renamePath(oldPath, newPath, options = {}) {
    setErrorMessage('');
    setEditorStatusMessage?.('');
    try {
      await apiPost('/api/files/rename', { old_path: oldPath, new_path: newPath,
        ...(options.expectedHash !== undefined ? { expected_hash: options.expectedHash } : {}) });
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
    const path = nextAvailablePath(basePath, '新建文稿.md');
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

  function getDefaultCreateBasePath() {
    return defaultCreateBasePath(selectedItemPath, selectedItemType);
  }

  function nextAvailablePath(basePath, defaultName) {
    return nextAvailablePathInTree(flatFiles, basePath, defaultName);
  }

  function showTransientUploadStatus(message, timeout = 2200) {
    setUploadStatus(message);
    window.setTimeout(() => setUploadStatus(''), timeout);
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
  };
}
