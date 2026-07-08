import { ChevronDown, ChevronRight, Download, FileCode2, FolderClosed, FolderOpen, Pencil, Trash2, Upload } from 'lucide-react';
import { getFileTreeDropZone, getParentPath, pathMatches } from '../../hooks/ideUtils';
import { shouldStartFileTreeItemDrag } from '../../hooks/fileTreeDragUtils';
import { isNotePath } from '../../hooks/noteTreeUtils.js';
import FileTree from './FileTree';
import DropInsertionLine from './FileTreeDropInsertionLine';
import NoteTreeExportButton from './FileTreeExportButton';
import FileTreeLabel from './FileTreeLabel';
import InlineRenameInput from './FileTreeInlineRenameInput';

export default function FileTreeRow({ item, items, depth, ide }) {
    const indicator = ide.fileDropIndicator?.targetPath === item.path ? ide.fileDropIndicator : null;
    const showInsertBefore = indicator?.position === 'before';
    const showInsertAfter = indicator?.position === 'after';
    const isInlineRenaming = ide.inlineRenamePath === item.path;
    const isTextFile = item.type === 'file' && ide.isTextPath(item.path);
    const isActiveRow = ide.selectedPaths.has(item.path) || item.path === ide.currentFilePath;
    const isExitingRow = ide.exitingFilePaths?.has(item.path);
    const isRecentRow = ide.recentFileTreePaths?.has(item.path);
    const actionCount = item.type === 'folder' ? 5 : isTextFile ? 3 : 2;
    const actionWidth = actionCount * 20 + Math.max(0, actionCount - 1) * 4 + 2;

    const setInsertionIndicator = (position) => {
      const sourcePath = ide.draggingFilePath || '';
      if (sourcePath === item.path) {
        ide.setFileDropIndicator(null);
        ide.setFileDragOverId(null);
        return;
      }

      const siblings = items.filter((candidate) => candidate.path !== sourcePath);
      const targetIndex = siblings.findIndex((candidate) => candidate.path === item.path);
      if (targetIndex < 0) return;
      const previousPath = position === 'before' ? siblings[targetIndex - 1]?.path || '' : item.path;
      const nextPath = position === 'before' ? item.path : siblings[targetIndex + 1]?.path || '';
      ide.setFileDragOverId(null);
      ide.setFileDropIndicator({
        parentPath: getParentPath(item.path),
        position,
        previousPath,
        nextPath,
        targetPath: item.path,
      });
    };

    const clearRowDropState = () => {
      if (ide.fileDropIndicator?.targetPath === item.path) ide.setFileDropIndicator(null);
      if (ide.fileDragOverId === item.id) ide.setFileDragOverId(null);
    };

    return (
    <div key={item.id} className="w-full">
      {showInsertBefore && <DropInsertionLine depth={depth} />}
      <div
        data-testid={`file-row-${item.path}`}
        data-file-path={item.path}
        data-file-type={item.type}
        data-file-active={isActiveRow ? 'true' : undefined}
        draggable={!isInlineRenaming}
        onDragStart={(e) => {
          if (!shouldStartFileTreeItemDrag({ target: e.target, isInlineRenaming })) {
            e.preventDefault();
            e.stopPropagation();
            return;
          }
          ide.handleFileDragStart(e, item.id, item.path);
          e.dataTransfer.effectAllowed = 'move';
          const selectedRefs =
            ide.selectedPaths?.has(item.path) && ide.selectedItems?.length > 1
              ? ide.selectedItems.map((selected) => ({
                  path: selected.path,
                  name: selected.name,
                  type: selected.type,
                  file_type: selected.type === 'file' ? ide.detectFileType?.(selected.path) || 'file' : 'folder',
                }))
              : [{ path: item.path, name: item.name, type: item.type, file_type: item.type === 'file' ? ide.detectFileType?.(item.path) || 'file' : 'folder' }];
          e.dataTransfer.setData('workspace_refs', JSON.stringify(selectedRefs));
          e.dataTransfer.setData('workspace_path', item.path);
          e.dataTransfer.setData('workspace_type', item.type);
        }}
        onDragEnd={ide.resetFileDragState}
        onDragOver={(e) => {
          const isWorkspaceDrag = Boolean(
            ide.draggingFileId ||
              e.dataTransfer?.types?.includes('workspace_path') ||
              e.dataTransfer?.types?.includes('file_id')
          );
          const isFileUpload = e.dataTransfer?.types?.includes('Files');
          if (isWorkspaceDrag) {
            e.preventDefault();
            e.stopPropagation();
            e.dataTransfer.dropEffect = 'move';
            const zone = getFileTreeDropZone(e.clientY, e.currentTarget.getBoundingClientRect(), item.type);
            if (zone === 'before' || zone === 'after') {
              setInsertionIndicator(zone);
              return;
            }
            ide.setFileDropIndicator(null);
            if (zone === 'inside' && item.type === 'folder') {
              ide.setFileDragOverId(item.id);
            } else {
              ide.setFileDragOverId(null);
            }
            return;
          }

          if (item.type === 'folder' && isFileUpload) {
            e.preventDefault();
            e.stopPropagation();
            e.dataTransfer.dropEffect = 'copy';
            ide.setFileDropIndicator(null);
            ide.setFileDragOverId(item.id);
          }
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) {
            e.stopPropagation();
            clearRowDropState();
          }
        }}
        onDrop={(e) => {
          const sourcePath = e.dataTransfer?.getData('workspace_path');
          if (sourcePath) {
            e.preventDefault();
            e.stopPropagation();
            const dropIndicator = ide.fileDropIndicator;
            ide.resetFileDragState();
            if (dropIndicator?.targetPath === item.path && (dropIndicator.position === 'before' || dropIndicator.position === 'after')) {
              ide.reorderPath({
                sourcePath,
                targetParentPath: dropIndicator.parentPath,
                previousPath: dropIndicator.previousPath,
                nextPath: dropIndicator.nextPath,
              }).catch((error) => {
                console.error(error);
              });
              return;
            }
            if (item.type !== 'folder') return;
            if (pathMatches(item.path, sourcePath)) {
              ide.movePath(sourcePath, item.path).catch((error) => {
                console.error(error);
              });
              return;
            }
            ide.movePath(sourcePath, item.path).catch((error) => {
              console.error(error);
            });
            return;
          }

          if (item.type !== 'folder') return;
          if (e.dataTransfer?.files?.length) {
            e.preventDefault();
            e.stopPropagation();
            ide.resetFileDragState();
            ide.uploadFiles(e.dataTransfer.files, item.path).catch((error) => {
              console.error(error);
            });
          }
        }}
        onClick={(e) => {
          if (ide.inlineRenamePath === item.path) return;
          ide.selectTreeItem(item.path, item.type, e);
          e.currentTarget.closest('aside')?.focus();
          if (e.shiftKey || e.ctrlKey || e.metaKey) return;
          if (item.type === 'file') ide.openFile(item.path);
        }}
        style={{
          paddingLeft: `${depth * 12 + 8}px`,
          '--file-tree-action-width': `${actionWidth}px`,
        }}
        className={`file-tree-row motion-row motion-list-enter relative min-w-0 overflow-hidden py-1.5 pr-2 rounded-lg cursor-pointer group text-[13px]
            ${
              ide.fileDragOverId === item.id
                ? 'bg-blue-100 dark:bg-blue-900/50 ring-1 ring-blue-400'
                : ide.selectedPaths.has(item.path)
                  ? 'bg-blue-50 dark:bg-blue-500/15 ring-1 ring-blue-300/70 dark:ring-blue-500/40'
                : item.path === ide.currentFilePath
                  ? 'bg-blue-50 dark:bg-blue-500/15 ring-1 ring-blue-300/70 dark:ring-blue-500/40'
                  : 'hover:bg-gray-100 dark:hover:bg-white/5'
            }
            ${isExitingRow ? 'motion-list-exit pointer-events-none' : ''}
            ${isRecentRow ? 'file-tree-row-recent' : ''}
            ${ide.draggingFileId === item.id ? 'opacity-40' : 'opacity-100'}
            text-gray-700 dark:text-gray-300`}
      >
        <button
          type="button"
          data-testid={`toggle-folder-${item.path}`}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => {
            e.stopPropagation();
            if (item.type === 'folder') ide.toggleFolder(item.path);
          }}
          className={`motion-button ui-icon-button ui-icon-button-compact file-tree-toggle h-5 w-5 ${
            item.type === 'folder' ? 'visible' : 'invisible'
          }`}
          title={item.isOpen ? '收起文件夹' : '展开文件夹'}
          tabIndex={item.type === 'folder' ? 0 : -1}
        >
          {item.isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <span className="file-tree-icon motion-color flex h-5 w-4 items-center justify-center text-gray-400 group-hover:text-blue-500 dark:text-gray-500">
          {item.type === 'folder' ? (
            item.isOpen ? (
              <FolderOpen size={15} className="text-blue-400" />
            ) : (
              <FolderClosed size={15} />
            )
          ) : (
            <FileCode2 size={15} />
          )}
        </span>
        {isInlineRenaming ? (
          <InlineRenameInput item={item} ide={ide} />
        ) : (
          <FileTreeLabel item={item} />
        )}
        {!isInlineRenaming && (
          <div className="file-tree-actions flex items-center gap-1">
            {item.type === 'folder' && (
              <>
                <button
                  data-testid={`create-file-${item.path}`}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.selectItem(item.path, item.type);
                    ide.openCreateFileModal(item.path).catch((error) => console.error(error));
                  }}
                  className="motion-button ui-icon-button ui-icon-button-compact h-5 w-5"
                  title="新建文件"
                >
                  <FileCode2 size={12} />
                </button>
                <label
                  data-testid={`upload-${item.path}`}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => e.stopPropagation()}
                  className="motion-button ui-icon-button ui-icon-button-compact h-5 w-5 cursor-pointer"
                  title="上传到此文件夹"
                >
                  <Upload size={12} />
                  <input
                    type="file"
                    multiple
                    className="hidden"
                    onChange={(e) => {
                      ide.uploadFiles(e.target.files, item.path).catch((error) => console.error(error));
                      e.target.value = '';
                    }}
                  />
                </label>
                <button
                  data-testid={`download-${item.path}`}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    ide.downloadFromTreeAction(item.path, item.type).catch((error) => console.error(error));
                  }}
                  className="motion-button ui-icon-button ui-icon-button-compact h-5 w-5"
                  title="下载文件夹"
                >
                  <Download size={12} />
                </button>
              </>
            )}
            <button
              data-testid={`rename-${item.path}`}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                ide.openRenameModal(item.path, item.type).catch((error) => console.error(error));
              }}
              className="motion-button ui-icon-button ui-icon-button-compact h-5 w-5"
              title="重命名"
            >
              <Pencil size={12} />
            </button>
            {isTextFile && isNotePath(item.path) && <NoteTreeExportButton item={item} ide={ide} />}
            {isTextFile && !isNotePath(item.path) && (
              <button
                data-testid={`download-${item.path}`}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.stopPropagation();
                  ide.downloadFromTreeAction(item.path, item.type).catch((error) => console.error(error));
                }}
                className="motion-button ui-icon-button ui-icon-button-compact h-5 w-5"
                title="下载"
              >
                <Download size={12} />
              </button>
            )}
            <button
              data-testid={`delete-${item.path}`}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation();
                if (ide.selectedPaths?.has(item.path) && ide.selectedItems?.length > 1) {
                  ide.deleteSelectedItems().catch((error) => console.error(error));
                  return;
                }
                ide.openDeleteModal(item.path, item.type).catch((error) => console.error(error));
              }}
              className="motion-button ui-icon-button ui-icon-button-compact ui-icon-button-danger h-5 w-5"
              title="删除"
            >
              <Trash2 size={12} />
            </button>
          </div>
        )}
      </div>
      {showInsertAfter && <DropInsertionLine depth={depth} />}

      {item.type === 'folder' && ide.fileDragOverId === item.id && (
        <div
          className="file-drop-hint settings-expand-enter mx-2 mb-1 rounded-lg border border-blue-200 bg-blue-50 px-2 py-1 text-[11px] font-semibold text-blue-600 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-200"
          style={{ marginLeft: `${depth * 12 + 28}px` }}
        >
          {ide.draggingFileId ? `松开后移动到 ${item.path}` : `松开后上传到 ${item.path}`}
        </div>
      )}

      {item.type === 'folder' && item.isOpen && item.children && (
        <div className="settings-expand-enter w-full">
          <FileTree items={item.children} depth={depth + 1} ide={ide} />
        </div>
      )}
    </div>
    );
}
