export const FLOATING_TOOLBAR_HEIGHT = 48;
export const FLOATING_TOOLBAR_GAP = 14;
export const FLOATING_TOOLBAR_SAFE_TOP = 92;
export const FLOATING_TOOLBAR_SWITCH_BUFFER = 16;

export function createHiddenFloatingToolbarState(extra = {}) {
  return {
    visible: false,
    position: null,
    placement: 'above',
    ...extra,
  };
}

export function resolveFloatingToolbarPosition({
  anchorTop,
  anchorBottom,
  fallbackTop = anchorTop,
  viewportTop,
  viewportBottom = window.innerHeight,
  toolbarHeight = FLOATING_TOOLBAR_HEIGHT,
  gap = FLOATING_TOOLBAR_GAP,
  switchBuffer = FLOATING_TOOLBAR_SWITCH_BUFFER,
  previousPlacement = 'above',
}) {
  const safeViewportTop = Math.max(viewportTop, FLOATING_TOOLBAR_SAFE_TOP);
  const availableAbove = anchorTop - safeViewportTop;
  const availableBelow = Math.max(viewportBottom - anchorBottom, 0);
  const canPlaceAbove = availableAbove >= toolbarHeight + gap;
  const canPlaceBelow = availableBelow >= toolbarHeight + gap;

  let placement = 'above';
  if (!canPlaceAbove && canPlaceBelow) {
    placement = 'below';
  } else if (
    previousPlacement === 'below' &&
    availableAbove < toolbarHeight + gap + switchBuffer
  ) {
    placement = 'below';
  }

  return {
    placement,
    top:
      placement === 'below'
        ? anchorBottom + gap
        : Math.max(fallbackTop - gap, safeViewportTop),
  };
}
