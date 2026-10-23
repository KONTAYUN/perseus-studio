import { useEffect, useLayoutEffect, useRef } from 'react';
import { animateElement, cancelMotion, MOTION } from '../../../motion/policy.js';
import { useReducedMotion } from '../../../motion/useReducedMotion.js';

export function usePanelMotion({ displayWidths, draggingPanelId, isDraggingMode, isResizing,
  layoutAnimationAction, mainWidth, setLayoutAnimationAction, visiblePanels }) {
  const previous = useRef(new Map());
  const elements = useRef(new Set());
  const reduced = useReducedMotion();

  useLayoutEffect(() => () => {
    elements.current.forEach(cancelMotion);
    elements.current.clear();
  }, []);

  useEffect(() => {
    if (!layoutAnimationAction) return undefined;
    const key = layoutAnimationAction.key;
    const timer = window.setTimeout(() => {
      setLayoutAnimationAction?.((current) => current?.key === key ? null : current);
    }, reduced ? 0 : MOTION.panel);
    return () => window.clearTimeout(timer);
  }, [layoutAnimationAction, reduced, setLayoutAnimationAction]);

  useLayoutEffect(() => {
    if (mainWidth <= 0) return;
    const next = new Map();
    const live = new Set();
    const shouldAnimate = Boolean(layoutAnimationAction) && !isResizing && !isDraggingMode && !draggingPanelId && !reduced;
    for (const panel of visiblePanels) {
      const element = document.getElementById('panel-wrapper-' + panel.id);
      if (!element) continue;
      live.add(element);
      const old = previous.current.get(panel.id);
      // Read the visible offset before canceling a previous FLIP.
      const matrix = new DOMMatrixReadOnly(getComputedStyle(element).transform);
      cancelMotion(element);
      const rect = element.getBoundingClientRect();
      next.set(panel.id, { left: rect.left, top: rect.top });
      if (!shouldAnimate) continue;
      if (!old) {
        animateElement(element, [{ opacity: 0, transform: 'translateX(8px)' }, { opacity: 1, transform: 'translateX(0)' }]);
      } else {
        const x = old.left + matrix.m41 - rect.left;
        const y = old.top + matrix.m42 - rect.top;
        if (Math.abs(x) > 0.5 || Math.abs(y) > 0.5) {
          animateElement(element, [{ transform: 'translate(' + x + 'px, ' + y + 'px)' }, { transform: 'translate(0, 0)' }]);
        }
      }
    }
    elements.current.forEach((element) => { if (!live.has(element)) cancelMotion(element); });
    previous.current = next;
    elements.current = live;
  }, [displayWidths, draggingPanelId, isDraggingMode, isResizing, layoutAnimationAction, mainWidth, reduced, visiblePanels]);
}
