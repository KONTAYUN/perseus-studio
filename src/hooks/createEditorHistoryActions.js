import { apiPost } from '../api/client';

export function createEditorHistoryActions({
  currentFilePathRef,
  loadFileIntoEditor,
  loadTree,
  setEditorStatusMessage,
  setErrorMessage,
}) {
  async function restoreFileVersion(versionId) {
    const path = currentFilePathRef.current;
    if (!path || !versionId || versionId === 'current') return false;
    setErrorMessage?.('');
    setEditorStatusMessage?.('正在恢复历史版本...');
    try {
      const result = await apiPost('/api/files/history/restore', {
        path,
        version_id: versionId,
      });
      await loadFileIntoEditor(path, {
        hash: result.hash || '',
        versionId: result.version_id || '',
        statusMessage: '已恢复到所选历史版本',
      });
      await loadTree?.();
      return true;
    } catch (error) {
      console.error('restoreFileVersion failed', error);
      setErrorMessage?.(error.message);
      setEditorStatusMessage?.('');
      return false;
    }
  }

  async function deleteFileVersion(versionId) {
    const path = currentFilePathRef.current;
    if (!path || !versionId || versionId === 'current') return false;
    setErrorMessage?.('');
    setEditorStatusMessage?.('正在删除历史版本...');
    try {
      await apiPost('/api/files/history/delete', {
        path,
        version_id: versionId,
      });
      setEditorStatusMessage?.('已删除所选历史版本');
      return true;
    } catch (error) {
      console.error('deleteFileVersion failed', error);
      setErrorMessage?.(error.message);
      setEditorStatusMessage?.('');
      return false;
    }
  }

  async function clearFileHistory() {
    const path = currentFilePathRef.current;
    if (!path) return false;
    setErrorMessage?.('');
    setEditorStatusMessage?.('正在清空历史版本...');
    try {
      await apiPost('/api/files/history/clear', { path });
      setEditorStatusMessage?.('已清空历史版本');
      return true;
    } catch (error) {
      console.error('clearFileHistory failed', error);
      setErrorMessage?.(error.message);
      setEditorStatusMessage?.('');
      return false;
    }
  }

  return {
    clearFileHistory,
    deleteFileVersion,
    restoreFileVersion,
  };
}
