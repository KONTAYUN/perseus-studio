import { useState } from 'react';
import { apiGet, apiPost } from '../api/client';

export function useFileTreeTrashActions({
  confirm,
  loadTree,
  openConfirmDialog,
  setEditorStatusMessage,
  setErrorMessage,
  setIsAddMenuOpen,
  setUploadStatus,
}) {
  const [trashItems, setTrashItems] = useState([]);
  const [isTrashPanelOpen, setIsTrashPanelOpen] = useState(false);
  const [isTrashLoading, setIsTrashLoading] = useState(false);

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

  return {
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
  };
}
