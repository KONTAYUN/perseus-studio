import {
  buildZipDownloadBlob,
  downloadBlob,
  downloadWorkspaceFile,
  downloadText,
  handleDownloadDragStart,
  readTextFileForDownload,
} from './fileTreeDownloadUtils';

export function createFileTreeDownloadActions({
  getCurrentFile,
  selectedItems,
  selectedPaths,
  setErrorMessage,
  treeData,
}) {
  function downloadCurrentFile() {
    const current = getCurrentFile?.() || {};
    if (!current.path || !current.isTextFile) return;
    downloadText(current.path, current.content || '');
  }

  async function downloadFile(path) {
    try {
      await downloadWorkspaceFile(path);
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

  async function downloadPathsAsZip(items, fileName) {
    setErrorMessage('');
    try {
      const { blob, skippedCount } = await buildZipDownloadBlob(items, treeData);
      downloadBlob(fileName, blob);
      if (skippedCount > 0) {
        setErrorMessage(`已跳过 ${skippedCount} 个暂不支持的二进制/图片文件`);
      }
    } catch (error) {
      console.error('downloadPathsAsZip failed', error);
      setErrorMessage(error.message);
      throw error;
    }
  }

  return {
    downloadCurrentFile,
    downloadFile,
    downloadFromTreeAction,
    downloadItem,
    downloadSelectedItems,
    handleDownloadDragStart,
  };
}
