import React from 'react';
import { Eraser, FileCode2, FolderClosed, Recycle, RotateCcw, Trash2, X } from 'lucide-react';
import { AnchoredPopover, Button, IconButton } from '../ui';

export default function TrashPopover({ ide, anchorRef }) {
  return (
    <AnchoredPopover
      anchorRef={anchorRef}
      width={320}
      open={ide.isTrashPanelOpen}
      onClose={ide.closeTrashPanel}
      testId="trash-popover"
    >
      {({ position }) => (
        <>
          <div className="flex items-center justify-between gap-3 border-b border-gray-100 px-3 py-2.5 dark:border-white/10">
            <div className="min-w-0">
              <div className="text-sm font-bold text-gray-800 dark:text-gray-100">回收站</div>
              <div className="mt-0.5 text-[11px] text-gray-400">
                {ide.trashItems?.length ? `${ide.trashItems.length} 项已删除` : '没有已删除项目'}
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Button
                type="button"
                data-testid="empty-trash-from-popover"
                variant="dangerSubtle"
                size="xs"
                disabled={ide.isTrashLoading || ide.trashItems?.length === 0}
                onClick={(e) => {
                  e.stopPropagation();
                  ide.closeTrashPanel();
                  ide.emptyTrash().catch((error) => console.error(error));
                }}
                className="h-7 border-transparent bg-transparent px-2 text-[11px] text-gray-500 dark:bg-transparent dark:text-gray-400"
                title="清空回收站"
              >
                <Eraser size={13} />
                清空
              </Button>
              <IconButton
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  ide.closeTrashPanel();
                }}
                size="xs"
                className="h-7 w-7"
                title="关闭"
              >
                <X size={14} />
              </IconButton>
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
                  className="motion-row trash-item-row group/trash flex min-w-0 items-center gap-2 rounded-xl px-3 py-2.5 text-gray-700 hover:bg-gray-50/90 dark:text-gray-200 dark:hover:bg-white/5"
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
                  <div className="flex shrink-0 items-center gap-1 opacity-0 transition-opacity duration-200 ease-apple group-hover/trash:opacity-100">
                    <IconButton
                      type="button"
                      data-testid={`restore-trash-${item.path}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        ide.restoreTrashItem(item).catch((error) => console.error(error));
                      }}
                      size="xs"
                      className="h-7 w-7"
                      title="移出回收站"
                    >
                      <RotateCcw size={13} />
                    </IconButton>
                    <IconButton
                      type="button"
                      data-testid={`delete-trash-${item.path}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        ide.deleteTrashItem(item).catch((error) => console.error(error));
                      }}
                      variant="danger"
                      size="xs"
                      className="h-7 w-7"
                      title="彻底删除"
                    >
                      <Trash2 size={13} />
                    </IconButton>
                  </div>
                </div>
              ))
            )}
          </div>
        </>
      )}
    </AnchoredPopover>
  );
}
