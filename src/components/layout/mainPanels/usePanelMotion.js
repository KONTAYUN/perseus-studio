import { useEffect, useLayoutEffect, useRef } from 'react';

const PANEL_LAYOUT_DURATION_MS = 400;
const PANEL_LAYOUT_EASING = 'cubic-bezier(0.16, 1, 0.28, 1)';

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
    }, PANEL_LAYOUT_DURATION_MS + 40);
    return () => window.clearTimeout(timeoutId);
  }, [layoutAnimationAction, setLayoutAnimationAction]);

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
      Boolean(layoutAnimationAction) &&
      (layoutAnimationAction.kind !== 'reorder' || orderChanged);

    visiblePanels.forEach((panel) => {
      const element = document.getElementById(`panel-wrapper-${panel.id}`);
      if (!element) return;

      if (panelAnimationsRef.current[panel.id]) {
        panelAnimationsRef.current[panel.id].cancel();
        delete panelAnimationsRef.current[panel.id];
      }

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
      const scaleX = rect.width > 0 ? previous.width / rect.width : 1;
      const scaleY = rect.height > 0 ? previous.height / rect.height : 1;
      const hasPositionChange = Math.abs(deltaX) >= 1 || Math.abs(deltaY) >= 1;
      const hasSizeChange = Math.abs(1 - scaleX) >= 0.002 || Math.abs(1 - scaleY) >= 0.002;
      if (!hasPositionChange && !hasSizeChange) return;

      panelAnimationsRef.current[panel.id] = element.animate(
        [
          {
            transform: `translate(${deltaX}px, ${deltaY}px) scale(${scaleX}, ${scaleY})`,
            transformOrigin: 'top left',
          },
          {
            transform: 'translate(0, 0) scale(1, 1)',
            transformOrigin: 'top left',
          },
        ],
        {
          duration: PANEL_LAYOUT_DURATION_MS,
          easing: PANEL_LAYOUT_EASING,
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
    const forceIntro =
      layoutAnimationAction?.kind === 'add' && layoutAnimationAction.panelId === id;
    if (forceIntro && !appearingPanelIdsRef.current.has(id)) {
      appearingPanelIdsRef.current.add(id);
      window.clearTimeout(appearTimeoutsRef.current[id]);
      appearTimeoutsRef.current[id] = window.setTimeout(() => {
        appearingPanelIdsRef.current.delete(id);
        delete appearTimeoutsRef.current[id];
      }, PANEL_LAYOUT_DURATION_MS + 40);
    }
    if (!appearedPanelIdsRef.current.has(id)) {
      appearedPanelIdsRef.current.add(id);
      if (hasMeasuredPanelsRef.current) {
        appearingPanelIdsRef.current.add(id);
        window.clearTimeout(appearTimeoutsRef.current[id]);
        appearTimeoutsRef.current[id] = window.setTimeout(() => {
          appearingPanelIdsRef.current.delete(id);
          delete appearTimeoutsRef.current[id];
        }, PANEL_LAYOUT_DURATION_MS + 40);
      }
    }

    return {
      isOpeningPanel: false,
      shouldIntro: appearingPanelIdsRef.current.has(id),
    };
  };

  return { getPanelIntroState };
}
