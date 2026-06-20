import React from 'react';
import { Download, History, RotateCcw, Sparkles, Trash2, X } from 'lucide-react';
import { apiGet } from '../../../api/client';
import { workspaceFileContentUrl } from '../../../api/client';
import { downloadBlob } from '../../../hooks/fileTreeDownloadUtils';
import { getNoteTitle, resolveNoteAssetPath } from '../../../hooks/noteTreeUtils.js';
import { Button, IconButton } from '../../ui';

function formatTimestamp(value) {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) return '未知时间';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function isAiSource(source = '') {
  return String(source || '').toLowerCase().startsWith('ai_');
}

function decoratePreviewHtml(notePath, html) {
  if (typeof window === 'undefined') return html;
  const template = window.document.createElement('template');
  template.innerHTML = html || '';
  template.content.querySelectorAll('img').forEach((image) => {
    const original = image.getAttribute('src') || '';
    const resolvedPath = resolveNoteAssetPath(notePath, original);
    if (resolvedPath) {
      image.setAttribute('src', workspaceFileContentUrl(resolvedPath));
    }
  });
  return template.innerHTML;
}

export default function RichTextHistoryPanel({
  currentHtml,
  filePath,
  onClearHistory,
  onClose,
  onDeleteVersion,
  onRestoreVersion,
  open = false,
}) {
  const noteTitle = React.useMemo(() => getNoteTitle(filePath) || '未命名笔记', [filePath]);
  const [items, setItems] = React.useState([]);
  const [loading, setLoading] = React.useState(false);
  const [selectedVersionId, setSelectedVersionId] = React.useState('current');
  const [preview, setPreview] = React.useState({ content: currentHtml || '', loading: false });
  const [restoringId, setRestoringId] = React.useState('');
  const [deletingId, setDeletingId] = React.useState('');
  const [clearingHistory, setClearingHistory] = React.useState(false);
  const selectedVersionIdRef = React.useRef('current');

  React.useEffect(() => {
    selectedVersionIdRef.current = selectedVersionId;
  }, [selectedVersionId]);

  const refreshHistory = React.useCallback(() => {
    if (!open || !filePath) return () => {};
    let disposed = false;
    setLoading(true);
    apiGet(`/api/files/history?path=${encodeURIComponent(filePath)}`)
      .then((result) => {
        if (disposed) return;
        const nextItems = Array.isArray(result?.items) ? result.items : [];
        setItems(nextItems);
        if (!nextItems.some((item) => item.id === selectedVersionIdRef.current)) {
          setSelectedVersionId('current');
        }
      })
      .catch((error) => {
        console.error('load note history failed', error);
        if (!disposed) setItems([]);
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });
    return () => {
      disposed = true;
    };
  }, [filePath, open]);

  React.useEffect(() => {
    if (!open || !filePath) return;
    setSelectedVersionId('current');
    setPreview({ content: currentHtml || '', loading: false });
    return refreshHistory();
  }, [currentHtml, filePath, open, refreshHistory]);

  React.useEffect(() => {
    if (!open || !filePath || !selectedVersionId) return;
    if (selectedVersionId === 'current') {
      setPreview({ content: currentHtml || '', loading: false });
      return;
    }
    let disposed = false;
    setPreview((current) => ({ ...current, loading: true }));
    apiGet(
      `/api/files/history/read?path=${encodeURIComponent(filePath)}&version_id=${encodeURIComponent(selectedVersionId)}`
    )
      .then((result) => {
        if (!disposed) {
          setPreview({ content: result?.content || '', loading: false });
        }
      })
      .catch((error) => {
        console.error('load note history preview failed', error);
        if (!disposed) setPreview({ content: '', loading: false });
      });
    return () => {
      disposed = true;
    };
  }, [currentHtml, filePath, open, selectedVersionId]);

  if (!open) return null;

  const selectedItem = items.find((item) => item.id === selectedVersionId) || items[0] || null;
  const selectedTimestamp =
    selectedVersionId === 'current'
      ? formatTimestamp(items[0]?.created_at)
      : formatTimestamp(selectedItem?.created_at);
  const canRestore = selectedVersionId !== 'current';
  const canDeleteOne = selectedVersionId !== 'current';
  const hasHistoryEntries = items.some((item) => item.id !== 'current');

  const handleDownload = async () => {
    try {
      let content = currentHtml || '';
      let versionStamp = formatTimestamp(new Date()).replace(/[/: ]/g, '-');
      if (selectedVersionId !== 'current') {
        const result = await apiGet(
          `/api/files/history/read?path=${encodeURIComponent(filePath)}&version_id=${encodeURIComponent(selectedVersionId)}`
        );
        content = result?.content || '';
        versionStamp = formatTimestamp(selectedItem?.created_at).replace(/[/: ]/g, '-');
      }
      downloadBlob(
        `${noteTitle}-${versionStamp}.html`,
        new Blob([content], { type: 'text/html;charset=utf-8' })
      );
    } catch (error) {
      console.error('download note history failed', error);
    }
  };

  const handleDeleteVersion = async () => {
    if (!canDeleteOne || !selectedItem?.id) return;
    if (!window.confirm('删除这个历史版本后将无法恢复，确认继续吗？')) return;
    setDeletingId(selectedItem.id);
    const ok = await onDeleteVersion?.(selectedItem.id);
    setDeletingId('');
    if (ok) {
      setSelectedVersionId('current');
      refreshHistory();
    }
  };

  const handleClearHistory = async () => {
    if (!hasHistoryEntries) return;
    if (!window.confirm(`确认删除“${noteTitle}”的所有历史版本吗？`)) return;
    setClearingHistory(true);
    const ok = await onClearHistory?.();
    setClearingHistory(false);
    if (ok) {
      setSelectedVersionId('current');
      refreshHistory();
    }
  };

  return (
    <div className="rich-text-history-overlay rich-text-history-overlay-surface absolute inset-0 z-[180] flex items-center justify-center p-3">
      <div className="rich-text-history-shell rich-text-history-shell-surface flex h-full min-h-0 w-full overflow-hidden">
        <div className="rich-text-history-sidebar flex min-h-0 w-[clamp(180px,24%,260px)] min-w-[160px] max-w-[260px] flex-col border-r border-gray-200/80 dark:border-white/10">
          <div className="flex items-center justify-between gap-3 border-b border-gray-200/80 px-4 py-4 dark:border-white/10">
            <div className="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
              <History size={16} />
              <span>笔记修改历史</span>
            </div>
            <IconButton size="xs" title="关闭历史面板" onClick={onClose}>
              <X size={15} />
            </IconButton>
          </div>
          <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-2 py-2">
            {loading ? (
              <div className="px-3 py-8 text-sm text-gray-500 dark:text-gray-400">正在加载历史...</div>
            ) : (
              <div className="space-y-1.5">
                {items.map((item) => {
                  const selected = selectedVersionId === item.id;
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => setSelectedVersionId(item.id)}
                      className={`rich-text-history-list-item motion-row block w-full rounded-2xl border px-4 py-3 text-left ${
                        selected
                          ? 'rich-text-history-list-item-active border-blue-400/35 text-white'
                          : 'border-transparent text-gray-700 hover:border-gray-200 dark:text-gray-300 dark:hover:border-white/10'
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        {isAiSource(item.source) && (
                          <span className="rich-text-history-ai-badge inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em]">
                            <Sparkles size={10} />
                            AI
                          </span>
                        )}
                        {item.is_current && (
                          <span className="text-[10px] font-semibold uppercase tracking-[0.12em] text-blue-500 dark:text-blue-300">
                            当前
                          </span>
                        )}
                      </div>
                      <div className="mt-2 text-sm font-semibold">{formatTimestamp(item.created_at)}</div>
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-white dark:bg-[#17181b]">
          <div className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200/80 px-5 py-4 dark:border-white/10">
            <div className="min-w-0">
              <div className="truncate text-xl font-semibold text-gray-900 dark:text-white">{noteTitle}</div>
              <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                <span>{selectedTimestamp}</span>
                {selectedItem && isAiSource(selectedItem.source) && (
                  <span className="rich-text-history-ai-badge inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em]">
                    <Sparkles size={10} />
                    AI
                  </span>
                )}
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="subtle" size="sm" onClick={handleDownload}>
                <Download size={14} />
                下载
              </Button>
              <Button
                variant="dangerSubtle"
                size="sm"
                disabled={!hasHistoryEntries || clearingHistory}
                onClick={handleClearHistory}
              >
                <Trash2 size={14} />
                删除全部
              </Button>
              <Button
                variant="dangerSubtle"
                size="sm"
                disabled={!canDeleteOne || deletingId === selectedVersionId}
                onClick={handleDeleteVersion}
              >
                <Trash2 size={14} />
                删除此版本
              </Button>
              <Button
                variant="subtle"
                size="sm"
                disabled={!canRestore || restoringId === selectedVersionId}
                onClick={async () => {
                  setRestoringId(selectedVersionId);
                  const restored = await onRestoreVersion?.(selectedVersionId);
                  setRestoringId('');
                  if (restored) onClose?.();
                }}
              >
                <RotateCcw size={14} />
                恢复此版本
              </Button>
            </div>
          </div>
          <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-6 py-6">
            {preview.loading ? (
              <div className="text-sm text-gray-500 dark:text-gray-400">正在加载版本预览...</div>
            ) : (
              <article
                className="rich-text-history-preview rich-text-editor-content"
                dangerouslySetInnerHTML={{
                  __html: decoratePreviewHtml(filePath, preview.content || ''),
                }}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
