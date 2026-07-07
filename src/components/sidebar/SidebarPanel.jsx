import React from 'react';
import { Download, FilePlus, FolderPlus, PanelTop, Plus, Recycle, Search, Trash2, Upload } from 'lucide-react';
import FileTree from './FileTree';
import FileActionModal from './FileActionModal';
import FileSearchPopover from './FileSearchPopover';
import TrashPopover from './TrashPopover';
import { AnchoredPopover, Button, IconButton } from '../ui';

export default function SidebarPanel({ ide, panelId = 'fileTree' }) {
  const rootUploadInputRef = React.useRef(null);
  const addButtonRef = React.useRef(null);
  const searchButtonRef = React.useRef(null);
  const trashButtonRef = React.useRef(null);
  const isOverFolderRow = (event) =>
    event.target.closest?.('[data-file-type="folder"]') &&
    event.currentTarget.contains(event.target.closest('[data-file-type="folder"]'));

  const uploadToRoot = (files) => {
    const fileItems = Array.from(files || []);
    const importable = fileItems.filter((file) => /\.(html?|md|markdown|docx|zip)$/i.test(file.name || ''));
    const generic = fileItems.filter((file) => !importable.includes(file));
    Promise.resolve()
      .then(async () => {
        for (const file of importable) {
          await ide.importNotes?.(file);
        }
        if (generic.length > 0) {
          await ide.uploadFiles(generic);
        }
      })
      .catch((error) => {
        console.error(error);
      });
  };

  return (
    <aside
      id="panel-content-sidebar"
      data-testid="file-tree-panel"
      className={`ui-panel-surface h-full flex flex-col bg-white/70 dark:bg-[#1C1C1E]/90 backdrop-blur-2xl overflow-hidden border relative ${ide.getPanelGlowStyle(
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
            className="motion-button ui-icon-button ui-panel-drag-handle h-7 w-7"
            title="拖动面板"
          >
            <PanelTop size={15} strokeWidth={1.9} className="shrink-0" />
          </span>
          <span className="truncate uppercase tracking-wider text-xs font-semibold">工作区</span>
          {ide.selectedItems.length > 1 && (
            <>
              <Button
                data-testid="download-selected"
                type="button"
                variant="subtle"
                size="xs"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.downloadSelectedItems().catch((error) => console.error(error));
                }}
                className="ml-1 h-7 border-blue-100 bg-blue-50 px-2 text-[11px] text-blue-600 hover:bg-blue-100 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-300 dark:hover:bg-blue-500/20"
                title="下载选中项"
              >
                <Download size={12} />
                {ide.selectedItems.length}
              </Button>
              <Button
                data-testid="delete-selected"
                type="button"
                variant="dangerSubtle"
                size="xs"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.deleteSelectedItems().catch((error) => console.error(error));
                }}
                className="h-7 px-2 text-[11px]"
                title="删除选中项"
              >
                <Trash2 size={12} />
              </Button>
            </>
          )}
        </div>

        <div className="relative">
          <div className="flex items-center gap-1">
            <IconButton
              ref={searchButtonRef}
              data-testid="workspace-search-button"
              onClick={(e) => {
                e.stopPropagation();
                if (ide.isFileSearchOpen) {
                  ide.closeFileSearch();
                } else {
                  ide.openFileSearch();
                }
              }}
              active={ide.isFileSearchOpen}
              size="xs"
              className="h-7 w-7"
              title="搜索文件"
            >
              <Search size={16} />
            </IconButton>
            <IconButton
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
              active={ide.isTrashPanelOpen}
              size="xs"
              className="h-7 w-7"
              title="打开回收站"
            >
              <Recycle size={16} />
            </IconButton>
            <IconButton
              ref={addButtonRef}
              data-testid="workspace-add-button"
              onClick={(e) => {
                e.stopPropagation();
                ide.setIsAddMenuOpen(!ide.isAddMenuOpen);
              }}
              size="xs"
              className="h-7 w-7"
              title="新建或上传"
            >
              <Plus size={16} />
            </IconButton>
          </div>

          <AnchoredPopover
            anchorRef={addButtonRef}
            width={144}
            minWidth={144}
            maxHeight={220}
            open={ide.isAddMenuOpen}
            onClose={() => ide.setIsAddMenuOpen(false)}
            testId="workspace-add-menu"
            className="py-1"
          >
              <button
                data-testid="workspace-create-file"
                onClick={() => ide.openCreateFileModal().catch((error) => console.error(error))}
                className="motion-row flex w-full items-center px-3 py-2 text-left text-sm text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-blue-500/20"
              >
                <FilePlus size={14} className="mr-2 opacity-70" /> 新建笔记
              </button>
              <button
                data-testid="workspace-create-folder"
                onClick={() => ide.openCreateFolderModal().catch((error) => console.error(error))}
                className="motion-row flex w-full items-center px-3 py-2 text-left text-sm text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-blue-500/20"
              >
                <FolderPlus size={14} className="mr-2 opacity-70" /> 新建文件夹
              </button>
              <button
                data-testid="workspace-upload-file"
                onClick={() => {
                  ide.setIsAddMenuOpen(false);
                  rootUploadInputRef.current?.click();
                }}
                className="motion-row flex w-full items-center px-3 py-2 text-left text-sm text-gray-700 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-blue-500/20"
              >
                <Upload size={14} className="mr-2 opacity-70" /> 导入/上传
              </button>
          </AnchoredPopover>
          <FileSearchPopover ide={ide} anchorRef={searchButtonRef} />
          <TrashPopover ide={ide} anchorRef={trashButtonRef} />
        </div>
      </div>

      <input
        ref={rootUploadInputRef}
        data-testid="workspace-upload-input"
        type="file"
        multiple
        accept=".html,.htm,.md,.markdown,.docx,.zip,text/*,image/*"
        className="hidden"
        onChange={(e) => {
          uploadToRoot(e.target.files);
          e.target.value = '';
        }}
      />

      <div
        data-testid="workspace-drop-root"
        className={`flex-1 overflow-y-auto p-2 custom-scrollbar scrollbar-rounded-clip motion-color relative ${
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
          <div className="workspace-drop-overlay pointer-events-none absolute inset-2 z-20 flex items-center justify-center rounded-2xl border-2 border-dashed border-blue-400/80 bg-blue-500/10 text-sm font-semibold text-blue-600 shadow-inner dark:bg-blue-500/15 dark:text-blue-200">
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
      <FileActionModal ide={ide} />
    </aside>
  );
}
