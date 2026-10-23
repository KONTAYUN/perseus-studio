export const MOTION = Object.freeze({ fast: 120, control: 160, popover: 180, panel: 240, modal: 220 });
export const MOTION_EASE = 'cubic-bezier(0.22, 1, 0.36, 1)';

export function prefersReducedMotion() {
  return globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
}

const active = new WeakMap();

export function cancelMotion(element) {
  active.get(element)?.cancel();
  active.delete(element);
}

// One owner per element. An old animation finishing must never clear the
// reference to its replacement during rapid interaction.
export function animateElement(element, keyframes, options = {}) {
  cancelMotion(element);
  if (!element?.animate || prefersReducedMotion()) return null;
  const animation = element.animate(keyframes, { duration: MOTION.panel, easing: MOTION_EASE, ...options });
  active.set(element, animation);
  animation.finished.catch(() => {}).finally(() => {
    if (active.get(element) === animation) active.delete(element);
  });
  return animation;
}
