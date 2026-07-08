import useEditorTabs from './useEditorTabs';
import { buildEditorContextFromEntries } from './ideContextUtils';

export default function useIdeEditorPanels({
  confirm,
  editorRef,
  layout,
  loadTree,
  notify,
  setErrorMessage,
  setSelectedItemPath,
  setSelectedItemType,
} = {}) {
  const makeEditor = (panelId) =>
    useEditorTabs({
      loadTree,
      setActivePanel: (id) => {
        layout.setActivePanel(id === 'editor' ? panelId : id);
        layout.setActiveEditorPanelId(panelId);
      },
      setErrorMessage,
      setSelectedItemPath,
      setSelectedItemType,
      notify,
      confirm,
    });

  const editor1 = makeEditor('editor_1');
  const editor2 = makeEditor('editor_2');
  const editor3 = makeEditor('editor_3');
  const editor4 = makeEditor('editor_4');
  const editor5 = makeEditor('editor_5');
  const editorPanels = {
    editor_1: editor1,
    editor_2: editor2,
    editor_3: editor3,
    editor_4: editor4,
    editor_5: editor5,
  };
  const activeEditor = editorPanels[layout.activeEditorPanelId] || editor1;

  function visibleEditorEntries() {
    return layout.panels
      .filter((panel) => panel.type === 'editor')
      .map((panel) => ({ panel, editor: editorPanels[panel.id] }))
      .filter((item) => item.editor);
  }

  function preferredEditorPanelId() {
    if (layout.activeEditorPanelId && layout.panels.some((panel) => panel.id === layout.activeEditorPanelId && panel.type === 'editor')) {
      return layout.activeEditorPanelId;
    }
    return layout.panels.find((panel) => panel.type === 'editor')?.id || '';
  }

  function ensureEditorPanel() {
    const existing = preferredEditorPanelId();
    if (existing) return existing;
    return layout.addPanel('editor') || 'editor_1';
  }

  async function openFile(path) {
    const existing = visibleEditorEntries().find(({ editor }) => editor.openTabs.some((tab) => tab.path === path));
    if (existing) {
      layout.setActiveEditorPanelId(existing.panel.id);
      layout.setActivePanel(existing.panel.id);
      await existing.editor.activateTab(path);
      return;
    }
    const panelId = ensureEditorPanel();
    const target = editorPanels[panelId] || editor1;
    layout.setActiveEditorPanelId(panelId);
    layout.setActivePanel(panelId);
    await target.openFile(path);
  }

  async function openFileInEditorPanel(panelId, path, options = {}) {
    if (!path) return;
    const target = editorPanels[panelId] || editor1;
    let movedSnapshot = null;
    if (options.sourcePanelId && options.sourcePanelId !== panelId) {
      const source = editorPanels[options.sourcePanelId];
      movedSnapshot = source?.getTabSnapshot(path) || null;
      if (source && !(await source.detachTab(path))) return;
    }
    layout.setActiveEditorPanelId(panelId);
    layout.setActivePanel(panelId);
    if (movedSnapshot) {
      target.openTabSnapshot(movedSnapshot);
      return;
    }
    await target.openFile(path);
  }

  editorRef.current = {
    applyPathUpdate: (...args) => Object.values(editorPanels).forEach((item) => item.applyPathUpdate(...args)),
    clearPathState: (...args) => Object.values(editorPanels).forEach((item) => item.clearPathState(...args)),
    get currentFileContent() {
      return (editorPanels[preferredEditorPanelId()] || editor1).currentFileContent;
    },
    get currentFilePath() {
      return (editorPanels[preferredEditorPanelId()] || editor1).currentFilePath;
    },
    get isTextFile() {
      return (editorPanels[preferredEditorPanelId()] || editor1).isTextFile;
    },
    openFile,
    setEditorStatusMessage: (...args) => (editorPanels[preferredEditorPanelId()] || editor1).setEditorStatusMessage(...args),
  };

  function buildCurrentFileContext() {
    return buildEditorContextFromEntries(visibleEditorEntries());
  }

  function getEditorPanelState(panelId) {
    return editorPanels[panelId] || editor1;
  }

  function addPanel(type) {
    const panelId = layout.addPanel(type);
    if (type === 'editor' && panelId) layout.setActiveEditorPanelId(panelId);
  }

  async function closePanel(panelId) {
    const canRemove = await (async () => {
      const panel = layout.panels.find((item) => item.id === panelId);
      if (!panel || panel.type !== 'editor') return true;
      const target = editorPanels[panel.id];
      const hasDirtyTab = target?.isDirty || target?.openTabs?.some((tab) => (tab.content || '') !== (tab.savedContent || ''));
      if (!hasDirtyTab) return true;
      if (!confirm) return false;
      return confirm({
        title: '关闭编辑器面板',
        message: '该编辑器有未保存内容，确认关闭这个面板？',
        confirmLabel: '关闭面板',
        tone: 'red',
      });
    })();
    if (!canRemove) return false;
    return layout.removePanel(panelId, (panel) => {
      if (panel.type !== 'editor') return true;
      const target = editorPanels[panel.id];
      const hasDirtyTab = target?.isDirty || target?.openTabs?.some((tab) => (tab.content || '') !== (tab.savedContent || ''));
      if (!hasDirtyTab) return true;
      return true;
    });
  }

  async function closeDraggedPanel() {
    if (!layout.draggingPanelId) return;
    await closePanel(layout.draggingPanelId);
    layout.handlePanelDragEnd();
  }

  return {
    activeEditor,
    addPanel,
    buildCurrentFileContext,
    closeDraggedPanel,
    closePanel,
    editorPanels,
    getEditorPanelState,
    openFile,
    openFileInEditorPanel,
  };
}
