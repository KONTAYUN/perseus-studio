import React, { useRef, useLayoutEffect } from 'react';
import { Download, FileCode2, GripHorizontal, X } from 'lucide-react';

export default function EditorPanel({ ide, editor, panelId }) {
  const textareaRef = useRef(null);

  useLayoutEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = '0';
    el.style.height = Math.max(el.scrollHeight, el.parentElement?.clientHeight || 0) + 'px';
  }, [editor.currentFileContent]);

  return (
    <section
      id="panel-content-editor"
      data-testid="editor-panel"
      className={`editor-panel-root ui-panel-surface h-full flex flex-col bg-white dark:bg-[#1E1E1E] overflow-hidden transition-[border-color,box-shadow,background-color,transform] duration-300 ease-[cubic-bezier(0.19,1,0.22,1)] border relative ${ide.getPanelGlowStyle(
        panelId
      )}`}
      onClick={() => {
        ide.setActivePanel(panelId);
        ide.setActiveEditorPanelId(panelId);
      }}
    >
      <div
        className="h-12 flex-none min-w-0 bg-gray-50/80 dark:bg-[#252526] border-b border-gray-200/50 dark:border-white/5 flex items-center gap-4 px-6 group select-none relative z-20 max-[640px]:px-4"
        onDragOver={(e) => {
          const hasPath = e.dataTransfer?.types?.includes('workspace_path') || e.dataTransfer?.types?.includes('editor_tab_path');
          if (!hasPath) return;
          e.preventDefault();
          e.stopPropagation();
          e.dataTransfer.dropEffect = 'move';
        }}
        onDrop={(e) => {
          if (!e.dataTransfer?.getData('editor_tab_path') && e.dataTransfer?.getData('workspace_type') !== 'file') return;
          const path = e.dataTransfer?.getData('editor_tab_path') || e.dataTransfer?.getData('workspace_path');
          const sourcePanelId = e.dataTransfer?.getData('editor_tab_source_panel_id');
          if (!path) return;
          e.preventDefault();
          e.stopPropagation();
          ide.openFileInEditorPanel(panelId, path, { sourcePanelId }).catch((error) => console.error(error));
        }}
      >
        <span
          draggable
          onDragStart={(e) => ide.handlePanelDragStart(e, panelId)}
          onDragEnd={ide.handlePanelDragEnd}
          className="cursor-grab active:cursor-grabbing"
          title="拖动面板"
        >
          <GripHorizontal size={14} className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400" />
        </span>
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div className="editor-tabs-strip flex min-w-0 flex-1 items-center gap-1 overflow-hidden">
            {editor.openTabs.length === 0 ? (
              <div className="flex min-w-0 max-w-[220px] items-center bg-white dark:bg-[#1C1C1E] px-2.5 py-1.5 rounded-lg border border-gray-200 dark:border-gray-700/60 shadow-sm gap-2 text-sm text-gray-500 dark:text-gray-400">
                <FileCode2 size={14} className="shrink-0 text-blue-500" />
                <span className="truncate">未打开文件</span>
              </div>
            ) : (
              editor.openTabs.map((tab) => {
                const active = tab.path === editor.currentFilePath;
                const dirty = (tab.content || '') !== (tab.savedContent || '');
                return (
                  <button
                    key={tab.path}
                    data-testid={`editor-tab-${tab.path}`}
                    draggable
                    onDragStart={(e) => {
                      e.stopPropagation();
                      e.dataTransfer.effectAllowed = 'move';
                      e.dataTransfer.setData('editor_tab_path', tab.path);
                      e.dataTransfer.setData('editor_tab_source_panel_id', panelId);
                      e.dataTransfer.setData(
                        'workspace_refs',
                        JSON.stringify([
                          {
                            path: tab.path,
                            name: tab.path.split('/').pop() || tab.path,
                            type: 'file',
                            file_type: tab.fileType || ide.detectFileType?.(tab.path) || 'file',
                          },
                        ])
                      );
                      e.dataTransfer.setData('workspace_path', tab.path);
                      e.dataTransfer.setData('workspace_type', 'file');
                    }}
                    onClick={(e) => {
                      e.stopPropagation();
                      editor.activateTab(tab.path);
                    }}
                    className={`motion-chip group/tab flex h-8 min-w-0 max-w-[220px] shrink-0 items-center gap-2 rounded-lg border px-2.5 text-sm shadow-sm transition ${
                      active
                        ? 'border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-500/40 dark:bg-blue-500/15 dark:text-blue-100'
                        : 'border-gray-200 bg-white text-gray-600 hover:border-blue-200 dark:border-gray-700/60 dark:bg-[#1C1C1E] dark:text-gray-300'
                    }`}
                    title={tab.path}
                  >
                    <FileCode2 size={14} className="shrink-0 text-blue-500" />
                    <span className="min-w-0 truncate font-medium">{tab.path.split('/').pop() || tab.path}</span>
                    {dirty && <span className="shrink-0 text-[10px] text-orange-500">●</span>}
                    <button
                      type="button"
                      tabIndex={-1}
                      onClick={(e) => {
                        e.stopPropagation();
                        editor.closeTab(tab.path);
                      }}
                      className="motion-button editor-tab-close flex h-5 w-5 shrink-0 items-center justify-center rounded text-gray-400 hover:bg-gray-200 hover:text-gray-700 dark:hover:bg-white/10 dark:hover:text-gray-100"
                      title="关闭文件"
                    >
                      <X size={12} />
                    </button>
                  </button>
                );
              })
            )}
          </div>
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <button
            data-testid="editor-download"
            draggable={Boolean(editor.currentFilePath && editor.isTextFile)}
            onDragStart={(e) => {
              e.stopPropagation();
              ide.handleDownloadDragStart(e, editor.currentFilePath, editor.currentFileContent);
            }}
            onClick={(e) => {
              e.stopPropagation();
              ide.downloadFile(editor.currentFilePath).catch((error) => console.error(error));
            }}
            disabled={!editor.currentFilePath || !editor.isTextFile}
            className={`motion-button flex h-8 items-center gap-1 rounded-lg px-2 text-xs font-semibold ${
              !editor.currentFilePath || !editor.isTextFile
                ? 'bg-gray-200/70 dark:bg-white/5 text-gray-400 cursor-not-allowed'
                : 'bg-white dark:bg-[#1C1C1E] text-gray-600 dark:text-gray-300 border border-gray-200 dark:border-gray-700/60 hover:border-blue-400 hover:text-blue-500'
            }`}
          >
            <Download size={13} />
            <span className="hidden xl:inline">下载</span>
          </button>
        </div>
      </div>

      <div
        className="relative flex-1 min-h-0 overflow-auto px-12 py-7 pb-36 font-mono text-[14px] leading-7 tracking-[0.012em] text-gray-800 dark:text-gray-300 bg-[#FAFAFA] dark:bg-[#1E1E1E] custom-scrollbar max-[900px]:px-8 max-[640px]:px-5"
        onDragOver={(e) => {
          const hasPath = e.dataTransfer?.types?.includes('workspace_path') || e.dataTransfer?.types?.includes('editor_tab_path');
          if (!hasPath) return;
          e.preventDefault();
          e.stopPropagation();
          e.dataTransfer.dropEffect = 'move';
          e.currentTarget.querySelector('.editor-drop-overlay')?.classList.add('border-blue-400/70', 'bg-blue-500/10');
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) {
            e.currentTarget.querySelector('.editor-drop-overlay')?.classList.remove('border-blue-400/70', 'bg-blue-500/10');
          }
        }}
        onDrop={(e) => {
          if (!e.dataTransfer?.getData('editor_tab_path') && e.dataTransfer?.getData('workspace_type') !== 'file') return;
          const path = e.dataTransfer?.getData('editor_tab_path') || e.dataTransfer?.getData('workspace_path');
          const sourcePanelId = e.dataTransfer?.getData('editor_tab_source_panel_id');
          if (!path) return;
          e.preventDefault();
          e.stopPropagation();
          e.currentTarget.querySelector('.editor-drop-overlay')?.classList.remove('border-blue-400/70', 'bg-blue-500/10');
          ide.openFileInEditorPanel(panelId, path, { sourcePanelId }).catch((error) => console.error(error));
        }}
      >
        <div
          className="editor-drop-overlay pointer-events-none absolute inset-x-10 bottom-4 top-16 z-10 rounded-2xl border-2 border-dashed border-transparent transition-colors max-[900px]:inset-x-6 max-[640px]:inset-x-3"
        />
        <div className="relative z-20">
        {ide.errorMessage && (
          <div className="settings-expand-enter mb-4 rounded-xl border border-red-200/80 bg-red-50/80 px-3 py-2 text-[12px] text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
            {ide.errorMessage}
          </div>
        )}

        {editor.externalFileConflict && (
          <div className="settings-expand-enter mb-4 rounded-xl border border-amber-200/80 bg-amber-50/90 px-3 py-3 text-[12px] text-amber-700 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-200">
            <div>{editor.externalFileConflict.message}</div>
            <div className="mt-3 flex flex-wrap gap-2">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  editor.keepLocalFileChanges();
                }}
                className="rounded-lg border border-amber-300/80 px-3 py-1.5 text-[12px] font-semibold text-amber-700 transition hover:bg-amber-100/80 dark:border-amber-300/20 dark:text-amber-100 dark:hover:bg-amber-400/10"
              >
                保留本地修改
              </button>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  editor.reloadCurrentFileFromConflict();
                }}
                className="rounded-lg bg-amber-500 px-3 py-1.5 text-[12px] font-semibold text-white transition hover:bg-amber-600"
              >
                重新加载文件
              </button>
            </div>
          </div>
        )}

        {editor.editorStatusMessage && !editor.externalFileConflict && editor.currentFilePath && (
          <div className="settings-expand-enter mb-4 rounded-xl border border-blue-200/80 bg-blue-50/80 px-3 py-2 text-[12px] text-blue-600 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-200">
            {editor.editorStatusMessage}
          </div>
        )}
        </div>

        {!editor.currentFilePath ? (
          <div className="relative z-20 h-full flex items-center justify-center text-sm text-gray-400 dark:text-gray-500">
            {editor.editorStatusMessage || '从左侧文件树选择一个文件开始编辑'}
          </div>
        ) : editor.isEditorLoading ? (
          <div className="relative z-20 h-full flex items-center justify-center text-sm text-gray-400 dark:text-gray-500">
            正在加载文件...
          </div>
        ) : !editor.isTextFile ? (
          <div className="relative z-20 h-full flex items-center justify-center text-sm text-gray-400 dark:text-gray-500">
            {editor.currentFileNotice}
          </div>
        ) : (
          <textarea
            ref={textareaRef}
            data-testid="editor-textarea"
            value={editor.currentFileContent}
            onChange={(e) => editor.setCurrentFileContent(e.target.value)}
            className="relative z-20 block w-full min-h-full resize-none overflow-hidden bg-transparent p-0 pb-40 leading-7 tracking-[0.012em] outline-none text-gray-800 dark:text-gray-300 placeholder-gray-400 dark:placeholder-gray-500"
            spellCheck={false}
          />
        )}
      </div>
    </section>
  );
}
