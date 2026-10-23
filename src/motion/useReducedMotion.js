import { useSyncExternalStore } from 'react';
import { prefersReducedMotion } from './policy.js';

function subscribe(onChange) {
  const media = globalThis.matchMedia?.('(prefers-reduced-motion: reduce)');
  media?.addEventListener('change', onChange);
  return () => media?.removeEventListener('change', onChange);
}

export function useReducedMotion() {
  return useSyncExternalStore(subscribe, prefersReducedMotion, () => true);
}
