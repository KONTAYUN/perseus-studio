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

function setPanelDragImage(event) {
  if (typeof document === 'undefined' || !event.dataTransfer?.setDragImage) return;
  const dragImage = document.createElement('div');
  dragImage.setAttribute('aria-hidden', 'true');
  dragImage.style.position = 'fixed';
  dragImage.style.left = '-1000px';
  dragImage.style.top = '-1000px';
  dragImage.style.width = '30px';
  dragImage.style.height = '30px';
  dragImage.style.display = 'flex';
  dragImage.style.alignItems = 'center';
  dragImage.style.justifyContent = 'center';
  dragImage.style.borderRadius = '999px';
  dragImage.style.color = 'rgb(254 226 226)';
  dragImage.style.background = 'rgb(248 113 113 / 0.58)';
  dragImage.style.boxShadow = 'inset 0 0 0 1px rgb(254 202 202 / 0.22)';
  dragImage.style.pointerEvents = 'none';
  dragImage.innerHTML = `
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"
      stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
      <rect width="18" height="14" x="3" y="5" rx="2" />
      <path d="M3 10h18" />
    </svg>
  `;
  document.body.appendChild(dragImage);
  event.dataTransfer.setDragImage(dragImage, 15, 15);
  window.setTimeout(() => dragImage.remove(), 0);
}

function getPanelWrapperElement(panelId) {
  if (typeof document === 'undefined' || !panelId) return null;
  return document.getElementById(`panel-wrapper-${panelId}`);
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
    const leftPanelElement = getPanelWrapperElement(leftPanelId);
    const rightPanelElement = getPanelWrapperElement(rightPanelId);
    const canResizeDom = Boolean(leftPanelElement && rightPanelElement && initialLeftW && initialRightW);
    setWidths(initialWidths);
    let animationFrameId = null;
    let latestClientX = initialX;

    const nextWidthsForClientX = (clientX) => {
      const deltaX = clientX - initialX;
      const clampedDelta = Math.max(minDelta, Math.min(deltaX, maxDelta));
      const next = { ...initialWidths };
      if (initialLeftW) next[leftPanelId] = Math.round(initialLeftW + clampedDelta);
      if (initialRightW) next[rightPanelId] = Math.round(initialRightW - clampedDelta);
      return next;
    };

    const applyDomWidths = (next) => {
      if (!canResizeDom) return;
      leftPanelElement.style.width = `${next[leftPanelId]}px`;
      rightPanelElement.style.width = `${next[rightPanelId]}px`;
    };

    const onMouseMove = (moveEvent) => {
      window.getSelection()?.removeAllRanges();
      latestClientX = moveEvent.clientX;
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      animationFrameId = requestAnimationFrame(() => {
        const next = nextWidthsForClientX(latestClientX);
        if (canResizeDom) {
          applyDomWidths(next);
        } else {
          setWidths(next);
        }
      });
    };

    const onMouseUp = (upEvent) => {
      setIsDraggingMode(false);
      setDragType(null);
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      const finalClientX = typeof upEvent?.clientX === 'number' ? upEvent.clientX : latestClientX;
      const finalWidths = nextWidthsForClientX(finalClientX);
      applyDomWidths(finalWidths);
      setWidths(finalWidths);
    };

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  };

  const handlePanelDragStart = (e, id) => {
    e.stopPropagation();
    e.dataTransfer.setData('panel_id', id);
    e.dataTransfer.effectAllowed = 'move';
    setPanelDragImage(e);
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
      return 'ui-panel-surface-drop bg-blue-50/20 dark:bg-blue-500/20 z-40';
    }
    if (activePanel === panelName) {
      return 'border-gray-200/80 dark:border-white/10 z-40';
    }
    return 'border-gray-200/80 dark:border-white/10 z-10';
  };

  const getInputGlowStyle = () => {
    if (activePanel === 'chat-input') {
      return 'composer-shell-active';
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
