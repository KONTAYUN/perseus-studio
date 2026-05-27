import React, { useRef } from 'react';
import {
  ClipboardList,
  MessageSquare,
  Moon,
  PanelLeft,
  PanelRight,
  Plus,
  LogOut,
  Settings,
  Sun,
  Trash2,
} from 'lucide-react';
import { MAX_EDITOR_PANELS, MAX_TOTAL_PANELS } from '../../hooks/useLayoutState';
import { AnchoredPopover } from '../ui';

export default function AppHeader({
  isDarkMode,
  layoutColumns,
  onLayoutChange,
  onAddPanel,
  onCloseDraggedPanel,
  onPanelTrashOver,
  onOpenSettings,
  onOpenTasks,
  onToggleDarkMode,
  onLogout,
  panels = [],
  taskCount = 0,
  taskDoneUnread = false,
  isPanelDragging = false,
  isPanelTrashOver = false,
  panelAddMenuOpen = false,
  setPanelAddMenuOpen,
}) {
  const addPanelButtonRef = useRef(null);
  const hasChatPanel = panels.some((panel) => panel.type === 'chat');
  const hasFileTreePanel = panels.some((panel) => panel.type === 'fileTree');
  const editorPanelCount = panels.filter((panel) => panel.type === 'editor').length;
  const panelLimitReached = panels.length >= MAX_TOTAL_PANELS;
  const editorLimitReached = editorPanelCount >= MAX_EDITOR_PANELS;
  const showSettings = typeof onOpenSettings === 'function';
  const addPanelOptions = [
    ['fileTree', '文件树', !hasFileTreePanel && !panelLimitReached],
    ['editor', '编辑器', !panelLimitReached && !editorLimitReached],
    ['chat', '聊天', !hasChatPanel && !panelLimitReached],
  ].filter(([, , visible]) => visible);

  const handleAddClick = () => {
    if (isPanelDragging) return;
    if (panelLimitReached) {
      setPanelAddMenuOpen?.(false);
      onAddPanel?.('editor');
      return;
    }
    if (addPanelOptions.length === 1) {
      setPanelAddMenuOpen?.(false);
      onAddPanel?.(addPanelOptions[0][0]);
      return;
    }
    if (hasChatPanel && hasFileTreePanel) {
      setPanelAddMenuOpen?.(false);
      onAddPanel?.('editor');
      return;
    }
    setPanelAddMenuOpen?.(!panelAddMenuOpen);
  };

  const handleTrashDragEnter = (e) => {
    if (!isPanelDragging) return;
    e.preventDefault();
    onPanelTrashOver?.(true);
  };

  const handleTrashDragOver = (e) => {
    if (!isPanelDragging) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    onPanelTrashOver?.(true);
  };

  const handleTrashDrop = (e) => {
    if (!isPanelDragging) return;
    e.preventDefault();
    onCloseDraggedPanel?.();
  };

  return (
    <header className="app-header h-14 flex-none bg-white/60 dark:bg-[#1C1C1E]/60 backdrop-blur-2xl border-b border-gray-200/50 dark:border-white/5 flex items-center justify-between px-5 z-30 transition-colors duration-200">
      <div className="flex h-full w-1/3 items-center gap-3">
        <img src="/perseus-mark.svg?v=6" alt="" className="h-7 w-7 rounded-xl" draggable="false" />
        <span className="font-bold text-[15px] leading-none tracking-tight text-gray-800 dark:text-gray-100">
          Perseus Studio
        </span>
      </div>

      <div
        className={`header-segmented relative flex h-9 w-1/3 items-center justify-center gap-1.5 p-1.5 shadow-inner transition-[background-color,color,box-shadow,ring-color,transform] duration-150 ease-out ${
          isPanelDragging
            ? isPanelTrashOver
              ? 'bg-red-500/95 text-white ring-2 ring-red-300/60'
              : 'bg-red-500/15 text-red-300 ring-1 ring-red-500/30'
            : 'bg-gray-100/80 dark:bg-black/40'
        }`}
        onDragEnter={handleTrashDragEnter}
        onDragOver={handleTrashDragOver}
        onDragLeave={() => onPanelTrashOver?.(false)}
        onDrop={handleTrashDrop}
        title={isPanelDragging ? '拖到这里关闭面板' : undefined}
      >
        {isPanelDragging ? (
          <div className={`pointer-events-none flex items-center gap-2 text-sm font-bold transition-transform duration-150 ${isPanelTrashOver ? 'scale-[1.03]' : 'scale-100'}`}>
            <Trash2 size={17} className="shrink-0" />
            <span>拖动到此处释放关闭面板</span>
          </div>
        ) : (
          <>
            {[
              { mode: 1, icon: <MessageSquare size={16} />, tooltip: '单栏模式' },
              { mode: 2, icon: <PanelRight size={16} />, tooltip: '双栏模式' },
              { mode: 3, icon: <PanelLeft size={16} />, tooltip: '三栏模式' },
            ].map((btn) => (
              <button
                key={btn.mode}
                data-testid={`layout-${btn.mode}`}
                onClick={() => onLayoutChange(btn.mode)}
                className={`motion-button ui-icon-button h-7 w-7 outline-none focus-visible:ring-2 focus-visible:ring-blue-400/35 ${
                  layoutColumns === btn.mode
                    ? 'ui-icon-button-active'
                    : ''
                }`}
                title={btn.tooltip}
              >
                {btn.icon}
              </button>
            ))}
            <div className="relative">
              <button
                ref={addPanelButtonRef}
                data-testid="add-panel-button"
                onClick={handleAddClick}
                className="motion-button ui-icon-button h-7 w-7 outline-none focus-visible:ring-2 focus-visible:ring-blue-400/35"
                title={panelLimitReached ? '当前最多支持 5 个面板' : hasChatPanel && hasFileTreePanel ? '添加编辑器' : '添加面板'}
              >
                <Plus size={16} />
              </button>
          <AnchoredPopover
            anchorRef={addPanelButtonRef}
            align="center"
            width={128}
            minWidth={128}
            maxHeight={220}
            open={panelAddMenuOpen && !isPanelDragging && addPanelOptions.length > 0}
            onClose={() => setPanelAddMenuOpen?.(false)}
            testId="add-panel-menu"
            className="py-1"
          >
              {addPanelOptions.map(([type, label]) => (
                <button
                  key={type}
                  data-testid={`add-panel-${type}`}
                  type="button"
                  onClick={() => onAddPanel?.(type)}
                  className="motion-row block w-full px-3 py-2 text-left text-sm font-semibold text-gray-600 hover:bg-blue-50 hover:text-blue-600 dark:text-gray-300 dark:hover:bg-blue-500/15 dark:hover:text-blue-200"
                >
                  {label}
                </button>
              ))}
          </AnchoredPopover>
            </div>
          </>
        )}
      </div>

      <div className="flex h-full w-1/3 items-center justify-end gap-3">
        <button
          onClick={onOpenTasks}
          className={`motion-button relative flex h-9 items-center gap-2 rounded-xl border px-2.5 text-xs font-semibold leading-none ${
            taskCount > 0
              ? 'border-blue-100/50 bg-blue-50/80 text-blue-600 hover:bg-blue-100/80 dark:border-blue-500/20 dark:bg-blue-500/15 dark:text-blue-400 dark:hover:bg-blue-500/20'
              : 'border-transparent text-gray-500 hover:bg-white hover:text-blue-500 dark:text-gray-400 dark:hover:bg-[#2C2C2E] dark:hover:text-blue-400'
          }`}
          title={taskCount > 0 ? `${taskCount} 个任务进行中` : '打开任务面板'}
        >
          {taskDoneUnread && taskCount === 0 && (
            <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-rose-500 ring-2 ring-rose-500/20" />
          )}
          <ClipboardList size={18} />
          <span className="hidden max-w-[120px] truncate sm:inline">
            {taskCount > 0 ? `${taskCount} 个任务进行中` : '任务'}
          </span>
        </button>
        <div className="h-4 w-px bg-gray-300 dark:bg-gray-700 mx-1" />

        <button
          onClick={onToggleDarkMode}
          className="motion-button ui-icon-button h-9 w-9"
        >
          {isDarkMode ? <Sun size={18} /> : <Moon size={18} />}
        </button>

        {showSettings && (
          <button
            data-testid="settings-button"
            onClick={onOpenSettings}
            className="motion-button ui-icon-button h-9 w-9"
          >
            <Settings size={18} />
          </button>
        )}
        <button
          onClick={onLogout}
          className="motion-button ui-icon-button ui-icon-button-danger h-9 w-9"
          title="退出登录"
        >
          <LogOut size={18} />
        </button>
      </div>
    </header>
  );
}
