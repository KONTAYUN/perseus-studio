import React, { useRef, useState } from 'react';
import { Download, FileCode2, PanelTop, X } from 'lucide-react';
import { getNoteTitle, isNotePath } from '../../hooks/noteTreeUtils.js';
import { AnchoredPopover, Button } from '../ui';
import RichTextEditor from './richText/RichTextEditor.jsx';

export default function EditorPanel({ ide, editor, panelId }) {
  const tabsStripRef = useRef(null);
  const exportButtonRef = useRef(null);
  const [tabDropIndicator, setTabDropIndicator] = useState(null);
  const [isExportMenuOpen, setIsExportMenuOpen] = useState(false);
  const isCurrentNote = isNotePath(editor.currentFilePath);

  const scrollTabsWithWheel = (event) => {
    const strip = tabsStripRef.current;
    if (!strip || strip.scrollWidth <= strip.clientWidth + 1) return;
    const delta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
    if (!delta) return;
    event.preventDefault();
    strip.scrollLeft += delta;
  };

  const getTabDropPosition = (event) => {
    const rect = event.currentTarget.getBoundingClientRect();
    return event.clientX < rect.left + rect.width / 2 ? 'before' : 'after';
  };

  const isSamePanelTabDrag = (event) =>
    event.dataTransfer?.types?.includes('editor_tab_path') &&
    event.dataTransfer?.getData('editor_tab_source_panel_id') === panelId;

  const clearTabDropIndicator = () => setTabDropIndicator(null);

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
          className="motion-button ui-icon-button ui-panel-drag-handle h-7 w-7"
          title="拖动面板"
        >
          <PanelTop size={15} strokeWidth={1.9} className="shrink-0" />
        </span>
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <div
            ref={tabsStripRef}
            className="editor-tabs-strip custom-scrollbar flex min-w-0 flex-1 items-center gap-1 overflow-x-auto overflow-y-hidden"
            onWheel={scrollTabsWithWheel}
            onDragLeave={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget)) clearTabDropIndicator();
            }}
          >
            {editor.openTabs.length === 0 ? (
              <div className="editor-empty-tab flex min-w-0 items-center bg-white dark:bg-[#1C1C1E] px-2.5 py-1.5 rounded-lg border border-gray-200 dark:border-gray-700/60 gap-2 text-sm text-gray-500 dark:text-gray-400">
                <FileCode2 size={14} className="shrink-0 text-blue-500" />
                <span className="truncate">未打开文件</span>
              </div>
            ) : (
              editor.openTabs.map((tab) => {
                const active = tab.path === editor.currentFilePath;
                const dirty = (tab.content || '') !== (tab.savedContent || '');
                return (
                  <div
                    key={tab.path}
                    role="button"
                    tabIndex={0}
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
                    onDragOver={(e) => {
                      if (!isSamePanelTabDrag(e)) return;
                      const sourcePath = e.dataTransfer.getData('editor_tab_path');
                      if (!sourcePath || sourcePath === tab.path) {
                        e.preventDefault();
                        e.stopPropagation();
                        clearTabDropIndicator();
                        return;
                      }
                      e.preventDefault();
                      e.stopPropagation();
                      e.dataTransfer.dropEffect = 'move';
                      const position = getTabDropPosition(e);
                      setTabDropIndicator((current) =>
                        current?.targetPath === tab.path && current?.position === position
                          ? current
                          : { targetPath: tab.path, position }
                      );
                    }}
                    onDrop={(e) => {
                      if (!isSamePanelTabDrag(e)) return;
                      const sourcePath = e.dataTransfer.getData('editor_tab_path');
                      if (!sourcePath || sourcePath === tab.path) {
                        e.preventDefault();
                        e.stopPropagation();
                        clearTabDropIndicator();
                        return;
                      }
                      e.preventDefault();
                      e.stopPropagation();
                      const position = tabDropIndicator?.targetPath === tab.path
                        ? tabDropIndicator.position
                        : getTabDropPosition(e);
                      editor.reorderTab?.(sourcePath, tab.path, position);
                      clearTabDropIndicator();
                    }}
                    onDragEnd={clearTabDropIndicator}
                    onClick={(e) => {
                      e.stopPropagation();
                      editor.activateTab(tab.path);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        e.stopPropagation();
                        editor.activateTab(tab.path);
                      }
                    }}
                    className={`motion-chip editor-tab-item group/tab flex h-8 min-w-0 items-center gap-2 rounded-lg border px-2.5 text-sm transition-[background-color,border-color,color,opacity] duration-180 ease-apple ${
                      active
                        ? 'border-blue-300 bg-blue-50 text-blue-700 dark:border-blue-500/40 dark:bg-blue-500/15 dark:text-blue-100'
                        : 'border-gray-200 bg-white text-gray-600 hover:border-blue-200 dark:border-gray-700/60 dark:bg-[#1C1C1E] dark:text-gray-300'
                    }`}
                    title={tab.path}
                  >
                    {tabDropIndicator?.targetPath === tab.path && (
                      <span
                        aria-hidden="true"
                        className={`editor-tab-drop-line ${
                          tabDropIndicator.position === 'after' ? 'editor-tab-drop-line-after' : 'editor-tab-drop-line-before'
                        }`}
                      />
                    )}
                    <FileCode2 size={14} className="shrink-0 text-blue-500" />
                    <span className="min-w-0 flex-1 truncate font-medium">
                      {isNotePath(tab.path) ? getNoteTitle(tab.path) : tab.path.split('/').pop() || tab.path}
                    </span>
                    {dirty && <span className="shrink-0 text-[10px] text-orange-500">●</span>}
                    <button
                      type="button"
                      tabIndex={-1}
                      onClick={(e) => {
                        e.stopPropagation();
                        editor.closeTab(tab.path);
                      }}
                      className="motion-button ui-icon-button ui-icon-button-compact editor-tab-close h-5 w-5"
                      title="关闭文件"
                    >
                      <X size={12} />
                    </button>
                  </div>
                );
              })
            )}
          </div>
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-2">
          <Button
            ref={exportButtonRef}
            data-testid="editor-download"
            type="button"
            variant="subtle"
            size="xs"
            draggable={Boolean(editor.currentFilePath && editor.isTextFile)}
            onDragStart={(e) => {
              e.stopPropagation();
              ide.handleDownloadDragStart(e, editor.currentFilePath, editor.currentFileContent);
            }}
            onClick={(e) => {
              e.stopPropagation();
              if (isCurrentNote) {
                setIsExportMenuOpen((current) => !current);
                return;
              }
              ide.downloadFile(editor.currentFilePath).catch((error) => console.error(error));
            }}
            disabled={!editor.currentFilePath || !editor.isTextFile}
            className="h-8 px-2"
          >
            <Download size={13} />
            <span className="hidden xl:inline">{isCurrentNote ? '导出' : '下载'}</span>
          </Button>
          <AnchoredPopover
            anchorRef={exportButtonRef}
            width={176}
            minWidth={176}
            maxHeight={220}
            open={isExportMenuOpen && isCurrentNote}
            onClose={() => setIsExportMenuOpen(false)}
            testId="note-export-menu"
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
                  setIsExportMenuOpen(false);
                  ide.exportNotes?.({
                    path: editor.currentFilePath,
                    type: 'file',
                    format,
                  }).catch((error) => console.error(error));
                }}
              >
                {label}
              </button>
            ))}
          </AnchoredPopover>
        </div>
      </div>

      <div
        className="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-[#FAFAFA] text-[14px] leading-7 tracking-[0.012em] text-gray-800 dark:bg-[#1E1E1E] dark:text-gray-300"
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
        <div className="relative z-20 flex min-h-0 flex-1 flex-col">
          {(ide.errorMessage || editor.externalFileConflict || (editor.editorStatusMessage && editor.currentFilePath)) && (
            <div className="flex-none px-12 pt-7 max-[900px]:px-8 max-[640px]:px-5">
              {ide.errorMessage && (
                <div className="settings-expand-enter mb-4 rounded-xl border border-red-200/80 bg-red-50/80 px-3 py-2 text-[12px] text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
                  {ide.errorMessage}
                </div>
              )}

              {editor.externalFileConflict && (
                <div className="editor-conflict-notice settings-expand-enter mb-4 rounded-xl border border-amber-200/80 bg-amber-50/90 px-3 py-3 text-[12px] text-amber-700 dark:border-amber-400/20 dark:bg-amber-500/10 dark:text-amber-200">
                  <div>{editor.externalFileConflict.message}</div>
                  <div className="mt-3 flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant="subtle"
                      size="xs"
                      onClick={(e) => {
                        e.stopPropagation();
                        editor.keepLocalFileChanges();
                      }}
                      className="border-amber-300/80 text-amber-700 hover:bg-amber-100/80 dark:border-amber-300/20 dark:text-amber-100 dark:hover:bg-amber-400/10"
                    >
                      保留本地修改
                    </Button>
                    <Button
                      type="button"
                      variant="warning"
                      size="xs"
                      onClick={(e) => {
                        e.stopPropagation();
                        editor.reloadCurrentFileFromConflict();
                      }}
                    >
                      重新加载文件
                    </Button>
                  </div>
                </div>
              )}

              {editor.editorStatusMessage && !editor.externalFileConflict && editor.currentFilePath && (
                <div className="settings-expand-enter mb-4 rounded-xl border border-blue-200/80 bg-blue-50/80 px-3 py-2 text-[12px] text-blue-600 dark:border-blue-500/20 dark:bg-blue-500/10 dark:text-blue-200">
                  {editor.editorStatusMessage}
                </div>
              )}
            </div>
          )}

          <div className="min-h-0 flex-1">
            {!editor.currentFilePath ? (
              <div className="flex h-full items-center justify-center px-12 py-7 text-sm text-gray-400 dark:text-gray-500 max-[900px]:px-8 max-[640px]:px-5">
                {editor.editorStatusMessage || '从左侧文件树选择一个文件开始编辑'}
              </div>
            ) : editor.isEditorLoading ? (
              <div className="flex h-full items-center justify-center px-12 py-7 text-sm text-gray-400 dark:text-gray-500 max-[900px]:px-8 max-[640px]:px-5">
                正在加载文件...
              </div>
            ) : !editor.isTextFile ? (
              <div className="flex h-full items-center justify-center px-12 py-7 text-sm text-gray-400 dark:text-gray-500 max-[900px]:px-8 max-[640px]:px-5">
                {editor.currentFileNotice}
              </div>
            ) : isCurrentNote ? (
              <RichTextEditor
                filePath={editor.currentFilePath}
                html={editor.currentFileContent}
                onHtmlChange={editor.setCurrentFileContent}
                onStatusMessage={editor.setEditorStatusMessage}
              />
            ) : (
              <textarea
                data-testid="editor-textarea"
                value={editor.currentFileContent}
                onChange={(e) => editor.setCurrentFileContent(e.target.value)}
                className="editor-textarea custom-scrollbar block h-full min-h-0 w-full resize-none overflow-y-auto overflow-x-hidden bg-transparent px-12 py-7 pb-36 leading-7 tracking-[0.012em] text-gray-800 outline-none placeholder-gray-400 dark:text-gray-300 dark:placeholder-gray-500 max-[900px]:px-8 max-[640px]:px-5"
                spellCheck={false}
              />
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
