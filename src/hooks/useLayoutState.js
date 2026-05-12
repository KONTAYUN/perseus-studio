import { useEffect, useMemo, useState } from 'react';

const EDITOR_PANEL_IDS = ['editor_1', 'editor_2', 'editor_3'];
const MAX_TOTAL_PANELS = 5;
const MAX_EDITOR_PANELS = 3;

function minWidthForPanelType(type) {
  if (type === 'chat') return 340;
  if (type === 'fileTree') return 220;
  return 260;
}

const PRESETS = {
  1: [{ id: 'chat', type: 'chat' }],
  2: [
    { id: 'editor_1', type: 'editor' },
    { id: 'chat', type: 'chat' },
  ],
  3: [
    { id: 'fileTree', type: 'fileTree' },
    { id: 'editor_1', type: 'editor' },
    { id: 'chat', type: 'chat' },
  ],
};

function themeStorageKey(userId) {
  return userId ? `perseus:theme:${userId}` : '';
}

function readStoredTheme(userId) {
  if (!userId || typeof window === 'undefined') return null;
  const value = window.localStorage.getItem(themeStorageKey(userId));
  return value === 'dark' || value === 'light' ? value : null;
}

export default function useLayoutState({ userId = '' } = {}) {
  const [layoutColumns, setLayoutColumnsValue] = useState(3);
  const [panels, setPanels] = useState(PRESETS[3]);
  const [isDarkMode, setIsDarkMode] = useState(() => readStoredTheme(userId) === 'dark');
  const [activePanel, setActivePanel] = useState('none');
  const [activeEditorPanelId, setActiveEditorPanelId] = useState('editor_1');
  const [widths, setWidths] = useState({ fileTree: 260, chat: 400 });
  const [isDraggingMode, setIsDraggingMode] = useState(false);
  const [dragType, setDragType] = useState(null);
  const [dragOverId, setDragOverId] = useState(null);
  const [draggingPanelId, setDraggingPanelId] = useState(null);
  const [isPanelTrashOver, setIsPanelTrashOver] = useState(false);
  const [panelAddMenuOpen, setPanelAddMenuOpen] = useState(false);
  const [layoutNotice, setLayoutNotice] = useState('');
  const [recentlyAddedPanelId, setRecentlyAddedPanelId] = useState('');
  const [layoutAnimationAction, setLayoutAnimationAction] = useState(null);

  const panelOrder = useMemo(() => panels.map((panel) => panel.id), [panels]);

  useEffect(() => {
    const storedTheme = readStoredTheme(userId);
    if (storedTheme) {
      setIsDarkMode(storedTheme === 'dark');
    } else {
      setIsDarkMode(false);
    }
  }, [userId]);

  useEffect(() => {
    if (!userId || typeof window === 'undefined') return;
    window.localStorage.setItem(themeStorageKey(userId), isDarkMode ? 'dark' : 'light');
  }, [isDarkMode, userId]);

  const setLayoutColumns = (columns) => {
    setLayoutAnimationAction({ kind: 'preset', key: Date.now() });
    setLayoutColumnsValue(columns);
    setPanels(PRESETS[columns] || PRESETS[3]);
    setWidths({ fileTree: 260, chat: 400 });
    setActivePanel('none');
    setPanelAddMenuOpen(false);
    setRecentlyAddedPanelId('');
  };

  const isPanelVisible = (id) => panels.some((panel) => panel.id === id);

  const addPanel = (type) => {
    setPanelAddMenuOpen(false);
    if (panels.length >= MAX_TOTAL_PANELS) {
      setLayoutNotice('当前最多支持 5 个面板');
      return '';
    }
    if (type === 'chat') {
      const existing = panels.find((panel) => panel.type === 'chat');
      if (existing) {
        setActivePanel(existing.id);
        setLayoutNotice('当前仅支持一个聊天面板');
        return existing.id;
      }
      setLayoutAnimationAction({ kind: 'add', panelId: 'chat', key: Date.now() });
      setPanels((current) => [...current, { id: 'chat', type: 'chat' }]);
      flashAddedPanel('chat');
      return 'chat';
    }
    if (type === 'fileTree') {
      const existing = panels.find((panel) => panel.type === 'fileTree');
      if (existing) {
        setActivePanel(existing.id);
        setLayoutNotice('当前仅支持一个文件树面板');
        return existing.id;
      }
      setLayoutAnimationAction({ kind: 'add', panelId: 'fileTree', key: Date.now() });
      setPanels((current) => [...current, { id: 'fileTree', type: 'fileTree' }]);
      flashAddedPanel('fileTree');
      return 'fileTree';
    }
    const editorCount = panels.filter((panel) => panel.type === 'editor').length;
    if (editorCount >= MAX_EDITOR_PANELS) {
      setLayoutNotice('当前最多支持 3 个编辑器面板');
      return '';
    }
    const used = new Set(panels.filter((panel) => panel.type === 'editor').map((panel) => panel.id));
    const nextId = EDITOR_PANEL_IDS.find((id) => !used.has(id));
    if (!nextId) {
      setLayoutNotice('当前最多支持 3 个编辑器面板');
      return '';
    }
    setLayoutAnimationAction({ kind: 'add', panelId: nextId, key: Date.now() });
    setPanels((current) => [...current, { id: nextId, type: 'editor' }]);
    flashAddedPanel(nextId);
    return nextId;
  };

  const flashAddedPanel = (panelId) => {
    setRecentlyAddedPanelId(panelId);
    window.setTimeout(() => {
      setRecentlyAddedPanelId((current) => (current === panelId ? '' : current));
    }, 320);
  };

  const removePanel = (panelId, canRemove) => {
    const panel = panels.find((item) => item.id === panelId);
    if (!panel) return false;
    if (canRemove && !canRemove(panel)) return false;
    setLayoutAnimationAction({ kind: 'remove', key: Date.now() });
    setPanels((current) => {
      const next = current.filter((item) => item.id !== panelId);
      return next.length > 0 ? next : [{ id: 'chat', type: 'chat' }];
    });
    if (activePanel === panelId) setActivePanel('none');
    if (activeEditorPanelId === panelId) {
      const nextEditor = panels.find((item) => item.type === 'editor' && item.id !== panelId);
      setActiveEditorPanelId(nextEditor?.id || '');
    }
    return true;
  };

  const startResize = (e, leftPanelId, rightPanelId, panelSizes = {}) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDraggingMode(true);
    setDragType('resize');

    const initialX = e.clientX;
    const initialWidths = { ...widths, ...panelSizes };
    const initialLeftW = initialWidths[leftPanelId] || 0;
    const initialRightW = initialWidths[rightPanelId] || 0;
    const leftPanelType = panels.find((panel) => panel.id === leftPanelId)?.type || 'editor';
    const rightPanelType = panels.find((panel) => panel.id === rightPanelId)?.type || 'editor';
    const minLeftW = Math.min(minWidthForPanelType(leftPanelType), Math.max(120, initialLeftW));
    const minRightW = Math.min(minWidthForPanelType(rightPanelType), Math.max(120, initialRightW));
    const minDelta = minLeftW - initialLeftW;
    const maxDelta = initialRightW - minRightW;
    setWidths(initialWidths);
    let animationFrameId = null;

    const onMouseMove = (moveEvent) => {
      window.getSelection()?.removeAllRanges();
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      animationFrameId = requestAnimationFrame(() => {
        const deltaX = moveEvent.clientX - initialX;
        setWidths(() => {
          const next = { ...initialWidths };
          const clampedDelta = Math.max(minDelta, Math.min(deltaX, maxDelta));
          if (initialLeftW) next[leftPanelId] = Math.round(initialLeftW + clampedDelta);
          if (initialRightW) next[rightPanelId] = Math.round(initialRightW - clampedDelta);
          return next;
        });
      });
    };

    const onMouseUp = () => {
      setIsDraggingMode(false);
      setDragType(null);
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
    };

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  };

  const handlePanelDragStart = (e, id) => {
    e.stopPropagation();
    e.dataTransfer.setData('panel_id', id);
    e.dataTransfer.effectAllowed = 'move';
    setTimeout(() => {
      setDraggingPanelId(id);
      setIsDraggingMode(true);
      setDragType('reorder');
      setPanelAddMenuOpen(false);
    }, 0);
  };

  const handlePanelDragOver = (e, id) => {
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = 'move';
    if (dragOverId !== id) setDragOverId(id);
  };

  const handlePanelDrop = (e, targetId) => {
    e.preventDefault();
    e.stopPropagation();
    setDragOverId(null);
    const sourceId = draggingPanelId || e.dataTransfer.getData('panel_id');
    if (sourceId && sourceId !== targetId) {
      setLayoutAnimationAction({ kind: 'reorder', key: Date.now() });
      setPanels((current) => {
        const sourceIndex = current.findIndex((panel) => panel.id === sourceId);
        const targetIndex = current.findIndex((panel) => panel.id === targetId);
        if (sourceIndex === -1 || targetIndex === -1) return current;
        const next = [...current];
        const [item] = next.splice(sourceIndex, 1);
        next.splice(targetIndex, 0, item);
        return next;
      });
    }
    handlePanelDragEnd();
  };

  const handlePanelDragEnd = () => {
    setDraggingPanelId(null);
    setIsDraggingMode(false);
    setDragType(null);
    setDragOverId(null);
    setIsPanelTrashOver(false);
  };

  const getPanelGlowStyle = (panelName) => {
    if (dragOverId === panelName && dragType !== 'reorder') {
      return 'ring-2 ring-blue-500 bg-blue-50/20 dark:bg-blue-500/20 border-transparent';
    }
    if (activePanel === panelName) {
      return 'ring-[1.5px] ring-blue-500/80 shadow-[0_0_24px_rgba(59,130,246,0.25)] dark:shadow-[0_0_30px_rgba(59,130,246,0.35)] border-transparent z-40';
    }
    return 'border-gray-200/80 dark:border-white/10 shadow-xl z-10';
  };

  const getInputGlowStyle = () => {
    if (activePanel === 'chat-input') {
      return 'border-blue-400/45 ring-1 ring-blue-500/16 shadow-[0_18px_44px_rgba(15,23,42,0.12),0_0_0_1px_rgba(59,130,246,0.06)] dark:border-blue-300/30 dark:ring-blue-300/14 dark:shadow-[0_18px_46px_rgba(0,0,0,0.36),0_0_0_1px_rgba(96,165,250,0.08)]';
    }
    return 'border-gray-200/75 dark:border-white/10 hover:border-gray-300/85 dark:hover:border-white/16';
  };

  return {
    activeEditorPanelId,
    activePanel,
    addPanel,
    dragOverId,
    dragType,
    draggingPanelId,
    getInputGlowStyle,
    getPanelGlowStyle,
    handlePanelDragEnd,
    handlePanelDragOver,
    handlePanelDragStart,
    handlePanelDrop,
    isDarkMode,
    isDraggingMode,
    isPanelTrashOver,
    layoutAnimationAction,
    isPanelVisible,
    layoutColumns,
    layoutNotice,
    panelAddMenuOpen,
    panelOrder,
    panels,
    recentlyAddedPanelId,
    removePanel,
    setActiveEditorPanelId,
    setActivePanel,
    setDragOverId,
    setIsDarkMode,
    setIsPanelTrashOver,
    setLayoutColumns,
    setLayoutAnimationAction,
    setLayoutNotice,
    setPanelAddMenuOpen,
    startResize,
    widths,
  };
}

export { EDITOR_PANEL_IDS, MAX_EDITOR_PANELS, MAX_TOTAL_PANELS, minWidthForPanelType };
