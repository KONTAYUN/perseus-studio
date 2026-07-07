import React from 'react';
import { Download, History, RotateCcw, Sparkles, Trash2, X } from 'lucide-react';
import { API_BASE_URL, apiGet, workspaceFileContentUrl } from '../../../api/client';
import { getNoteTitle, isNotePath, resolveNoteAssetPath } from '../../../hooks/noteTreeUtils.js';
import { AnchoredPopover, Button, IconButton } from '../../ui';

function toDate(value) {
  const date = value ? new Date(value) : null;
  return date && !Number.isNaN(date.getTime()) ? date : null;
}

function formatDate(value) {
  const date = toDate(value);
  if (!date) return '未知日期';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(date);
}

function formatTime(value) {
  const date = toDate(value);
  if (!date) return '未知时间';
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function formatTimestamp(value) {
  const date = toDate(value);
  if (!date) return '未知时间';
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

function sourceLabel(source = '') {
  const normalized = String(source || '').toLowerCase();
  if (normalized === 'restore_version') return '恢复版本';
  if (normalized === 'manual_write') return '手动保存';
  if (normalized === 'create') return '新建';
  if (normalized.startsWith('ai_')) return 'AI 修改';
  if (normalized) return normalized.replaceAll('_', ' ');
  return '修改';
}

function baseName(path = '') {
  return String(path || '').split('/').filter(Boolean).pop() || '未命名文件';
}

function parentPath(path = '') {
  const parts = String(path || '').split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function displayName(path = '') {
  return isNotePath(path) ? getNoteTitle(path) : baseName(path);
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

function groupHistoryItems(items) {
  return items.reduce((groups, item) => {
    const label = formatDate(item.created_at);
    const group = groups.find((entry) => entry.label === label);
    if (group) {
      group.items.push(item);
    } else {
      groups.push({ label, items: [item] });
    }
    return groups;
  }, []);
}

function triggerBrowserDownload(path) {
  const anchor = document.createElement('a');
  anchor.href = `${API_BASE_URL}${path}`;
  anchor.download = '';
  anchor.rel = 'noopener';
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
}

function HistoryExportButton({ selectedItem, selectedPath, disabled }) {
  const anchorRef = React.useRef(null);
  const [isOpen, setIsOpen] = React.useState(false);
  const canExport = Boolean(selectedItem?.id && selectedPath && isNotePath(selectedPath));

  const exportVersion = (format) => {
    if (!canExport) return;
    const params = new URLSearchParams();
    params.set('version_id', selectedItem.id);
    params.set('path', selectedPath);
    params.set('format', format);
    triggerBrowserDownload(`/api/files/export?${params.toString()}`);
  };

  return (
    <>
      <Button
        ref={anchorRef}
        variant="subtle"
        size="sm"
        disabled={disabled || !canExport}
        onClick={() => setIsOpen((current) => !current)}
        title={canExport ? '导出历史记录' : '只有 HTML 笔记历史支持导出'}
      >
        <Download size={14} />
        导出
      </Button>
      <AnchoredPopover
        anchorRef={anchorRef}
        width={176}
        minWidth={176}
        maxHeight={220}
        open={isOpen && canExport}
        onClose={() => setIsOpen(false)}
        testId="history-export-menu"
        className="py-1"
      >
        {[
          ['html', 'HTML (.html)'],
          ['md', 'Markdown (.md)'],
          ['docx', 'Word (.docx)'],
        ].map(([format, label]) => (
          <button
            key={format}
            type="button"
            className="motion-row flex w-full items-center px-3 py-2 text-left text-sm text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-blue-500/20"
            onClick={(event) => {
              event.stopPropagation();
              setIsOpen(false);
              exportVersion(format);
            }}
          >
            {label}
          </button>
        ))}
      </AnchoredPopover>
    </>
  );
}

export default function RichTextHistoryPanel({
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
    }, 160);
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
    if (!window.confirm('删除这条历史记录后将无法恢复，确认继续吗？')) return;
    setDeletingId(selectedItem.id);
    const ok = await onDeleteVersion?.(selectedItem.id);
    setDeletingId('');
    if (ok) {
      setSelectedVersionId('');
      refreshHistory();
    }
  };

  const handleClearHistory = async () => {
    if (!hasHistoryEntries) return;
    if (!window.confirm('确认清空全部最近修改历史吗？')) return;
    setClearingHistory(true);
    const ok = await onClearHistory?.();
    setClearingHistory(false);
    if (ok) {
      setSelectedVersionId('');
      refreshHistory();
    }
  };

  return (
    <div
      className={`modal-backdrop rich-text-history-overlay rich-text-history-overlay-surface fixed inset-0 z-[110] flex items-center justify-center p-4 transition-opacity duration-300 ease-apple ${
        isClosing || !hasEntered ? 'opacity-0' : 'opacity-100'
      }`}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) closeWithAnimation();
      }}
    >
      <div
        className={`rich-text-history-shell rich-text-history-shell-surface ui-overlay-surface flex h-[88vh] min-h-0 w-full max-w-6xl overflow-hidden border bg-white/90 backdrop-blur-3xl transition-[opacity,transform,box-shadow,border-color,background-color] duration-500 ease-apple dark:bg-[#1C1C1E]/95 ${
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
              <div className="space-y-4">
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
                            className={`rich-text-history-list-item motion-row block w-full rounded-lg border px-3 py-2.5 text-left ${
                              selected
                                ? 'rich-text-history-list-item-active border-blue-300/70 text-gray-800 ring-1 ring-blue-300/70 dark:border-blue-500/40 dark:text-gray-100 dark:ring-blue-500/40'
                                : 'border-transparent text-gray-700 hover:border-gray-200 dark:text-gray-300 dark:hover:border-white/10'
                            }`}
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
              <HistoryExportButton
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
  );
}
