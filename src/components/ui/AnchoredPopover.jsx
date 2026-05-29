import React from 'react';
import { createPortal } from 'react-dom';
import { cn } from './cn';

export function PopoverSurface({ children, className = '', ...props }) {
  return (
    <div
      className={cn(
        'ui-popover-surface overflow-hidden rounded-xl border border-gray-100 bg-white/95 backdrop-blur-xl dark:border-white/10 dark:bg-[#252527]/95',
        className
      )}
      {...props}
    >
      {children}
    </div>
  );
}

export function AnchoredPopover({
  anchorRef,
  children,
  className = '',
  align = 'end',
  gap = 8,
  estimatedHeight = 0,
  margin = 12,
  maxHeight = 420,
  matchAnchorWidth = false,
  minWidth = 180,
  onClose,
  open = true,
  placement = 'bottom',
  showBackdrop = true,
  testId,
  width = 320,
  zIndex = 430,
}) {
  const popoverRef = React.useRef(null);
  const position = useAnchoredPopoverPosition(anchorRef, popoverRef, {
    align,
    estimatedHeight,
    gap,
    margin,
    maxHeight,
    matchAnchorWidth,
    minWidth,
    open,
    placement,
    width,
  });

  React.useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (event) => {
      if (event.key === 'Escape') onClose?.();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose, open]);

  if (!open || typeof document === 'undefined') return null;

  return createPortal(
    <>
      {showBackdrop && <div className="fixed inset-0" style={{ zIndex: zIndex - 10 }} onClick={() => onClose?.()} />}
      <div
        ref={popoverRef}
        data-testid={testId}
        className={cn(
          'motion-popover ui-popover-surface fixed overflow-hidden rounded-2xl border border-gray-100/90 bg-white/95 backdrop-blur-2xl dark:border-white/10 dark:bg-[#252527]/95',
          className
        )}
        style={{
          left: position.left,
          top: position.top,
          width: position.width,
          maxHeight: position.maxHeight,
          zIndex,
        }}
        onClick={(event) => event.stopPropagation()}
      >
        {typeof children === 'function' ? children({ position }) : children}
      </div>
    </>,
    document.body
  );
}

function useAnchoredPopoverPosition(anchorRef, popoverRef, options) {
  const {
    align = 'end',
    estimatedHeight = 0,
    gap = 8,
    margin = 12,
    maxHeight = 420,
    matchAnchorWidth = false,
    minWidth = 180,
    open = true,
    placement = 'bottom',
    width: preferredWidth = 320,
  } = options || {};
  const [position, setPosition] = React.useState({
    left: margin,
    top: 64,
    width: preferredWidth,
    maxHeight,
  });

  const updatePosition = React.useCallback(() => {
    const anchor = anchorRef?.current;
    if (!anchor) return;

    const rect = anchor.getBoundingClientRect();
    const popoverHeight = measurePopoverHeight(popoverRef?.current);
    const viewportWidth = window.innerWidth || document.documentElement.clientWidth;
    const viewportHeight = window.innerHeight || document.documentElement.clientHeight;
    const baseWidth = matchAnchorWidth ? rect.width : preferredWidth;
    const resolvedWidth = Math.max(minWidth, Math.min(baseWidth, viewportWidth - margin * 2));

    if (placement === 'left' || placement === 'right') {
      const rightSpace = viewportWidth - rect.right - gap - margin;
      const leftSpace = rect.left - gap - margin;
      const openLeft = placement === 'left' ? leftSpace >= resolvedWidth || leftSpace >= rightSpace : rightSpace < resolvedWidth && leftSpace > rightSpace;
      const sideLeft = openLeft ? rect.left - gap - resolvedWidth : rect.right + gap;
      const left = Math.min(Math.max(sideLeft, margin), viewportWidth - resolvedWidth - margin);
      const availableHeight = Math.max(0, viewportHeight - margin * 2);
      const resolvedMaxHeight = resolveMaxHeight(availableHeight, maxHeight, 120);
      const placementHeight = resolvePlacementHeight(popoverHeight, resolvedMaxHeight, estimatedHeight);
      const anchorMiddle = rect.top + rect.height / 2;
      const centeredTop = anchorMiddle - placementHeight / 2;
      const top = Math.min(Math.max(centeredTop, margin), viewportHeight - placementHeight - margin);

      setPosition({ left, top, width: resolvedWidth, maxHeight: resolvedMaxHeight });
      return;
    }

    const alignedLeft =
      align === 'start'
        ? rect.left
        : align === 'center'
          ? rect.left + rect.width / 2 - resolvedWidth / 2
          : rect.right - resolvedWidth;
    const left = Math.min(Math.max(alignedLeft, margin), viewportWidth - resolvedWidth - margin);

    const below = viewportHeight - rect.bottom - gap - margin;
    const above = rect.top - gap - margin;
    const opensAbove = below < 240 && above > below;
    const resolvedMaxHeight = resolveMaxHeight(opensAbove ? above : below, maxHeight, 80);
    const placementHeight = resolvePlacementHeight(popoverHeight, resolvedMaxHeight, estimatedHeight);
    const top = opensAbove
      ? Math.max(margin, rect.top - gap - placementHeight)
      : Math.min(rect.bottom + gap, viewportHeight - placementHeight - margin);

    setPosition({ left, top, width: resolvedWidth, maxHeight: resolvedMaxHeight });
  }, [align, anchorRef, estimatedHeight, gap, margin, matchAnchorWidth, maxHeight, minWidth, placement, popoverRef, preferredWidth]);

  React.useLayoutEffect(() => {
    if (!open) return undefined;
    updatePosition();
    let frameId = 0;
    let startedAt = 0;
    const followAnchor = (timestamp) => {
      if (!startedAt) startedAt = timestamp;
      updatePosition();
      if (timestamp - startedAt < 700) {
        frameId = window.requestAnimationFrame(followAnchor);
      }
    };
    frameId = window.requestAnimationFrame(followAnchor);

    const anchor = anchorRef?.current;
    const popover = popoverRef?.current;
    const observer = typeof ResizeObserver !== 'undefined' && (anchor || popover) ? new ResizeObserver(updatePosition) : null;
    if (anchor) observer?.observe(anchor);
    if (popover) observer?.observe(popover);

    window.addEventListener('resize', updatePosition);
    window.addEventListener('scroll', updatePosition, true);
    return () => {
      if (frameId) window.cancelAnimationFrame(frameId);
      observer?.disconnect();
      window.removeEventListener('resize', updatePosition);
      window.removeEventListener('scroll', updatePosition, true);
    };
  }, [anchorRef, open, popoverRef, updatePosition]);

  return position;
}

function measurePopoverHeight(popover) {
  if (!popover) return 0;
  const rectHeight = Math.ceil(popover.getBoundingClientRect().height || 0);
  const scrollHeight = Math.ceil(popover.scrollHeight || 0);
  return Math.max(rectHeight, scrollHeight, 0);
}

function resolveMaxHeight(available, maxHeight, minHeight) {
  const availableHeight = Math.max(0, available);
  return Math.max(minHeight, Math.min(maxHeight, availableHeight));
}

function resolvePlacementHeight(popoverHeight, maxHeight, estimatedHeight = 0) {
  const measuredHeight = popoverHeight > 0 ? popoverHeight : estimatedHeight;
  return measuredHeight > 0 ? Math.min(measuredHeight, maxHeight) : maxHeight;
}
