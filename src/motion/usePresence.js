import { useEffect, useState } from 'react';
import { MOTION } from './policy.js';
import { useReducedMotion } from './useReducedMotion.js';

// Keep closing overlays mounted until their exit transition completes. Opening
// uses CSS @starting-style, so no animation frames are scheduled during render.
export function usePresence(open, duration = MOTION.fast) {
  const [mounted, setMounted] = useState(open);
  const reduced = useReducedMotion();
  useEffect(() => {
    if (open) { setMounted(true); return undefined; }
    if (reduced) { setMounted(false); return undefined; }
    const timer = window.setTimeout(() => setMounted(false), duration);
    return () => window.clearTimeout(timer);
  }, [open, reduced, duration]);
  return open || mounted;
}
