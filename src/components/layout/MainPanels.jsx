import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import ChatPanel from '../chat/ChatPanel';
import EditorPanel from '../editor/EditorPanel';
import SidebarPanel from '../sidebar/SidebarPanel';
import { minWidthForPanelType } from '../../hooks/useLayoutState';

function getPanelContent(panel, ide) {
  if (panel.type === 'fileTree') return <SidebarPanel ide={ide} panelId={panel.id} />;
  if (panel.type === 'editor') {
    const editor = ide.getEditorPanelState(panel.id);
    return <EditorPanel ide={ide} editor={editor} panelId={panel.id} />;
  }
  if (panel.type === 'chat') return <ChatPanel ide={ide} panelId={panel.id} />;
  return null;
}

export default function MainPanels({ ide }) {
  const mainRef = useRef(null);
  const panelRectsRef = useRef({});
  const panelAnimationsRef = useRef({});
  const previousPanelIdsRef = useRef([]);
  const hasMeasuredPanelsRef = useRef(false);
  const appearedPanelIdsRef = useRef(new Set());
  const appearingPanelIdsRef = useRef(new Set());
  const appearTimeoutsRef = useRef({});
  const [mainWidth, setMainWidth] = useState(0);
  const [expandedIntroKey, setExpandedIntroKey] = useState('');
  const visiblePanels = ide.panels;
  const centerSinglePanel = visiblePanels.length === 1 && visiblePanels[0]?.type === 'chat';
  const isResizing = ide.dragType === 'resize';
  const isReordering = ide.dragType === 'reorder';
  const isReorderMotion = isReordering || ide.layoutAnimationAction?.kind === 'reorder';
  const panelGap = 12;
  const contentWidth = Math.max(0, mainWidth - 24);
  const defaultWidthForPanel = (panel) => {
    if (panel.type === 'fileTree') return 260;
    if (panel.type === 'chat') return centerSinglePanel ? Math.min(1120, Math.max(760, contentWidth)) : 420;
    return 520;
  };
  const displayWidths = useMemo(() => {
    if (mainWidth <= 0 || visiblePanels.length === 0) return {};
    const gapTotal = Math.max(0, visiblePanels.length - 1) * panelGap;
    const minimumTotal = visiblePanels.reduce((sum, panel) => sum + minWidthForPanelType(panel.type), 0);
    const available = Math.max(minimumTotal, contentWidth - gapTotal);
    const entries = visiblePanels.map((panel) => ({
      id: panel.id,
      type: panel.type,
      width: centerSinglePanel && panel.type === 'chat' ? defaultWidthForPanel(panel) : ide.widths[panel.id] || defaultWidthForPanel(panel),
    }));
    let total = entries.reduce((sum, item) => sum + item.width, 0);

    if (total > available) {
      const overflow = total - available;
      const shrinkable = entries.map((item) => Math.max(0, item.width - minWidthForPanelType(item.type)));
      const shrinkableTotal = shrinkable.reduce((sum, value) => sum + value, 0);
      entries.forEach((item, index) => {
        const share = shrinkableTotal > 0 ? overflow * (shrinkable[index] / shrinkableTotal) : overflow / entries.length;
        item.width = Math.max(minWidthForPanelType(item.type), item.width - share);
      });
    } else if (!centerSinglePanel) {
      const growTargets = entries.filter((item) => item.type === 'editor');
      const targets = growTargets.length > 0 ? growTargets : entries;
      const extra = (available - total) / targets.length;
      targets.forEach((item) => {
        item.width += extra;
      });
    }

    return Object.fromEntries(entries.map((item) => [item.id, Math.round(item.width)]));
  }, [centerSinglePanel, ide.widths, mainWidth, visiblePanels]);

  const collectPanelSizes = () =>
    Object.fromEntries(
      visiblePanels
        .map((panel) => {
          const element = document.getElementById(`panel-wrapper-${panel.id}`);
          return [panel.id, Math.round(element?.getBoundingClientRect().width || displayWidths[panel.id] || 0)];
        })
        .filter(([, width]) => width > 0)
    );

  useLayoutEffect(() => {
    if (!mainRef.current) return undefined;
    const update = () => setMainWidth(Math.round(mainRef.current?.clientWidth || 0));
    update();
    const observer = new ResizeObserver(update);
    observer.observe(mainRef.current);
    return () => observer.disconnect();
  }, []);

  useLayoutEffect(() => () => {
    Object.values(appearTimeoutsRef.current).forEach((timeoutId) => window.clearTimeout(timeoutId));
  }, []);

  useLayoutEffect(() => {
    const visibleIds = new Set(visiblePanels.map((panel) => panel.id));
    [...appearedPanelIdsRef.current].forEach((id) => {
      if (!visibleIds.has(id)) appearedPanelIdsRef.current.delete(id);
    });
    [...appearingPanelIdsRef.current].forEach((id) => {
      if (!visibleIds.has(id)) {
        appearingPanelIdsRef.current.delete(id);
        window.clearTimeout(appearTimeoutsRef.current[id]);
        delete appearTimeoutsRef.current[id];
      }
    });
  }, [visiblePanels]);

  useEffect(() => {
    if (!ide.layoutAnimationAction) return undefined;
    const timeoutId = window.setTimeout(() => {
      ide.setLayoutAnimationAction?.(null);
    }, 560);
    return () => window.clearTimeout(timeoutId);
  }, [ide.layoutAnimationAction, ide.setLayoutAnimationAction]);

  useLayoutEffect(() => {
    if (ide.layoutAnimationAction?.kind !== 'add') return undefined;
    const actionKey = String(ide.layoutAnimationAction.key || '');
    setExpandedIntroKey('');
    const frameId = window.requestAnimationFrame(() => {
      setExpandedIntroKey(actionKey);
    });
    return () => window.cancelAnimationFrame(frameId);
  }, [ide.layoutAnimationAction]);

  useLayoutEffect(() => {
    if (mainWidth <= 0 || typeof document === 'undefined') return;
    if (ide.draggingPanelId || ide.isDraggingMode) return;

    const nextRects = {};
    const previousPanelIds = previousPanelIdsRef.current;
    const currentPanelIds = visiblePanels.map((panel) => panel.id);
    const samePanelSet =
      previousPanelIds.length === currentPanelIds.length &&
      previousPanelIds.every((id) => currentPanelIds.includes(id));
    const orderChanged = samePanelSet && previousPanelIds.some((id, index) => id !== currentPanelIds[index]);
    const shouldFlipPanels =
      !isResizing &&
      ((ide.layoutAnimationAction?.kind === 'reorder' && orderChanged) ||
        ((ide.layoutAnimationAction?.kind === 'remove' || ide.layoutAnimationAction?.kind === 'add') && !samePanelSet));

    visiblePanels.forEach((panel) => {
      const element = document.getElementById(`panel-wrapper-${panel.id}`);
      if (!element) return;

      const rect = element.getBoundingClientRect();
      nextRects[panel.id] = {
        left: rect.left,
        top: rect.top,
        width: rect.width,
        height: rect.height,
      };

      const previous = panelRectsRef.current[panel.id];
      if (!shouldFlipPanels) return;
      if (!previous || isResizing || ide.draggingPanelId === panel.id) return;

      const deltaX = previous.left - rect.left;
      const deltaY = previous.top - rect.top;
      if (Math.abs(deltaX) < 1 && Math.abs(deltaY) < 1) return;

      panelAnimationsRef.current[panel.id]?.cancel();
      panelAnimationsRef.current[panel.id] = element.animate(
        [
          {
            transform: `translate(${deltaX}px, ${deltaY}px)`,
            opacity: 0.98,
          },
          {
            transform: 'translate(0, 0)',
            opacity: 1,
          },
        ],
        {
          duration: ide.layoutAnimationAction?.kind === 'reorder' ? 380 : 560,
          easing:
            ide.layoutAnimationAction?.kind === 'reorder'
              ? 'cubic-bezier(0.18, 0.86, 0.28, 1)'
              : 'cubic-bezier(0.16, 1, 0.3, 1)',
          fill: 'none',
        }
      );
      panelAnimationsRef.current[panel.id].finished
        .catch(() => {})
        .finally(() => {
          delete panelAnimationsRef.current[panel.id];
        });
    });

    panelRectsRef.current = nextRects;
    previousPanelIdsRef.current = currentPanelIds;
    hasMeasuredPanelsRef.current = true;
  }, [centerSinglePanel, displayWidths, ide.draggingPanelId, ide.isDraggingMode, ide.layoutAnimationAction, isResizing, mainWidth, visiblePanels]);

  return (
    <main
      ref={mainRef}
      className="flex-1 flex overflow-hidden p-3 relative z-10"
      onClick={(e) => e.target.tagName === 'MAIN' && ide.setActivePanel('none')}
    >
      {ide.isDraggingMode && (
        <div
          className={`fixed inset-0 z-[100] pointer-events-none ${
            ide.dragType === 'resize' ? 'cursor-col-resize' : 'cursor-grabbing'
          }`}
        />
      )}

      {ide.layoutNotice && (
        <div className="absolute left-1/2 top-5 z-[120] -translate-x-1/2 rounded-xl border border-blue-100 bg-white/95 px-4 py-2 text-sm font-semibold text-blue-600 shadow-xl dark:border-blue-500/20 dark:bg-[#252526]/95 dark:text-blue-200">
          {ide.layoutNotice}
        </div>
      )}

      {centerSinglePanel && <div className="flex-1" />}

      {visiblePanels.map((panel, index) => {
        const id = panel.id;
        const nextPanel = visiblePanels[index + 1] || null;
        const showResizer = Boolean(nextPanel);
        const panelWidth = displayWidths[id] || defaultWidthForPanel(panel);
        const hasOpenFloatingMenu =
          (panel.type === 'chat' && (ide.isModelMenuOpen || ide.isConversationMenuOpen)) ||
          (panel.type === 'fileTree' && ide.isAddMenuOpen);
        const forceIntro = ide.layoutAnimationAction?.kind === 'add' && ide.layoutAnimationAction.panelId === id;
        const introActionKey = String(ide.layoutAnimationAction?.key || '');
        const isOpeningPanel = forceIntro && expandedIntroKey !== introActionKey;
        if (forceIntro && !appearingPanelIdsRef.current.has(id)) {
          appearingPanelIdsRef.current.add(id);
          window.clearTimeout(appearTimeoutsRef.current[id]);
          appearTimeoutsRef.current[id] = window.setTimeout(() => {
            appearingPanelIdsRef.current.delete(id);
            delete appearTimeoutsRef.current[id];
          }, 820);
        }
        if (!appearedPanelIdsRef.current.has(id)) {
          appearedPanelIdsRef.current.add(id);
          if (hasMeasuredPanelsRef.current) {
            appearingPanelIdsRef.current.add(id);
            window.clearTimeout(appearTimeoutsRef.current[id]);
            appearTimeoutsRef.current[id] = window.setTimeout(() => {
              appearingPanelIdsRef.current.delete(id);
              delete appearTimeoutsRef.current[id];
            }, 820);
          }
        }
        const shouldIntro = appearingPanelIdsRef.current.has(id);

        return (
          <div
            key={id}
            id={`panel-wrapper-${id}`}
            className={`relative flex h-full min-w-0 will-change-[transform,opacity] ${
              isOpeningPanel || shouldIntro ? 'overflow-hidden' : 'overflow-visible'
            } ${
              isResizing ? '' : 'transition-[width,opacity,margin-right]'
            }`}
            style={{
              width: `${isOpeningPanel ? 0 : panelWidth}px`,
              flex: 'none',
              opacity: isOpeningPanel ? 0 : ide.draggingPanelId === id ? 0.55 : 1,
              transform: ide.draggingPanelId === id
                ? 'scale(0.985) translateY(2px)'
                : 'scale(1) translateY(0)',
              marginRight: showResizer && !isOpeningPanel ? '12px' : '0px',
              zIndex:
                hasOpenFloatingMenu
                  ? 90
                  : ide.activePanel === id || (ide.activePanel === 'chat-input' && id === 'chat')
                  ? 40
                  : 10 - index,
              transitionDuration: isResizing ? '0ms' : isReorderMotion ? '280ms' : '760ms',
              transitionTimingFunction: isReorderMotion
                ? 'cubic-bezier(0.18, 0.86, 0.28, 1)'
                : 'cubic-bezier(0.16, 1, 0.3, 1)',
            }}
          >
            {ide.dragType === 'reorder' && ide.draggingPanelId !== id && (
              <div
                className={`ui-panel-surface absolute inset-0 z-[90] transition-colors ${
                  ide.dragOverId === id ? 'bg-blue-500/10 ring-2 ring-blue-400/60' : ''
                }`}
                onDragOver={(e) => ide.handlePanelDragOver(e, id)}
                onDrop={(e) => ide.handlePanelDrop(e, id)}
                onDragLeave={() => ide.setDragOverId(null)}
              />
            )}

            <div
              className={`h-full w-full min-w-0 ${
                shouldIntro ? 'animate-panel-appear' : ''
              }`}
              style={shouldIntro ? { animationDelay: `${Math.min(120, index * 36)}ms` } : undefined}
            >
              {getPanelContent(panel, ide)}
            </div>

            {showResizer && (
              <div
                className="absolute -right-4 top-0 bottom-0 w-5 cursor-col-resize group flex items-center justify-center z-50"
                onMouseDown={(e) => ide.startResize(e, id, nextPanel.id, collectPanelSizes())}
              >
                <div className="h-10 w-1.5 rounded-full opacity-0 group-hover:opacity-100 transition-opacity duration-300 bg-gray-400/60 dark:bg-gray-500/60 group-active:bg-blue-500" />
              </div>
            )}
          </div>
        );
      })}
      {centerSinglePanel && <div className="flex-1" />}
    </main>
  );
}
