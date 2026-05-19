import { useEffect, useLayoutEffect, useRef, useState } from 'react';

export function usePanelMotion({
  displayWidths,
  draggingPanelId,
  isDraggingMode,
  isResizing,
  layoutAnimationAction,
  mainWidth,
  setLayoutAnimationAction,
  visiblePanels,
}) {
  const panelRectsRef = useRef({});
  const panelAnimationsRef = useRef({});
  const previousPanelIdsRef = useRef([]);
  const hasMeasuredPanelsRef = useRef(false);
  const appearedPanelIdsRef = useRef(new Set());
  const appearingPanelIdsRef = useRef(new Set());
  const appearTimeoutsRef = useRef({});
  const [expandedIntroKey, setExpandedIntroKey] = useState('');

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
    if (!layoutAnimationAction) return undefined;
    const timeoutId = window.setTimeout(() => {
      setLayoutAnimationAction?.(null);
    }, 560);
    return () => window.clearTimeout(timeoutId);
  }, [layoutAnimationAction, setLayoutAnimationAction]);

  useLayoutEffect(() => {
    if (layoutAnimationAction?.kind !== 'add') return undefined;
    const actionKey = String(layoutAnimationAction.key || '');
    setExpandedIntroKey('');
    const frameId = window.requestAnimationFrame(() => {
      setExpandedIntroKey(actionKey);
    });
    return () => window.cancelAnimationFrame(frameId);
  }, [layoutAnimationAction]);

  useLayoutEffect(() => {
    if (mainWidth <= 0 || typeof document === 'undefined') return;
    if (draggingPanelId || isDraggingMode) return;

    const nextRects = {};
    const previousPanelIds = previousPanelIdsRef.current;
    const currentPanelIds = visiblePanels.map((panel) => panel.id);
    const samePanelSet =
      previousPanelIds.length === currentPanelIds.length &&
      previousPanelIds.every((id) => currentPanelIds.includes(id));
    const orderChanged = samePanelSet && previousPanelIds.some((id, index) => id !== currentPanelIds[index]);
    const shouldFlipPanels =
      !isResizing &&
      ((layoutAnimationAction?.kind === 'reorder' && orderChanged) ||
        ((layoutAnimationAction?.kind === 'remove' || layoutAnimationAction?.kind === 'add') && !samePanelSet));

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
      if (!previous || isResizing || draggingPanelId === panel.id) return;

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
          duration: layoutAnimationAction?.kind === 'reorder' ? 380 : 560,
          easing:
            layoutAnimationAction?.kind === 'reorder'
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
  }, [displayWidths, draggingPanelId, isDraggingMode, isResizing, layoutAnimationAction, mainWidth, visiblePanels]);

  const getPanelIntroState = (id) => {
    const forceIntro = layoutAnimationAction?.kind === 'add' && layoutAnimationAction.panelId === id;
    const introActionKey = String(layoutAnimationAction?.key || '');
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

    return {
      isOpeningPanel,
      shouldIntro: appearingPanelIdsRef.current.has(id),
    };
  };

  return { getPanelIntroState };
}
