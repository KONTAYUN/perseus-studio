import React from 'react';

export default function useFloatingToolbarViewportBehavior({
  visible,
  onScroll,
  onResize,
}) {
  React.useEffect(() => {
    if (!visible) return undefined;
    window.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('resize', onResize);
    };
  }, [visible, onResize, onScroll]);
}
