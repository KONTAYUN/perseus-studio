import React from 'react';
import { createPortal } from 'react-dom';
import { Download, Eraser, FileCode2, FilePlus, FolderClosed, FolderPlus, GripHorizontal, Plus, Recycle, RotateCcw, Trash2, Upload, X } from 'lucide-react';
import FileTree from './FileTree';

const headerIconButtonClass =
  'motion-button flex h-7 w-7 items-center justify-center rounded-lg text-gray-500 transition-colors hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/30 dark:text-gray-500 dark:hover:text-gray-200';

export default function SidebarPanel({ ide, panelId = 'fileTree' }) {
  const rootUploadInputRef = React.useRef(null);
  const trashButtonRef = React.useRef(null);
  const isOverFolderRow = (event) =>
    event.target.closest?.('[data-file-type="folder"]') &&
    event.currentTarget.contains(event.target.closest('[data-file-type="folder"]'));

  const uploadToRoot = (files) => {
    ide.uploadFiles(files).catch((error) => {
      console.error(error);
    });
  };

  return (
    <aside
      id="panel-content-sidebar"
      data-testid="file-tree-panel"
      className={`ui-panel-surface h-full flex flex-col bg-white/70 dark:bg-[#1C1C1E]/90 backdrop-blur-2xl overflow-hidden transition-[border-color,box-shadow,background-color,transform] duration-300 ease-[cubic-bezier(0.19,1,0.22,1)] border relative ${ide.getPanelGlowStyle(
        panelId
      )}`}
      onClick={() => ide.setActivePanel(panelId)}
      onKeyDown={(e) => {
        if (e.key === 'Delete' && ide.selectedItems.length > 0) {
          e.preventDefault();
          ide.deleteSelectedItems().catch((error) => console.error(error));
        }
      }}
      tabIndex={0}
    >
      <div className="h-12 flex-none min-w-0 px-3 border-b border-gray-100 dark:border-white/5 flex items-center justify-between gap-2 group select-none relative z-40">
        <div className="flex min-w-0 items-center space-x-2 text-gray-500 dark:text-gray-400">
          <span
            draggable
            onDragStart={(e) => ide.handlePanelDragStart(e, panelId)}
            onDragEnd={ide.handlePanelDragEnd}
            className="cursor-grab active:cursor-grabbing"
            title="拖动面板"
          >
            <GripHorizontal size={14} className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity" />
          </span>
          <span className="truncate uppercase tracking-wider text-xs font-semibold">工作区</span>
          {ide.selectedItems.length > 1 && (
            <>
              <button
                data-testid="download-selected"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.downloadSelectedItems().catch((error) => console.error(error));
                }}
                className="ml-1 flex items-center gap-1 rounded-lg bg-blue-50 px-2 py-1 text-[11px] font-semibold text-blue-600 hover:bg-blue-100 dark:bg-blue-500/10 dark:text-blue-300 dark:hover:bg-blue-500/20"
                title="下载选中项"
              >
                <Download size={12} />
                {ide.selectedItems.length}
              </button>
              <button
                data-testid="delete-selected"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.deleteSelectedItems().catch((error) => console.error(error));
                }}
                className="flex items-center gap-1 rounded-lg bg-red-50 px-2 py-1 text-[11px] font-semibold text-red-600 hover:bg-red-100 dark:bg-red-500/10 dark:text-red-300 dark:hover:bg-red-500/20"
                title="删除选中项"
              >
                <Trash2 size={12} />
              </button>
            </>
          )}
        </div>

        <div className="relative">
          <div className="flex items-center gap-1">
            <button
              ref={trashButtonRef}
              data-testid="workspace-trash-button"
              onClick={(e) => {
                e.stopPropagation();
                if (ide.isTrashPanelOpen) {
                  ide.closeTrashPanel();
                } else {
                  ide.openTrashPanel().catch((error) => console.error(error));
                }
              }}
              className={`${headerIconButtonClass} ${ide.isTrashPanelOpen ? 'text-blue-500 dark:text-blue-300' : ''}`}
              title="打开回收站"
            >
              <Recycle size={16} />
            </button>
            <button
              data-testid="workspace-add-button"
              onClick={(e) => {
                e.stopPropagation();
                ide.setIsAddMenuOpen(!ide.isAddMenuOpen);
              }}
              className={headerIconButtonClass}
              title="新建或上传"
            >
              <Plus size={16} />
            </button>
          </div>

          {ide.isAddMenuOpen && (
            <div className="absolute right-0 mt-1 w-36 bg-white dark:bg-[#2C2C2E] border border-gray-100 dark:border-white/10 shadow-xl rounded-xl py-1 z-50 animate-in fade-in zoom-in-95">
              <button
                data-testid="workspace-create-file"
                onClick={() => ide.openCreateFileModal().catch((error) => console.error(error))}
                className="w-full text-left px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-blue-50 dark:hover:bg-blue-500/20 hover:text-blue-600 flex items-center transition-colors"
              >
                <FilePlus size={14} className="mr-2 opacity-70" /> 新建文件
              </button>
              <button
                data-testid="workspace-create-folder"
                onClick={() => ide.openCreateFolderModal().catch((error) => console.error(error))}
                className="w-full text-left px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-blue-50 dark:hover:bg-blue-500/20 hover:text-blue-600 flex items-center transition-colors"
              >
                <FolderPlus size={14} className="mr-2 opacity-70" /> 新建文件夹
              </button>
              <button
                data-testid="workspace-upload-file"
                onClick={() => {
                  ide.setIsAddMenuOpen(false);
                  rootUploadInputRef.current?.click();
                }}
                className="w-full text-left px-3 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-blue-50 dark:hover:bg-blue-500/20 hover:text-blue-600 flex items-center transition-colors"
              >
                <Upload size={14} className="mr-2 opacity-70" /> 上传文本
              </button>
            </div>
          )}
          {ide.isTrashPanelOpen && <TrashPopover ide={ide} anchorRef={trashButtonRef} />}
        </div>
      </div>

      <input
        ref={rootUploadInputRef}
        data-testid="workspace-upload-input"
        type="file"
        multiple
        className="hidden"
        onChange={(e) => {
          uploadToRoot(e.target.files);
          e.target.value = '';
        }}
      />

      <div
        data-testid="workspace-drop-root"
        className={`flex-1 overflow-y-auto p-2 custom-scrollbar scrollbar-rounded-clip transition-colors relative ${
          ide.fileDragOverId === 'root' ? 'bg-blue-50/70 dark:bg-blue-900/30' : ''
        }`}
        onDragEnter={(e) => {
          if (isOverFolderRow(e)) return;
          if (e.dataTransfer?.types?.includes('Files') || ide.draggingFileId) {
            e.preventDefault();
            ide.setFileDragOverId('root');
          }
        }}
        onDragOver={(e) => {
          if (isOverFolderRow(e)) return;
          if (e.dataTransfer?.types?.includes('Files') || ide.draggingFileId) {
            e.preventDefault();
            e.dataTransfer.dropEffect = ide.draggingFileId ? 'move' : 'copy';
            ide.setFileDragOverId('root');
          }
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) {
            ide.resetFileDragState();
          }
        }}
        onDrop={(e) => {
          const sourcePath = e.dataTransfer?.getData('workspace_path');
          if (sourcePath) {
            e.preventDefault();
            ide.resetFileDragState();
            ide.movePath(sourcePath, '').catch((error) => console.error(error));
            return;
          }

          if (e.dataTransfer?.files?.length) {
            e.preventDefault();
            ide.resetFileDragState();
            uploadToRoot(e.dataTransfer.files);
          }
        }}
        onClick={(e) => {
          if (e.target.closest?.('[data-file-path]')) return;
          ide.commitInlineRename?.().catch((error) => console.error(error));
          ide.clearSelection?.();
          ide.setIsAddMenuOpen(false);
        }}
      >
        {ide.fileDragOverId === 'root' && (
          <div className="pointer-events-none absolute inset-2 z-20 flex items-center justify-center rounded-2xl border-2 border-dashed border-blue-400/80 bg-blue-500/10 text-sm font-semibold text-blue-600 shadow-inner dark:bg-blue-500/15 dark:text-blue-200">
            {ide.draggingFileId ? '松开后移动到 workspace 根目录' : '松开后上传到 workspace 根目录'}
          </div>
        )}

        <div className="relative z-10">
          {ide.uploadStatus && (
            <div className="mx-2 mb-2 rounded-xl border border-blue-200/80 bg-blue-50/80 px-3 py-2 text-[12px] text-blue-600 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-300">
              {ide.uploadStatus}
            </div>
          )}
          {ide.errorMessage && (
            <div className="mx-2 mb-2 rounded-xl border border-red-200/80 bg-red-50/80 px-3 py-2 text-[12px] text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
              {ide.errorMessage}
            </div>
          )}
          {ide.isTreeLoading ? (
            <div className="px-3 py-2 text-[12px] text-gray-400 dark:text-gray-500">加载文件树中...</div>
          ) : (
            <FileTree items={ide.files} ide={ide} />
          )}
          {!ide.isTreeLoading && <div aria-hidden="true" className="h-[45vh] min-h-[220px]" />}
        </div>
      </div>

      {ide.isAddMenuOpen && (
        <div className="fixed inset-0 z-[1]" onClick={() => ide.setIsAddMenuOpen(false)} />
      )}
      <FileActionModal ide={ide} />
    </aside>
  );
}

function TrashPopover({ ide, anchorRef }) {
  const [position, setPosition] = React.useState({
    left: 16,
    top: 64,
    width: 320,
    maxHeight: 420,
  });

  const updatePosition = React.useCallback(() => {
    const anchor = anchorRef?.current;
    if (!anchor) return;

    const margin = 12;
    const gap = 8;
    const rect = anchor.getBoundingClientRect();
    const viewportWidth = window.innerWidth || document.documentElement.clientWidth;
    const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
    const width = Math.max(180, Math.min(320, viewportWidth - margin * 2));
    const left = Math.min(Math.max(rect.right - width, margin), viewportWidth - width - margin);

    const below = viewportHeight - rect.bottom - gap - margin;
    const above = rect.top - gap - margin;
    const opensAbove = below < 240 && above > below;
    const maxHeight = Math.max(180, Math.min(420, opensAbove ? above : below));
    const top = opensAbove
      ? Math.max(margin, rect.top - gap - maxHeight)
      : Math.min(rect.bottom + gap, viewportHeight - maxHeight - margin);

    setPosition({ left, top, width, maxHeight });
  }, [anchorRef]);

  React.useLayoutEffect(() => {
    updatePosition();
    window.addEventListener('resize', updatePosition);
    window.addEventListener('scroll', updatePosition, true);
    return () => {
      window.removeEventListener('resize', updatePosition);
      window.removeEventListener('scroll', updatePosition, true);
    };
  }, [updatePosition]);

  React.useEffect(() => {
    const onKeyDown = (event) => {
      if (event.key === 'Escape') ide.closeTrashPanel();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [ide]);

  return createPortal(
    <>
      <div className="fixed inset-0 z-[420]" onClick={() => ide.closeTrashPanel()} />
      <div
        data-testid="trash-popover"
        className="motion-popover fixed z-[430] overflow-hidden rounded-2xl border border-gray-100/90 bg-white/95 shadow-2xl shadow-black/[0.12] backdrop-blur-2xl dark:border-white/10 dark:bg-[#252527]/95"
        style={{
          left: position.left,
          top: position.top,
          width: position.width,
          maxHeight: position.maxHeight,
        }}
        onClick={(e) => e.stopPropagation()}
      >
      <div className="flex items-center justify-between gap-3 border-b border-gray-100 px-3 py-2.5 dark:border-white/10">
        <div className="min-w-0">
          <div className="text-sm font-bold text-gray-800 dark:text-gray-100">回收站</div>
          <div className="mt-0.5 text-[11px] text-gray-400">
            {ide.trashItems?.length ? `${ide.trashItems.length} 项已删除` : '没有已删除项目'}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            data-testid="empty-trash-from-popover"
            disabled={ide.isTrashLoading || ide.trashItems?.length === 0}
            onClick={(e) => {
              e.stopPropagation();
              ide.closeTrashPanel();
              ide.emptyTrash().catch((error) => console.error(error));
            }}
            className="motion-button flex h-7 items-center gap-1.5 rounded-lg px-2 text-[11px] font-semibold text-gray-400 transition-colors hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:text-gray-400 dark:text-gray-500 dark:hover:text-red-300"
            title="清空回收站"
          >
            <Eraser size={13} />
            清空
          </button>
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              ide.closeTrashPanel();
            }}
            className="motion-button flex h-7 w-7 items-center justify-center rounded-lg text-gray-400 transition-colors hover:text-gray-700 dark:hover:text-gray-100"
            title="关闭"
          >
            <X size={14} />
          </button>
        </div>
      </div>
      <div
        className="overflow-y-auto p-1.5 custom-scrollbar scrollbar-rounded-clip"
        style={{ maxHeight: Math.max(132, position.maxHeight - 54) }}
      >
        {ide.isTrashLoading ? (
          <div className="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400">正在读取回收站...</div>
        ) : ide.trashItems?.length === 0 ? (
          <div className="px-3 py-6 text-center text-sm text-gray-500 dark:text-gray-400">回收站为空。</div>
        ) : (
          ide.trashItems.map((item) => (
            <div
              key={item.id || item.path}
              className="motion-row group/trash flex min-w-0 items-center gap-2 rounded-xl px-3 py-2.5 text-gray-700 hover:bg-gray-50/90 dark:text-gray-200 dark:hover:bg-white/5"
            >
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-gray-100 text-gray-500 dark:bg-white/[0.08] dark:text-gray-300">
                {item.type === 'folder' ? <FolderClosed size={14} /> : <FileCode2 size={14} />}
              </div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-semibold">{item.name || item.path}</div>
                <div className="mt-0.5 truncate text-[11px] text-gray-400">
                  原路径：{item.original_path || item.name || item.path}
                </div>
              </div>
              <div className="flex shrink-0 translate-x-1 items-center gap-1 opacity-0 transition-[opacity,transform] duration-200 ease-apple group-hover/trash:translate-x-0 group-hover/trash:opacity-100">
                <button
                  type="button"
                  data-testid={`restore-trash-${item.path}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.restoreTrashItem(item).catch((error) => console.error(error));
                  }}
                  className="motion-button flex h-7 w-7 items-center justify-center rounded-lg text-gray-400 hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-500/10 dark:hover:text-blue-200"
                  title="移出回收站"
                >
                  <RotateCcw size={13} />
                </button>
                <button
                  type="button"
                  data-testid={`delete-trash-${item.path}`}
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.deleteTrashItem(item).catch((error) => console.error(error));
                  }}
                  className="motion-button flex h-7 w-7 items-center justify-center rounded-lg text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-500/10"
                  title="彻底删除"
                >
                  <Trash2 size={13} />
                </button>
              </div>
            </div>
          ))
        )}
      </div>
      </div>
    </>,
    document.body
  );
}

function FileActionModal({ ide }) {
  const dialog = ide.fileDialog;
  const [value, setValue] = React.useState('');

  React.useEffect(() => {
    setValue(dialog?.defaultValue || '');
  }, [dialog]);

  if (!dialog) return null;

  const submit = () => {
    if (dialog.kind === 'confirm') {
      ide.submitFileDialog(true);
      return;
    }
    ide.submitFileDialog(value.trim());
  };

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/35 dark:bg-black/60 backdrop-blur-md p-4">
      <div className="w-full max-w-sm overflow-hidden rounded-2xl border border-white/40 dark:border-white/10 bg-white/95 dark:bg-[#1C1C1E]/95 shadow-2xl">
        <div className="px-5 py-4 border-b border-gray-100 dark:border-white/10">
          <h3 className="text-sm font-bold text-gray-800 dark:text-gray-100">{dialog.title}</h3>
        </div>
        <div className="px-5 py-4">
          {dialog.kind === 'confirm' ? (
            <p className="text-sm text-gray-600 dark:text-gray-300">{dialog.message}</p>
          ) : (
            <label className="block">
              <span className="block mb-2 text-xs font-semibold text-gray-500 dark:text-gray-400">
                {dialog.label}
              </span>
              <input
                data-testid="file-dialog-input"
                autoFocus
                value={value}
                onChange={(e) => setValue(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') submit();
                  if (e.key === 'Escape') ide.cancelFileDialog();
                }}
                className="w-full rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-[#2C2C2E] px-3 py-2 text-sm text-gray-800 dark:text-gray-100 outline-none focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20"
              />
            </label>
          )}
        </div>
        <div className="flex justify-end space-x-2 px-5 py-4 bg-gray-50/80 dark:bg-[#252526]/70 border-t border-gray-100 dark:border-white/10">
          <button
            data-testid="file-dialog-cancel"
            onClick={ide.cancelFileDialog}
            className="px-4 py-2 rounded-xl text-sm font-semibold text-gray-600 dark:text-gray-300 hover:bg-gray-200/80 dark:hover:bg-white/10 transition-colors"
          >
            {dialog.cancelText || '取消'}
          </button>
          <button
            data-testid="file-dialog-submit"
            onClick={submit}
            className={`px-4 py-2 rounded-xl text-sm font-semibold text-white shadow-md transition-all ${
              dialog.danger
                ? 'bg-red-500 hover:bg-red-600 shadow-red-500/20'
                : 'bg-blue-500 hover:bg-blue-600 shadow-blue-500/20'
            }`}
          >
            {dialog.confirmText || '确认'}
          </button>
        </div>
      </div>
    </div>
  );
}
