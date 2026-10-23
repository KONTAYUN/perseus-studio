import {
  buildZipDownloadBlob,
  downloadBlob,
  downloadText,
  handleDownloadDragStart,
  readFileForDownload,
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
      const file = await readFileForDownload(path);
      downloadBlob(path.split('/').pop() || 'download', file);
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
      const { blob } = await buildZipDownloadBlob(items, treeData);
      downloadBlob(fileName, blob);
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
