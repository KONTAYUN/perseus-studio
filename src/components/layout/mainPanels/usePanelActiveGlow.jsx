import { useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

function readPanelGlowRect(panelId) {
  const element = document.getElementById(`panel-wrapper-${panelId}`);
  if (!element) return null;
  const rect = element.getBoundingClientRect();
  return {
    height: Math.round(rect.height),
    left: Math.round(rect.left),
    top: Math.round(rect.top),
    width: Math.round(rect.width),
  };
}

export function usePanelActiveGlow({
  activePanel,
  displayWidths,
  dragType,
  draggingPanelId,
  isDraggingMode,
  layoutAnimationAction,
  mainWidth,
  visiblePanels,
}) {
  const activeGlowPanelRef = useRef('');
  const activeGlowLayersRef = useRef([]);
  const activeGlowLayerIdRef = useRef(0);
  const activeGlowScheduleRef = useRef({
    cleanupTimeouts: [],
    revealFrameIds: [],
  });
  const [activeGlowLayers, setActiveGlowLayers] = useState([]);

  const clearActiveGlowSchedule = () => {
    const schedule = activeGlowScheduleRef.current;
    schedule.cleanupTimeouts.forEach((timeoutId) => window.clearTimeout(timeoutId));
    schedule.revealFrameIds.forEach((frameId) => window.cancelAnimationFrame(frameId));
    activeGlowScheduleRef.current = {
      cleanupTimeouts: [],
      revealFrameIds: [],
    };
  };

  const setActiveGlowLayersValue = (updater) => {
    const next = typeof updater === 'function' ? updater(activeGlowLayersRef.current) : updater;
    activeGlowLayersRef.current = next;
    setActiveGlowLayers(next);
  };

  const scheduleActiveGlowCleanup = () => {
    const timeoutId = window.setTimeout(() => {
      setActiveGlowLayersValue((layers) => layers.filter((layer) => layer.visible));
      activeGlowScheduleRef.current.cleanupTimeouts = activeGlowScheduleRef.current.cleanupTimeouts.filter((id) => id !== timeoutId);
    }, 560);
    activeGlowScheduleRef.current.cleanupTimeouts.push(timeoutId);
  };

  const scheduleActiveGlowReveal = (layerId) => {
    const frameId = window.requestAnimationFrame(() => {
      activeGlowScheduleRef.current.revealFrameIds = activeGlowScheduleRef.current.revealFrameIds.filter((id) => id !== frameId);
      setActiveGlowLayersValue((layers) =>
        layers.map((layer) => (layer.id === layerId ? { ...layer, visible: true } : layer))
      );
    });
    activeGlowScheduleRef.current.revealFrameIds.push(frameId);
  };

  const syncActiveGlowRect = (panelId = activeGlowPanelRef.current) => {
    if (!panelId) return false;
    const rect = readPanelGlowRect(panelId);
    if (!rect) return false;
    setActiveGlowLayersValue((layers) => {
      let changed = false;
      const next = layers.map((layer) => {
        if (layer.panelId !== panelId || !layer.visible) return layer;
        if (
          layer.rect.height === rect.height &&
          layer.rect.left === rect.left &&
          layer.rect.top === rect.top &&
          layer.rect.width === rect.width
        ) {
          return layer;
        }
        changed = true;
        return { ...layer, rect };
      });
      return changed ? next : layers;
    });
    return true;
  };

  useLayoutEffect(() => () => {
    clearActiveGlowSchedule();
  }, []);

  useLayoutEffect(() => {
    clearActiveGlowSchedule();

    const nextPanelId =
      dragType === 'reorder' || draggingPanelId
        ? ''
        : activePanel && activePanel !== 'none' && activePanel !== 'chat-input'
        ? activePanel
        : '';

    const fadeExistingLayers = () => {
      setActiveGlowLayersValue((layers) => layers.map((layer) => ({ ...layer, visible: false })));
      scheduleActiveGlowCleanup();
    };

    if (!nextPanelId) {
      activeGlowPanelRef.current = '';
      fadeExistingLayers();
      return () => clearActiveGlowSchedule();
    }

    const activeElement = document.getElementById(`panel-wrapper-${nextPanelId}`);
    if (!activeElement) {
      activeGlowPanelRef.current = '';
      fadeExistingLayers();
      return () => clearActiveGlowSchedule();
    }

    const rect = readPanelGlowRect(nextPanelId);
    if (!rect) {
      activeGlowPanelRef.current = '';
      fadeExistingLayers();
      return () => clearActiveGlowSchedule();
    }

    const existingVisibleLayer = activeGlowLayersRef.current.find(
      (layer) => layer.panelId === nextPanelId && layer.visible
    );
    if (activeGlowPanelRef.current === nextPanelId && existingVisibleLayer) {
      syncActiveGlowRect(nextPanelId);
    } else {
      activeGlowPanelRef.current = nextPanelId;
      const nextLayer = {
        id: `panel-glow-${Date.now()}-${++activeGlowLayerIdRef.current}`,
        panelId: nextPanelId,
        rect,
        visible: false,
      };
      setActiveGlowLayersValue((layers) => [
        ...layers.map((layer) => ({ ...layer, visible: false })).slice(-2),
        nextLayer,
      ]);
      scheduleActiveGlowReveal(nextLayer.id);
      scheduleActiveGlowCleanup();
    }

    const updateRect = () => {
      if (!syncActiveGlowRect(nextPanelId)) {
        activeGlowPanelRef.current = '';
        fadeExistingLayers();
      }
    };

    window.addEventListener('resize', updateRect);
    window.addEventListener('scroll', updateRect, true);
    const observer = new ResizeObserver(updateRect);
    observer.observe(activeElement);
    return () => {
      clearActiveGlowSchedule();
      window.removeEventListener('resize', updateRect);
      window.removeEventListener('scroll', updateRect, true);
      observer.disconnect();
    };
  }, [activePanel, dragType, draggingPanelId]);

  useLayoutEffect(() => {
    syncActiveGlowRect();
  }, [displayWidths, mainWidth, visiblePanels]);

  useLayoutEffect(() => {
    if (!activeGlowPanelRef.current) return undefined;
    let frameId = 0;
    let stopped = false;
    let startedAt = 0;
    const follow = (timestamp) => {
      if (!startedAt) startedAt = timestamp;
      syncActiveGlowRect();
      const shouldKeepFollowing = isDraggingMode || layoutAnimationAction || timestamp - startedAt < 900;
      if (!stopped && shouldKeepFollowing) {
        frameId = window.requestAnimationFrame(follow);
      }
    };
    frameId = window.requestAnimationFrame(follow);
    return () => {
      stopped = true;
      if (frameId) window.cancelAnimationFrame(frameId);
    };
  }, [
    displayWidths,
    dragType,
    draggingPanelId,
    isDraggingMode,
    layoutAnimationAction,
    mainWidth,
    visiblePanels,
  ]);

  if (typeof document === 'undefined') return null;

  return createPortal(
    activeGlowLayers.map((layer) => (
      <div
        key={layer.id}
        aria-hidden="true"
        className={`panel-active-glow-fixed ${layer.visible ? 'panel-active-glow-fixed-on' : ''}`}
        style={{
          height: `${layer.rect.height}px`,
          left: `${layer.rect.left}px`,
          top: `${layer.rect.top}px`,
          width: `${layer.rect.width}px`,
        }}
      />
    )),
    document.body
  );
}
