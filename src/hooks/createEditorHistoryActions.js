import { apiPost } from '../api/client';

export function createEditorHistoryActions({
  currentFilePathRef,
  loadFileIntoEditor,
  loadTree,
  setEditorStatusMessage,
  setErrorMessage,
}) {
  async function restoreFileVersion(versionId) {
    if (!versionId || versionId === 'current') return false;
    setErrorMessage?.('');
    setEditorStatusMessage?.('正在恢复历史记录...');
    try {
      const result = await apiPost('/api/files/history/restore', {
        version_id: versionId,
      });
      const restoredPath = result.path || currentFilePathRef.current;
      if (restoredPath) {
        await loadFileIntoEditor(restoredPath, {
          hash: result.hash || '',
          versionId: result.version_id || '',
          statusMessage: '已恢复到所选历史记录',
        });
      }
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
    if (!versionId || versionId === 'current') return false;
    setErrorMessage?.('');
    setEditorStatusMessage?.('正在删除历史记录...');
    try {
      await apiPost('/api/files/history/delete', {
        version_id: versionId,
      });
      setEditorStatusMessage?.('已删除所选历史记录');
      return true;
    } catch (error) {
      console.error('deleteFileVersion failed', error);
      setErrorMessage?.(error.message);
      setEditorStatusMessage?.('');
      return false;
    }
  }

  async function clearFileHistory() {
    setErrorMessage?.('');
    setEditorStatusMessage?.('正在清空历史记录...');
    try {
      await apiPost('/api/files/history/clear', {});
      setEditorStatusMessage?.('已清空历史记录');
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
