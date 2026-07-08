import React from 'react';
import { History, RotateCcw, Sparkles, Trash2, X } from 'lucide-react';
import { apiGet } from '../../../api/client';
import { isNotePath } from '../../../hooks/noteTreeUtils.js';
import { Button, IconButton } from '../../ui';
import RichTextHistoryExportButton from './RichTextHistoryExportButton';
import {
  decoratePreviewHtml,
  displayName,
  formatDate,
  formatTime,
  formatTimestamp,
  groupHistoryItems,
  isAiSource,
  parentPath,
  sourceLabel,
} from './richTextHistoryUtils';

const HISTORY_EXIT_DURATION_MS = 360;
const HISTORY_LIST_EXIT_DELAY_MS = 180;

export default function RichTextHistoryPanel({
  dialog,
  onClearHistory,
  onClose,
  onDeleteVersion,
  onRestoreVersion,
  open = false,
}) {
  const [hasEntered, setHasEntered] = React.useState(false);
  const [isClosing, setIsClosing] = React.useState(false);
  const [items, setItems] = React.useState([]);
  const [loading, setLoading] = React.useState(false);
  const [selectedVersionId, setSelectedVersionId] = React.useState('');
  const [preview, setPreview] = React.useState({ content: '', loading: false });
  const [restoringId, setRestoringId] = React.useState('');
  const [deletingId, setDeletingId] = React.useState('');
  const [clearingHistory, setClearingHistory] = React.useState(false);
  const selectedVersionIdRef = React.useRef('');

  React.useEffect(() => {
    if (!open) return undefined;
    setIsClosing(false);
    const frame = requestAnimationFrame(() => setHasEntered(true));
    const closeOnEscape = (event) => {
      if (event.key === 'Escape') closeWithAnimation();
    };
    document.addEventListener('keydown', closeOnEscape);
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener('keydown', closeOnEscape);
    };
  }, [open]);

  const closeWithAnimation = () => {
    setIsClosing(true);
    window.setTimeout(() => {
      setHasEntered(false);
      onClose?.();
    }, HISTORY_EXIT_DURATION_MS);
  };

  React.useEffect(() => {
    selectedVersionIdRef.current = selectedVersionId;
  }, [selectedVersionId]);

  const refreshHistory = React.useCallback(() => {
    if (!open) return () => {};
    let disposed = false;
    setLoading(true);
    apiGet('/api/files/history')
      .then((result) => {
        if (disposed) return;
        const nextItems = Array.isArray(result?.items) ? result.items : [];
        setItems(nextItems);
        const keepSelected = nextItems.some((item) => item.id === selectedVersionIdRef.current);
        setSelectedVersionId(keepSelected ? selectedVersionIdRef.current : nextItems[0]?.id || '');
      })
      .catch((error) => {
        console.error('load global history failed', error);
        if (!disposed) {
          setItems([]);
          setSelectedVersionId('');
        }
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });
    return () => {
      disposed = true;
    };
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    return refreshHistory();
  }, [open, refreshHistory]);

  React.useEffect(() => {
    if (!open || !selectedVersionId) {
      setPreview({ content: '', loading: false });
      return;
    }
    let disposed = false;
    setPreview((current) => ({ ...current, loading: true }));
    apiGet(`/api/files/history/read?version_id=${encodeURIComponent(selectedVersionId)}`)
      .then((result) => {
        if (!disposed) setPreview({ content: result?.content || '', loading: false });
      })
      .catch((error) => {
        console.error('load global history preview failed', error);
        if (!disposed) setPreview({ content: '', loading: false });
      });
    return () => {
      disposed = true;
    };
  }, [open, selectedVersionId]);

  if (!open) return null;

  const selectedItem = items.find((item) => item.id === selectedVersionId) || null;
  const groupedItems = groupHistoryItems(items);
  const hasHistoryEntries = items.length > 0;
  const selectedPath = selectedItem?.path || '';
  const selectedName = selectedItem ? displayName(selectedPath) : '最近修改';
  const selectedParentPath = parentPath(selectedPath);
  const selectedTimestamp = selectedItem ? formatTimestamp(selectedItem.created_at) : '暂无历史记录';
  const selectedIsNote = isNotePath(selectedPath);

  const handleDeleteVersion = async () => {
    if (!selectedItem?.id) return;
    const confirmed = await dialog?.confirm?.({
      title: '删除历史记录',
      message: '删除这条历史记录后将无法恢复，确认继续吗？',
      confirmLabel: '删除记录',
      tone: 'danger',
    });
    if (!confirmed) return;
    setDeletingId(selectedItem.id);
    await new Promise((resolve) => window.setTimeout(resolve, HISTORY_LIST_EXIT_DELAY_MS));
    const ok = await onDeleteVersion?.(selectedItem.id);
    setDeletingId('');
    if (ok) {
      setSelectedVersionId('');
      refreshHistory();
    }
  };

  const handleClearHistory = async () => {
    if (!hasHistoryEntries) return;
    const confirmed = await dialog?.confirm?.({
      title: '清空最近修改',
      message: '确认清空全部最近修改历史吗？',
      confirmLabel: '清空历史',
      tone: 'danger',
    });
    if (!confirmed) return;
    setClearingHistory(true);
    await new Promise((resolve) => window.setTimeout(resolve, HISTORY_LIST_EXIT_DELAY_MS));
    const ok = await onClearHistory?.();
    setClearingHistory(false);
    if (ok) {
      setSelectedVersionId('');
      refreshHistory();
    }
  };

  return (
    <div
      className={`modal-backdrop rich-text-history-overlay rich-text-history-overlay-surface motion-fade-reveal fixed inset-0 z-[110] flex items-center justify-center p-4 ${
        isClosing || !hasEntered ? 'opacity-0' : 'opacity-100'
      }`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) closeWithAnimation();
      }}
    >
      <div
        className={`rich-text-history-shell rich-text-history-shell-surface ui-overlay-surface motion-modal motion-modal-controlled flex h-[88vh] min-h-0 w-full max-w-6xl overflow-hidden border bg-white/90 backdrop-blur-3xl dark:bg-[#1C1C1E]/95 ${
          isClosing || !hasEntered ? 'translate-y-3 scale-[0.975] opacity-0' : 'translate-y-0 scale-100 opacity-100'
        }`}
      >
        <div className="rich-text-history-sidebar flex min-h-0 w-[clamp(220px,28%,340px)] min-w-[200px] max-w-[360px] flex-col border-r border-gray-200/80 dark:border-white/10">
          <div className="flex items-center justify-between gap-3 border-b border-gray-200/80 px-4 py-4 dark:border-white/10">
            <div className="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
              <History size={16} />
              <span>最近修改</span>
            </div>
            <IconButton size="xs" title="关闭历史面板" onClick={closeWithAnimation}>
              <X size={15} />
            </IconButton>
          </div>
          <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-3 py-3">
            {loading ? (
              <div className="px-3 py-8 text-sm text-gray-500 dark:text-gray-400">正在加载历史...</div>
            ) : !hasHistoryEntries ? (
              <div className="px-3 py-8 text-sm text-gray-500 dark:text-gray-400">还没有历史记录</div>
            ) : (
              <div className={`space-y-4 ${clearingHistory ? 'motion-list-exit pointer-events-none' : ''}`}>
                {groupedItems.map((group) => (
                  <section key={group.label}>
                    <div className="px-2 pb-1 text-xs font-semibold text-gray-500 dark:text-gray-400">{group.label}</div>
                    <div className="space-y-1.5">
                      {group.items.map((item) => {
                        const selected = selectedVersionId === item.id;
                        const itemPath = item.path || '';
                        const itemParentPath = parentPath(itemPath);
                        return (
                          <button
                            key={item.id}
                            type="button"
                            onClick={() => setSelectedVersionId(item.id)}
                            className={`rich-text-history-list-item motion-row motion-list-enter block w-full rounded-lg border px-3 py-2.5 text-left ${
                              selected
                                ? 'rich-text-history-list-item-active border-blue-300/70 text-gray-800 ring-1 ring-blue-300/70 dark:border-blue-500/40 dark:text-gray-100 dark:ring-blue-500/40'
                                : 'border-transparent text-gray-700 hover:border-gray-200 dark:text-gray-300 dark:hover:border-white/10'
                            } ${deletingId === item.id ? 'motion-list-exit pointer-events-none' : ''}`}
                          >
                            <div className="flex min-w-0 items-center gap-2">
                              <span className="shrink-0 text-xs tabular-nums opacity-75">{formatTime(item.created_at)}</span>
                              <span className="min-w-0 flex-1 truncate text-sm font-semibold">{displayName(itemPath)}</span>
                            </div>
                            <div className="mt-1 flex min-w-0 items-center gap-2 text-xs opacity-75">
                              <span className="truncate">{itemParentPath || '根目录'}</span>
                              <span className="shrink-0">/</span>
                              <span className="shrink-0">{sourceLabel(item.source)}</span>
                              {isAiSource(item.source) && (
                                <span className="rich-text-history-ai-badge inline-flex shrink-0 items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em]">
                                  <Sparkles size={10} />
                                  AI
                                </span>
                              )}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  </section>
                ))}
              </div>
            )}
          </div>
        </div>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-white dark:bg-[#17181b]">
          <div className="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200/80 px-5 py-4 dark:border-white/10">
            <div className="min-w-0">
              <div className="truncate text-xl font-semibold text-gray-900 dark:text-white">{selectedName}</div>
              <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
                <span>{selectedTimestamp}</span>
                {selectedParentPath && <span className="truncate">{selectedParentPath}</span>}
                {selectedItem && <span>{sourceLabel(selectedItem.source)}</span>}
                {selectedItem && isAiSource(selectedItem.source) && (
                  <span className="rich-text-history-ai-badge inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.12em]">
                    <Sparkles size={10} />
                    AI
                  </span>
                )}
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <RichTextHistoryExportButton
                selectedItem={selectedItem}
                selectedPath={selectedPath}
                disabled={!selectedItem}
              />
              <Button
                variant="dangerSubtle"
                size="sm"
                disabled={!hasHistoryEntries || clearingHistory}
                onClick={handleClearHistory}
              >
                <Trash2 size={14} />
                清空历史
              </Button>
              <Button
                variant="dangerSubtle"
                size="sm"
                disabled={!selectedItem || deletingId === selectedVersionId}
                onClick={handleDeleteVersion}
              >
                <Trash2 size={14} />
                删除记录
              </Button>
              <Button
                variant="subtle"
                size="sm"
                disabled={!selectedItem || restoringId === selectedVersionId}
                onClick={async () => {
                  if (!selectedVersionId) return;
                  setRestoringId(selectedVersionId);
                  const restored = await onRestoreVersion?.(selectedVersionId);
                  setRestoringId('');
                  if (restored) closeWithAnimation();
                }}
              >
                <RotateCcw size={14} />
                恢复文件
              </Button>
            </div>
          </div>
          <div className="custom-scrollbar min-h-0 flex-1 overflow-y-auto px-6 py-6">
            <div key={selectedVersionId || 'empty'} className="motion-content-switch min-h-full">
            {preview.loading ? (
              <div className="text-sm text-gray-500 dark:text-gray-400">正在加载历史预览...</div>
            ) : !selectedItem ? (
              <div className="text-sm text-gray-500 dark:text-gray-400">选择一条历史记录查看内容</div>
            ) : selectedIsNote ? (
              <article
                className="rich-text-history-preview rich-text-editor-content prose prose-neutral dark:prose-invert max-w-none"
                dangerouslySetInnerHTML={{
                  __html: decoratePreviewHtml(selectedPath, preview.content || ''),
                }}
              />
            ) : (
              <pre className="rich-text-history-preview whitespace-pre-wrap break-words text-sm leading-7 text-gray-800 dark:text-gray-200">
                {preview.content || ''}
              </pre>
            )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
